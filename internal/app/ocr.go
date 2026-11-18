package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	archiveutil "github.com/shusfun/mistral-gui/internal/archive"
	"github.com/shusfun/mistral-gui/internal/catalog"
	"github.com/shusfun/mistral-gui/internal/download"
	"github.com/shusfun/mistral-gui/internal/update"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	ocrReleaseTag            = "ocr-v1.4.4"
	ocrEstimatedArchiveBytes = 138 * 1024 * 1024 // Rounded up from the 137.1 MiB release archive
	ocrRequiredDiskBytes     = 640 * 1024 * 1024 // Archive + 358.2 MiB unpacked runtime + headroom
	ocrIdleTimeout           = 10 * time.Minute
)

const embeddedOCRWorkerPy = `import io
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

TOKEN = os.environ.get("MISTRAL_GUI_OCR_TOKEN", "")
PORT = int(os.environ.get("MISTRAL_GUI_OCR_PORT", "0"))

engine = None

def get_engine():
    global engine
    if engine is None:
        from rapidocr_onnxruntime import RapidOCR
        engine = RapidOCR()
    return engine

class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        return

    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        if self.headers.get("X-OCR-Token", "") != TOKEN:
            self.send_response(401)
            self.end_headers()
            return
        if self.path != "/ocr":
            self.send_response(404)
            self.end_headers()
            return
        length = int(self.headers.get("Content-Length", "0"))
        if length <= 0 or length > 20 * 1024 * 1024:
            self.send_response(400)
            self.end_headers()
            return
        img_bytes = self.rfile.read(length)
        try:
            ocr = get_engine()
            result, _ = ocr(img_bytes)
            lines = []
            if result:
                for item in result:
                    if len(item) >= 3 and item[1]:
                        lines.append({
                            "text": str(item[1]).strip(),
                            "score": float(item[2]),
                        })
            body = json.dumps({"lines": lines}, ensure_ascii=False).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        except Exception as exc:
            err_body = json.dumps({"error": str(exc)}).encode("utf-8")
            self.send_response(500)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(err_body)

if __name__ == "__main__":
    get_engine()
    srv = HTTPServer(("127.0.0.1", PORT), Handler)
    srv.serve_forever()
`

type OCRLine struct {
	Text  string  `json:"text"`
	Score float64 `json:"score"`
}

type ocrProcess struct {
	cmd     *exec.Cmd
	target  string
	token   string
	done    chan struct{}
	release func()
}

type OCRSupervisor struct {
	app            *App
	mu             sync.Mutex
	startMu        sync.Mutex
	dir            string
	proc           *ocrProcess
	lastUse        time.Time
	customExecPath string // Optional override for offline integration tests
}

func newOCRSupervisor(a *App) *OCRSupervisor {
	s := &OCRSupervisor{
		app: a,
		dir: filepath.Join(a.dataDir, "ocr"),
	}
	go s.idleReaper()
	return s
}

func (o *OCRSupervisor) idleReaper() {
	if o == nil || o.app == nil {
		return
	}
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-o.app.closed:
			return
		case <-tick.C:
			o.mu.Lock()
			shouldStop := o.proc != nil && !o.lastUse.IsZero() && time.Since(o.lastUse) > ocrIdleTimeout
			o.mu.Unlock()
			if shouldStop {
				o.Stop()
			}
		}
	}
}

func (o *OCRSupervisor) findInstalledBinaryLocked() (bin string, args []string, ok bool) {
	if o.customExecPath != "" {
		return o.customExecPath, nil, true
	}
	candidates := []string{
		filepath.Join(o.dir, "python", "python.exe"),
		filepath.Join(o.dir, "python.exe"),
		filepath.Join(o.dir, "bin", "python3"),
		filepath.Join(o.dir, "python", "bin", "python3"),
	}
	workerScript := filepath.Join(o.dir, "ocr_worker.py")
	if _, err := os.Stat(workerScript); err != nil {
		return "", nil, false
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, []string{workerScript}, true
		}
	}
	return "", nil, false
}

