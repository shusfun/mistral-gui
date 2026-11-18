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
	Source      string `json:"source"`
	Translation string `json:"translation,omitempty"`
	Channel     string `json:"channel,omitempty"`
	Error       string `json:"error,omitempty"`
}

type TranslateRequest struct {
	Text              string `json:"text"`
	ImageBase64       string `json:"image_base64"`
	SourceLang        string `json:"source_lang"`
	TargetLang        string `json:"target_lang"`
	Routing           string `json:"routing"`
	Model             string `json:"model"`
	ConfirmCloudImage bool   `json:"confirm_cloud_image"`
}

type TranslateResponse struct {
	Segments       []TranslateSegmentResult `json:"segments"`
	OCRText        string                   `json:"ocr_text,omitempty"`
	OCRMs          int64                    `json:"ocr_ms,omitempty"`
	FallbackReason string                   `json:"fallback_reason,omitempty"`
	ModelUsed      string                   `json:"model_used,omitempty"`
}

func splitDesktopParagraphs(text string) []string {
	norm := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	rawParts := strings.Split(norm, "\n")
	out := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
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

func (a *App) callChatModelSingle(ctx context.Context, endpoint, bearer, model, source, targetLang, trace string) (string, error) {
	prompt, err := BuildTranslationPrompt(model, []string{source}, targetLang)
	if err != nil {
		return "", err
	}
	reqPayload := map[string]any{
		"model": model,
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

func (a *App) translateLocalSegment(ctx context.Context, deploymentID, source, targetLang, trace string) (string, string, error) {
	root := a.root()
	target := root
	if deploymentID != "" {
		resolved, err := root.resolveDeployment(deploymentID)
		if err != nil {
			return "", "", err
		}
		target = resolved
	}
	target.mu.Lock()
	auto := target.config.AutoStart
	running := target.engine != nil
	modelName := target.config.Model
	target.mu.Unlock()

	if !running && !auto {
		return "", "", errors.New("runtime_stopped")
	}

	target.mu.Lock()
	target.state.Active++
	target.lastUse = time.Now()
	target.mu.Unlock()
	defer func() {
		target.mu.Lock()
		target.state.Active--
		target.lastUse = time.Now()
		target.mu.Unlock()
	}()

	if err := target.ensureEngine(ctx, trace); err != nil {
		return "", "", err
	}
	target.mu.Lock()
	p := target.engine
	if p == nil {
		target.mu.Unlock()
		return "", "", errors.New("engine_exited")
	}
	engineEndpoint := p.target + "/v1/chat/completions"
	modelName = target.config.Model
	target.state.Requests++
	target.lastUse = time.Now()
	target.mu.Unlock()

	tr, err := root.callChatModelSingle(ctx, engineEndpoint, "", modelName, source, targetLang, trace)
	return tr, modelName, err
}

func (a *App) translateCloudSegment(ctx context.Context, cloudURL, cloudKey, cloudModel, source, targetLang, trace string) (string, error) {
	if strings.TrimSpace(cloudURL) == "" || strings.TrimSpace(cloudModel) == "" {
		return "", errors.New("cloud_not_configured")
	}
	if strings.TrimSpace(cloudKey) == "" {
		return "", errors.New("cloud_key_required")
	}
	return a.callChatModelSingle(ctx, strings.TrimSpace(cloudURL), strings.TrimSpace(cloudKey), strings.TrimSpace(cloudModel), source, targetLang, trace)
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

	srcLang := NormalizeRuleLang(req.SourceLang)
	tgtLang := NormalizeRuleLang(req.TargetLang)
	if tgtLang == "auto" || tgtLang == "*" {
		tgtLang = "zh-CN"
	}

	inputText := strings.TrimSpace(req.Text)
	var ocrText string
	var ocrMs int64

	if strings.TrimSpace(req.ImageBase64) != "" {
		rawB64 := strings.TrimSpace(req.ImageBase64)
		if idx := strings.Index(rawB64, ","); idx >= 0 && strings.HasPrefix(rawB64, "data:") {
			rawB64 = rawB64[idx+1:]
		}
		imgBytes, err := base64.StdEncoding.DecodeString(rawB64)
		if err != nil || len(imgBytes) == 0 {
			fail(w, 400, "invalid_json")
			return
		}
		// Privacy gate: if routing may send OCR-extracted text to cloud, require explicit one-time confirmation
		if routing != "local-only" && !cfg.CloudImageConfirmed && !req.ConfirmCloudImage {
			fail(w, 409, "cloud_image_confirmation_required")
			return
		}
		if req.ConfirmCloudImage && !cfg.CloudImageConfirmed {
			root.mu.Lock()
			root.config.CloudImageConfirmed = true
			root.mu.Unlock()
			_ = root.save()
		}
		lines, elapsed, err := root.ensureOCRSupervisor().Recognize(r.Context(), imgBytes)
		if err != nil {
			fail(w, 503, err.Error())
			return
		}
		parts := make([]string, 0, len(lines))
		for _, ln := range lines {
			if strings.TrimSpace(ln.Text) != "" {
				parts = append(parts, strings.TrimSpace(ln.Text))
			}
		}
		ocrText = strings.Join(parts, "\n")
		ocrMs = elapsed
		if ocrText == "" && inputText == "" {
			fail(w, 422, "ocr_empty_text")
			return
		}
		if inputText == "" {
			inputText = ocrText
		}
	}

	paragraphs := splitDesktopParagraphs(inputText)
	if len(paragraphs) == 0 {
		writeJSON(w, 200, TranslateResponse{
			Segments: []TranslateSegmentResult{},
			OCRText:  ocrText,
			OCRMs:    ocrMs,
		})
		return
	}
	if len(paragraphs) > 128 {
		logStage("translate.end", "input_limit", -1)
		fail(w, 400, "translation_input_limit")
		return
	}

	dict := root.ensureDictionaryStore()
	results := make([]TranslateSegmentResult, len(paragraphs))
	var fallbackReason string
	var modelUsed string

	for i, seg := range paragraphs {
		if r.Context().Err() != nil {
			logStage("translate.end", "cancelled", i)
			return
		}
		logStage("dictionary.lookup", "started", i)
		if tr, ch, ok := dict.Lookup(seg, srcLang, tgtLang); ok {
			logStage("dictionary.lookup", ch, i)
			results[i] = TranslateSegmentResult{
				Source:      seg,
				Translation: tr,
				Channel:     ch,
			}
			continue
		}
		logStage("model.request", routing, i)

		switch routing {
		case "local-only":
			// Strict local-only guarantee: NEVER touches CloudAPIURL under any condition.
			tr, mUsed, err := root.translateLocalSegment(r.Context(), req.Model, seg, tgtLang, trace)
			if err != nil {
				results[i] = TranslateSegmentResult{Source: seg, Error: err.Error()}
			} else {
				modelUsed = mUsed
				results[i] = TranslateSegmentResult{Source: seg, Translation: tr, Channel: "local"}
			}
		case "cloud-only":
			tr, err := root.translateCloudSegment(r.Context(), cfg.CloudAPIURL, cfg.CloudAPIKey, cfg.CloudModel, seg, tgtLang, trace)
			if err != nil {
				results[i] = TranslateSegmentResult{Source: seg, Error: err.Error()}
			} else {
				modelUsed = cfg.CloudModel
				results[i] = TranslateSegmentResult{Source: seg, Translation: tr, Channel: "cloud"}
			}
		default: // local-first
			tr, mUsed, localErr := root.translateLocalSegment(r.Context(), req.Model, seg, tgtLang, trace)
			if localErr == nil {
				modelUsed = mUsed
				results[i] = TranslateSegmentResult{Source: seg, Translation: tr, Channel: "local"}
				continue
			}
			if fallbackReason == "" {
				fallbackReason = localErr.Error()
			}
			cTr, cloudErr := root.translateCloudSegment(r.Context(), cfg.CloudAPIURL, cfg.CloudAPIKey, cfg.CloudModel, seg, tgtLang, trace)
			if cloudErr != nil {
				results[i] = TranslateSegmentResult{Source: seg, Error: cloudErr.Error()}
			} else {
				modelUsed = cfg.CloudModel
				results[i] = TranslateSegmentResult{Source: seg, Translation: cTr, Channel: "cloud"}
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
	if !anySuccess && firstErr != "" {
		logStage("translate.end", "failed", -1)
		fail(w, 503, firstErr)
		return
	}
	logStage("translate.end", "success", -1)

	writeJSON(w, 200, TranslateResponse{
		Segments:       results,
		OCRText:        ocrText,
		OCRMs:          ocrMs,
		FallbackReason: fallbackReason,
		ModelUsed:      modelUsed,
	})
}
