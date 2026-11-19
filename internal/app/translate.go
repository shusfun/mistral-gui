package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type TranslateSegmentResult struct {
	Source      string      `json:"source"`
	RawSource   string      `json:"raw_source,omitempty"`
	Box         *[4]float64 `json:"box,omitempty"`
	Translation string      `json:"translation,omitempty"`
	Channel     string      `json:"channel,omitempty"`
	Error       string      `json:"error,omitempty"`
}

type TranslateRequest struct {
	Text              string `json:"text"`
	ImageBase64       string `json:"image_base64"`
	SourceLang        string `json:"source_lang"`
	TargetLang        string `json:"target_lang"`
	Routing           string `json:"routing"`
	Model             string `json:"model"`
	PreserveLines     bool   `json:"preserve_lines"`
	ConfirmCloudImage bool   `json:"confirm_cloud_image"`
	ReadyOnly         bool   `json:"ready_only"`
	OCRDerived        bool   `json:"ocr_derived"`
}

type TranslateResponse struct {
	Segments                  []TranslateSegmentResult `json:"segments"`
	OCRText                   string                   `json:"ocr_text,omitempty"`
	OCRRawText                string                   `json:"ocr_raw_text,omitempty"`
	OCRMs                     int64                    `json:"ocr_ms,omitempty"`
	FallbackReason            string                   `json:"fallback_reason,omitempty"`
	ModelUsed                 string                   `json:"model_used,omitempty"`
	CloudConfirmationRequired bool                     `json:"cloud_confirmation_required,omitempty"`
}

func splitDesktopParagraphs(text string) []string {
	rebuilt := RebuildParagraphs(text, false)
	out := make([]string, 0, len(rebuilt))
	for _, p := range rebuilt {
		if p.Merged != "" {
			out = append(out, p.Merged)
		}
	}
	return out
}

func extractChatCompletionText(body []byte) (string, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", errors.New("invalid_response")
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("empty_response")
	}
	txt := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if txt == "" {
		txt = strings.TrimSpace(parsed.Choices[0].Text)
	}
	if txt == "" {
		return "", errors.New("empty_response")
	}
	// Strip optional markdown code fences if a general chat model wrapped single output
	if strings.HasPrefix(txt, "```") && strings.HasSuffix(txt, "```") {
		lines := strings.Split(txt, "\n")
		if len(lines) >= 3 {
			txt = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	return txt, nil
}

func (a *App) callChatModelSingle(ctx context.Context, endpoint, bearer, modelID, promptModel, source, targetLang, trace string) (string, error) {
	prompt, err := BuildTranslationPrompt(promptModel, []string{source}, targetLang)
	if err != nil {
		return "", err
	}
	reqPayload := map[string]any{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.1,
		"stream":      false,
	}
	encoded, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if trace != "" {
		req.Header.Set("traceparent", trace)
	}
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("engine_connection_failed")
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", errors.New("invalid_response")
	}
	if resp.StatusCode != 200 {
		if bearer == "" && resp.StatusCode >= http.StatusInternalServerError {
			return "", errors.New("engine_inference_failed")
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return "", errors.New("cloud_key_required")
		}
		return "", errors.New("operation_failed")
	}
	translated, err := extractChatCompletionText(respBytes)
	if err != nil {
		return "", err
	}
	if !ValidTranslatedSegment(source, translated) {
		return "", errors.New("invalid_response")
	}
	return translated, nil
}

func (a *App) reserveTranslationEngine(readyOnly bool) (*engineProcess, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if isProcessReady(a.engine) && a.state.State == "running" {
		a.state.Active++
		a.lastUse = time.Now()
		return a.engine, nil
	}
	if readyOnly {
		return nil, errors.New("translation_model_not_ready")
	}
	if a.engine == nil && !a.config.AutoStart {
		return nil, errors.New("runtime_stopped")
	}
	a.state.Active++
	a.lastUse = time.Now()
	return nil, nil
}

func (a *App) releaseTranslationEngine() {
	a.mu.Lock()
	a.state.Active--
	a.lastUse = time.Now()
	a.mu.Unlock()
}

func (a *App) translateLocalSegment(ctx context.Context, process *engineProcess, source, targetLang, trace string) (string, string, error) {
	a.mu.Lock()
	if a.engine != process || a.state.State != "running" {
		a.mu.Unlock()
		return "", "", errors.New("engine_exited")
	}
	modelName := a.config.Model
	a.state.Requests++
	a.mu.Unlock()
	translation, err := a.root().callChatModelSingle(ctx, process.target+"/v1/chat/completions", "", process.modelID, modelName, source, targetLang, trace)
	return translation, modelName, err
}