func (o *OCRSupervisor) dirDiskBytes() int64 {
	var total int64
	_ = filepath.WalkDir(o.dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (o *OCRSupervisor) Status() map[string]any {
	if o == nil {
		return map[string]any{"installed": false, "running": false, "state": "not_installed"}
	}
	o.mu.Lock()
	_, _, installed := o.findInstalledBinaryLocked()
	running := o.proc != nil
	o.mu.Unlock()

	state := "not_installed"
	if running {
		state = "running"
	} else if installed {
		state = "installed"
	}
	var diskBytes int64
	if installed {
		diskBytes = o.dirDiskBytes()
	}
	return map[string]any{
		"installed":        installed,
		"running":          running,
		"state":            state,
		"download_bytes":   int64(ocrEstimatedArchiveBytes),
		"disk_bytes":       diskBytes,
		"engine":           "RapidOCR v1.4.4 + PP-OCRv4 ONNX (CPU)",
		"license":          "Apache-2.0 (RapidOCR & PP-OCRv4) / MIT (ONNX Runtime) / PSF (Python)",
		"platform_capture": runtime.GOOS == "windows",
	}
}

func (o *OCRSupervisor) Stop() {
	if o == nil {
		return
	}
	o.mu.Lock()
	p := o.proc
	o.proc = nil
	o.mu.Unlock()
	if p != nil {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		if p.release != nil {
			p.release()
		}
		select {
		case <-p.done:
		case <-time.After(2 * time.Second):
		}
	}
}

func (o *OCRSupervisor) Delete() error {
	if o == nil {
		return nil
	}
	o.Stop()
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := os.RemoveAll(o.dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (o *OCRSupervisor) ensureWorker(ctx context.Context) (*ocrProcess, error) {
	o.startMu.Lock()
	defer o.startMu.Unlock()

	o.mu.Lock()
	if o.proc != nil {
		p := o.proc
		o.lastUse = time.Now()
		o.mu.Unlock()
		return p, nil
	}
	bin, args, installed := o.findInstalledBinaryLocked()
	o.mu.Unlock()
	if !installed {
		return nil, errors.New("ocr_not_installed")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("ocr_failed")
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	secret := randomToken()[:24]
	cmd := exec.Command(bin, args...)
	setProcess(cmd)
	cmd.Dir = o.dir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("MISTRAL_GUI_OCR_PORT=%d", port),
		"MISTRAL_GUI_OCR_TOKEN="+secret,
		"PYTHONUTF8=1",
	)
	if err := cmd.Start(); err != nil {
		return nil, errors.New("ocr_failed")
	}
	release, err := ownProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, errors.New("process_supervision_failed")
	}

	p := &ocrProcess{
		cmd:     cmd,
		target:  fmt.Sprintf("http://127.0.0.1:%d", port),
		token:   secret,
		done:    make(chan struct{}),
		release: release,
	}
	go func() {
		defer release()
		_ = cmd.Wait()
		o.mu.Lock()
		if o.proc == p {
			o.proc = nil
		}
		close(p.done)
		o.mu.Unlock()
	}()

	// Poll worker /healthz up to 45s for cold ONNX initialization
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return nil, ctx.Err()
		case <-p.done:
			return nil, errors.New("ocr_failed")
		default:
		}
		resp, hErr := client.Get(p.target + "/healthz")
		if hErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				o.mu.Lock()
				o.proc = p
				o.lastUse = time.Now()
				o.mu.Unlock()
				return p, nil
			}
		}
		time.Sleep(120 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return nil, errors.New("ocr_failed")
}

func (o *OCRSupervisor) Recognize(ctx context.Context, imageBytes []byte) ([]OCRLine, int64, error) {
	if len(imageBytes) == 0 || len(imageBytes) > 20<<20 {
		return nil, 0, errors.New("ocr_failed")
	}
	start := time.Now()
	p, err := o.ensureWorker(ctx)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", p.target+"/ocr", bytes.NewReader(imageBytes))
	if err != nil {
		return nil, 0, errors.New("ocr_failed")
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-OCR-Token", p.token)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, errors.New("ocr_failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, 0, errors.New("ocr_failed")
	}
	var parsed struct {
		Lines []OCRLine `json:"lines"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&parsed); err != nil {
		return nil, 0, errors.New("ocr_failed")
	}
	o.mu.Lock()
	o.lastUse = time.Now()
	o.mu.Unlock()
	return parsed.Lines, time.Since(start).Milliseconds(), nil
}

func ocrArchiveAssetName() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return "rapidocr-ppocrv4-windows-amd64.zip", nil
	case "darwin/arm64":
		return "rapidocr-ppocrv4-darwin-arm64.tar.gz", nil
	case "darwin/amd64":
		return "rapidocr-ppocrv4-darwin-amd64.tar.gz", nil
	default:
		return "", errors.New("platform_unsupported")
	}
}

func (a *App) ensureOCRSupervisor() *OCRSupervisor {
	root := a.root()
	root.mu.Lock()
	defer root.mu.Unlock()
	if root.ocr == nil {
		root.ocr = newOCRSupervisor(root)
	}
	return root.ocr
}

func (a *App) resolveOCRAsset(ctx context.Context) (catalog.Asset, error) {
	assetName, err := ocrArchiveAssetName()
	if err != nil {
		return catalog.Asset{}, err
	}
	repo := SupplementRepository
	if repo == "" {
		repo = UpdateRepository
	}
	if repo == "" {
		return catalog.Asset{}, errors.New("supplement_repository_required")
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, ocrReleaseTag)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return catalog.Asset{}, err
	}
	resp, err := a.catalog.HTTP.Do(req)
	if err != nil {
		return catalog.Asset{}, errors.New("download_connection_failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return catalog.Asset{}, fmt.Errorf("metadata_http_%03d", resp.StatusCode)
	}
	var rel struct {
		Assets []struct {
			Name               string `json:"name"`
			Size               int64  `json:"size"`
			Digest             string `json:"digest"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&rel); err != nil {
		return catalog.Asset{}, errors.New("manifest_invalid")
	}
	expectedPrefix := "https://github.com/" + repo + "/releases/download/"
	var found catalog.Asset
	var sigURL, sumsURL string
	for _, item := range rel.Assets {
		if !strings.HasPrefix(item.BrowserDownloadURL, expectedPrefix) {
			continue
		}
		if item.Name == assetName {
			found = catalog.Asset{
				Kind:    "ocr",
				ID:      assetName,
				Version: ocrReleaseTag,
				URL:     item.BrowserDownloadURL,
				Size:    item.Size,
				SHA256:  strings.TrimPrefix(item.Digest, "sha256:"),
			}
		} else if item.Name == assetName+".sig" {
			sigURL = item.BrowserDownloadURL
		} else if item.Name == "SHA256SUMS" {
			sumsURL = item.BrowserDownloadURL
		}
	}
	if found.ID == "" {
		return catalog.Asset{}, errors.New("asset_missing")
	}
	if found.SHA256 == "" && sumsURL != "" {
		sReq, _ := http.NewRequestWithContext(ctx, "GET", sumsURL, nil)
		if sResp, sErr := a.catalog.HTTP.Do(sReq); sErr == nil {
			b, _ := io.ReadAll(io.LimitReader(sResp.Body, 64<<10))
			_ = sResp.Body.Close()
			for _, line := range strings.Split(string(b), "\n") {
				parts := strings.Fields(line)
				if len(parts) >= 2 && strings.TrimPrefix(parts[1], "*") == assetName && len(parts[0]) == 64 {
					found.SHA256 = strings.ToLower(parts[0])
					break
				}
			}
		}
	}
	if len(found.SHA256) != 64 {
		return catalog.Asset{}, errors.New("official_digest_missing")
	}
	found.SignatureURL = sigURL
	return found, nil
}

func (a *App) installOCRBundle(ctx context.Context, opID string) error {
	root := a.root()
	sup := root.ensureOCRSupervisor()
	if err := checkDiskSpace(root.dataDir, ocrRequiredDiskBytes); err != nil {
		return err
	}
	root.changeOp(opID, func(o *Operation) {
		o.AssetKind = "ocr"
		o.AssetID = "rapidocr-ppocrv4"
		o.Step = "checking"
	})
	asset, err := root.resolveOCRAsset(ctx)
	if err != nil {
		return err
	}
	root.mu.Lock()
	mirrors := append([]string(nil), root.config.Mirrors...)
	root.mu.Unlock()

	urls := []string{asset.URL}
	for _, m := range mirrors {
		urls = append(urls, strings.TrimRight(m, "/")+"/"+asset.URL)
	}
	destArchive := filepath.Join(root.dataDir, "downloads", asset.Version+"-"+asset.ID)
	err = download.Download(ctx, download.Request{
		Headers: http.Header{"Traceparent": []string{root.operationTrace(opID)}},
		URLs:    urls,
		Dest:    destArchive,
		SHA256:  asset.SHA256,
		MaxSize: max(asset.Size, 256<<20),
		Idle:    60 * time.Second,
		OnProgress: func(p download.Progress) {
			root.changeOp(opID, func(o *Operation) {
				o.Progress = p.Done
				o.Total = p.Total
				o.Source = p.Source
				o.Step = "downloading"
			})
		},
	})
	if err != nil {
		return err
	}
	if UpdatePublicKey != "" && asset.SignatureURL == "" {
		return errors.New("asset_signature_invalid")
	}
	if asset.SignatureURL != "" && UpdatePublicKey != "" {
		sigReq, e := http.NewRequestWithContext(ctx, "GET", asset.SignatureURL, nil)
		if e != nil {
			return e
		}
		sResp, e := root.client.Do(sigReq)
		if e != nil {
			return e
		}
		sigBytes, rErr := io.ReadAll(io.LimitReader(sResp.Body, 4096))
		_ = sResp.Body.Close()
		if sResp.StatusCode != 200 || rErr != nil {
			return errors.New("asset_signature_invalid")
		}
		if e = update.VerifyFileSignature(destArchive, sigBytes, UpdatePublicKey); e != nil {
			_ = os.Remove(destArchive)
			return e
		}
	}

	root.changeOp(opID, func(o *Operation) { o.Step = "extracting" })
	sup.Stop()
	staging := sup.dir + ".staging"
	_ = os.RemoveAll(staging)
	if err := archiveutil.Extract(destArchive, staging, 512<<20); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, "ocr_worker.py"), []byte(embeddedOCRWorkerPy), 0600); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	_ = os.RemoveAll(sup.dir)
	if err := os.Rename(staging, sup.dir); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	_ = os.Remove(destArchive)
	slog.Info("ocr.installed", "dir", sup.dir)
	return nil
}

func (a *App) ocrStatus(w http.ResponseWriter, r *http.Request) {
	sup := a.root().ensureOCRSupervisor()
	writeJSON(w, 200, sup.Status())
}

func (a *App) ocrInstall(w http.ResponseWriter, r *http.Request) {
	root := a.root()
	root.taskStart.Lock()
	defer root.taskStart.Unlock()
	root.mu.Lock()
	for _, op := range root.operations {
		if (op.Kind == "ocr_install" || op.Kind == "install" || op.Kind == "update") && op.State == "running" {
			root.mu.Unlock()
			fail(w, 409, "installation_in_progress")
			return
		}
	}
	root.mu.Unlock()
	trace := r.Header.Get("traceparent")
	op := root.operation("ocr_install", trace, func(ctx context.Context, id string) error {
		return root.installOCRBundle(ctx, id)
	})
	writeJSON(w, 202, op)
}

func (a *App) ocrDelete(w http.ResponseWriter, r *http.Request) {
	sup := a.root().ensureOCRSupervisor()
	if err := sup.Delete(); err != nil {
		fail(w, 500, "operation_failed")
		return
	}
	writeJSON(w, 200, sup.Status())
}