func (a *App) translateCloudSegment(ctx context.Context, cloudURL, cloudKey, cloudModel, source, targetLang, trace string) (string, error) {
	if strings.TrimSpace(cloudURL) == "" || strings.TrimSpace(cloudModel) == "" {
		return "", errors.New("cloud_not_configured")
	}
	if strings.TrimSpace(cloudKey) == "" {
		return "", errors.New("cloud_key_required")
	}
	modelID := strings.TrimSpace(cloudModel)
	return a.callChatModelSingle(ctx, strings.TrimSpace(cloudURL), strings.TrimSpace(cloudKey), modelID, modelID, source, targetLang, trace)
}

func (a *App) translateDesktop(w http.ResponseWriter, r *http.Request) {
	root := a.root()
	trace := r.Header.Get("traceparent")
	started := time.Now()
	logStage := func(phase, outcome string, index int) {
		traceID := ""
		if validTraceparent(trace) {
			traceID = trace[3:35]
		}
		slog.Info("translation.stage", "traceId", traceID, "phase", phase, "outcome", outcome, "segment", index, "durationMs", time.Since(started).Milliseconds(), "code.file.path", "internal/app/translate.go", "code.function.name", "translateDesktop")
	}
	logStage("translate.start", "started", -1)
	var req TranslateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 24<<20))
	if err := dec.Decode(&req); err != nil {
		fail(w, 400, "invalid_json")
		return
	}

	root.mu.Lock()
	cfg := root.config
	root.mu.Unlock()

	routing := strings.TrimSpace(req.Routing)
	if routing == "" {
		routing = cfg.TranslateRouting
	}
	if routing == "" {
		routing = "local-first"
	}
	if routing != "local-first" && routing != "local-only" && routing != "cloud-only" {
		fail(w, 400, "invalid_routing")
		return
	}
	if req.ReadyOnly {
		if routing == "cloud-only" {
			logStage("model.ready_only", "skipped", -1)
			fail(w, 409, "translation_model_not_ready")
			return
		}
		routing = "local-only"
	}
	imageInput := strings.TrimSpace(req.ImageBase64) != ""
	imageDerived := imageInput || req.OCRDerived
	cloudConfigured := strings.TrimSpace(cfg.CloudAPIURL) != "" && strings.TrimSpace(cfg.CloudModel) != "" && strings.TrimSpace(cfg.CloudAPIKey) != ""
	canSendCloud := func() bool {
		root.mu.Lock()
		defer root.mu.Unlock()
		return !imageDerived || root.config.CloudImageConfirmed || (req.ConfirmCloudImage && !req.ReadyOnly)
	}
	cloudConfirmationRequired := false
	var confirmCloudSave bool
	defer func() {
		if confirmCloudSave {
			_ = root.save()
		}
	}()
	translateCloud := func(source, targetLang string) (string, error) {
		if imageDerived && cloudConfigured && req.ConfirmCloudImage && !req.ReadyOnly && !confirmCloudSave {
			root.mu.Lock()
			if !root.config.CloudImageConfirmed {
				root.config.CloudImageConfirmed = true
				confirmCloudSave = true
			}
			root.mu.Unlock()
			if confirmCloudSave {
				logStage("ocr.privacy_confirmation", "accepted", -1)
			}
		}
		return root.translateCloudSegment(r.Context(), cfg.CloudAPIURL, cfg.CloudAPIKey, cfg.CloudModel, source, targetLang, trace)
	}

	srcLang := NormalizeRuleLang(req.SourceLang)
	tgtLang := NormalizeRuleLang(req.TargetLang)
	if tgtLang == "auto" || tgtLang == "*" {
		tgtLang = "zh-CN"
	}
	var target *App
	var process *engineProcess
	var localErr error
	var startDone chan error
	var startCancel context.CancelFunc
	var localPrepared, reserved bool
	defer func() {
		if startCancel != nil {
			startCancel()
		}
		if reserved {
			target.releaseTranslationEngine()
		}
	}()
	prepareLocal := func() {
		if localPrepared {
			return
		}
		localPrepared = true
		target, localErr = root.resolveDeployment(req.Model)
		if localErr != nil {
			return
		}
		process, localErr = target.reserveTranslationEngine(req.ReadyOnly)
		if localErr != nil {
			return
		}
		reserved = true
		if process == nil {
			var startCtx context.Context
			startCtx, startCancel = context.WithCancel(r.Context())
			startDone = make(chan error, 1)
			go func() { startDone <- target.ensureEngine(startCtx, trace) }()
		}
	}
	if req.ReadyOnly {
		prepareLocal()
		if localErr != nil {
			logStage("model.ready_only", "skipped", -1)
			fail(w, 409, "translation_model_not_ready")
			return
		}
		logStage("model.ready_only", "hit", -1)
	}
	firstInference := true
	translateLocal := func(source string, index int) (string, string, error) {
		prepareLocal()
		if startDone != nil {
			localErr = <-startDone
			startDone = nil
			target.mu.Lock()
			process = target.engine
			target.mu.Unlock()
			if localErr == nil && process == nil {
				localErr = errors.New("engine_exited")
			}
		}
		if localErr != nil {
			return "", "", localErr
		}
		inferenceStarted := time.Now()
		translated, model, err := target.translateLocalSegment(r.Context(), process, source, tgtLang, trace)
		if firstInference {
			outcome := "success"
			if err != nil {
				outcome = err.Error()
			}
			slog.Info("translation.first_inference", "traceId", traceRoot(trace), "outcome", outcome, "segment", index, "durationMs", time.Since(inferenceStarted).Milliseconds(), "code.file.path", "internal/app/translate.go", "code.function.name", "translateDesktop")
			firstInference = false
		}
		return translated, model, err
	}

	inputText := strings.TrimSpace(req.Text)
	var ocrText string
	var ocrRawText string
	var ocrMs int64
	var builtParagraphs []OCRParagraph
	rawLineCount := 0

	if imageInput {
		rawB64 := strings.TrimSpace(req.ImageBase64)
		if idx := strings.Index(rawB64, ","); idx >= 0 && strings.HasPrefix(rawB64, "data:") {
			rawB64 = rawB64[idx+1:]
		}
		imgBytes, err := base64.StdEncoding.DecodeString(rawB64)
		if err != nil || len(imgBytes) == 0 {
			fail(w, 400, "invalid_json")
			return
		}
		imgW, imgH := decodeImageDimensions(imgBytes)
		if !req.ReadyOnly && routing != "cloud-only" && imgW > 0 && imgH > 0 {
			prepareLocal()
			if startDone != nil {
				logStage("model.ocr_overlap", "started", -1)
			}
		}
		logStage("ocr.recognize", "started", -1)
		lines, elapsed, err := root.ensureOCRSupervisor().Recognize(r.Context(), imgBytes)
		if err != nil {
			logStage("ocr.recognize", "failed", -1)
			fail(w, 503, err.Error())
			return
		}
		logStage("ocr.recognize", "success", -1)
		ocrParas, mergedTxt, rawTxt, lineCnt := SegmentOCRLines(lines, imgW, imgH, req.PreserveLines)
		ocrText = mergedTxt
		ocrRawText = rawTxt
		ocrMs = elapsed
		if ocrText == "" && inputText == "" {
			fail(w, 422, "ocr_empty_text")
			return
		}
		if inputText == "" {
			builtParagraphs = ocrParas
			rawLineCount = lineCnt
		}
	}

	if len(builtParagraphs) == 0 && inputText != "" {
		norm := strings.ReplaceAll(strings.ReplaceAll(inputText, "\r\n", "\n"), "\r", "\n")
		for _, ln := range strings.Split(norm, "\n") {
			if strings.TrimSpace(ln) != "" {
				rawLineCount++
			}
		}
		rebuilt := RebuildParagraphs(inputText, req.PreserveLines)
		builtParagraphs = make([]OCRParagraph, 0, len(rebuilt))
		for _, rp := range rebuilt {
			if rp.Merged != "" {
				builtParagraphs = append(builtParagraphs, OCRParagraph{
					Merged: rp.Merged,
					Raw:    rp.Raw,
				})
			}
		}
	}

	traceID := ""
	if validTraceparent(trace) {
		traceID = trace[3:35]
	}
	slog.Info(
		"translation.stage",
		"traceId", traceID,
		"phase", "paragraph.rebuild",
		"outcome", "success",
		"rawLines", rawLineCount,
		"rebuiltParagraphs", len(builtParagraphs),
		"preserveLines", req.PreserveLines,
		"durationMs", time.Since(started).Milliseconds(),
		"code.file.path", "internal/app/translate.go",
		"code.function.name", "translateDesktop",
	)

	if len(builtParagraphs) == 0 {
		writeJSON(w, 200, TranslateResponse{
			Segments:   []TranslateSegmentResult{},
			OCRText:    ocrText,
			OCRRawText: ocrRawText,
			OCRMs:      ocrMs,
		})
		return
	}
	if len(builtParagraphs) > 128 {
		logStage("translate.end", "input_limit", -1)
		fail(w, 400, "translation_input_limit")
		return
	}
	totalRunes := 0
	for _, bp := range builtParagraphs {
		segRunes := len([]rune(bp.Merged))
		if segRunes > 8000 {
			logStage("translate.end", "input_limit", -1)
			fail(w, 400, "translation_input_limit")
			return
		}
		totalRunes += segRunes
	}
	if totalRunes > 32000 {
		logStage("translate.end", "input_limit", -1)
		fail(w, 400, "translation_input_limit")
		return
	}

	dict := root.ensureDictionaryStore()
	results := make([]TranslateSegmentResult, len(builtParagraphs))
	var fallbackReason string
	var modelUsed string

	for i, item := range builtParagraphs {
		seg := item.Merged
		rawSrc := item.Raw
		if rawSrc == "" {
			rawSrc = seg
		}
		if r.Context().Err() != nil {
			logStage("translate.end", "cancelled", i)
			return
		}
		logStage("dictionary.lookup", "started", i)
		if tr, ch, ok := dict.Lookup(seg, srcLang, tgtLang); ok {
			logStage("dictionary.lookup", ch, i)
			results[i] = TranslateSegmentResult{
				Source:      seg,
				RawSource:   rawSrc,
				Box:         item.Box,
				Translation: tr,
				Channel:     ch,
			}
			continue
		}
		logStage("model.request", routing, i)

		switch routing {
		case "local-only":
			// Strict local-only guarantee: NEVER touches CloudAPIURL under any condition.
			tr, mUsed, err := translateLocal(seg, i)
			if err != nil {
				results[i] = TranslateSegmentResult{Source: seg, RawSource: rawSrc, Box: item.Box, Error: err.Error()}
			} else {
				modelUsed = mUsed
				results[i] = TranslateSegmentResult{Source: seg, RawSource: rawSrc, Box: item.Box, Translation: tr, Channel: "local"}
			}
		case "cloud-only":
			if cloudConfigured && !canSendCloud() {
				cloudConfirmationRequired = true
				results[i] = TranslateSegmentResult{Source: seg, RawSource: rawSrc, Box: item.Box, Error: "cloud_image_confirmation_required"}
				break
			}
			tr, err := translateCloud(seg, tgtLang)
			if err != nil {
				results[i] = TranslateSegmentResult{Source: seg, RawSource: rawSrc, Box: item.Box, Error: err.Error()}
			} else {
				modelUsed = cfg.CloudModel
				results[i] = TranslateSegmentResult{Source: seg, RawSource: rawSrc, Box: item.Box, Translation: tr, Channel: "cloud"}
			}
		default: // local-first
			tr, mUsed, localErr := translateLocal(seg, i)
			if localErr == nil {
				modelUsed = mUsed
				results[i] = TranslateSegmentResult{Source: seg, RawSource: rawSrc, Box: item.Box, Translation: tr, Channel: "local"}
				break
			}
			logStage("model.local_failed", localErr.Error(), i)
			if !cloudConfigured || !canSendCloud() {
				cloudConfirmationRequired = cloudConfirmationRequired || cloudConfigured
				results[i] = TranslateSegmentResult{Source: seg, RawSource: rawSrc, Box: item.Box, Error: localErr.Error()}
				break
			}
			cTr, cloudErr := translateCloud(seg, tgtLang)
			if cloudErr != nil {
				results[i] = TranslateSegmentResult{Source: seg, RawSource: rawSrc, Box: item.Box, Error: cloudErr.Error()}
			} else {
				if fallbackReason == "" {
					fallbackReason = localErr.Error()
				}
				modelUsed = cfg.CloudModel
				results[i] = TranslateSegmentResult{Source: seg, RawSource: rawSrc, Box: item.Box, Translation: cTr, Channel: "cloud"}
			}
		}
		outcome := results[i].Channel
		if results[i].Error != "" {
			outcome = results[i].Error
		}
		logStage("segment.end", outcome, i)
	}

	anySuccess := false
	firstErr := ""
	for _, rItem := range results {
		if rItem.Translation != "" {
			anySuccess = true
		} else if firstErr == "" && rItem.Error != "" {
			firstErr = rItem.Error
		}
	}
	if cloudConfirmationRequired {
		logStage("ocr.privacy_confirmation", "required", -1)
		logStage("translate.end", "cloud_confirmation_required", -1)
	} else if !anySuccess && firstErr != "" {
		if ocrText == "" {
			logStage("translate.end", "failed", -1)
			fail(w, 503, firstErr)
			return
		}
		logStage("translate.end", "ocr_only", -1)
	} else {
		logStage("translate.end", "success", -1)
	}

	writeJSON(w, 200, TranslateResponse{
		Segments:                  results,
		OCRText:                   ocrText,
		OCRRawText:                ocrRawText,
		OCRMs:                     ocrMs,
		FallbackReason:            fallbackReason,
		ModelUsed:                 modelUsed,
		CloudConfirmationRequired: cloudConfirmationRequired,
	})
}
