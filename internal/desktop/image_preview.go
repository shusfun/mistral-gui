package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	imagePreviewChanged = "desktop:image-preview-changed"
	maxPreviewDataURL   = ((16*1024*1024 + 2) / 3 * 4) + 128
	maxPreviewPayload   = maxPreviewDataURL + 2*1024*1024
)

type imagePreviewOverlay struct {
	Box         [4]float64 `json:"box"`
	Text        string     `json:"text"`
	SourceLines int        `json:"sourceLines"`
}

type imagePreviewSnapshot struct {
	Src      string                `json:"src"`
	Overlays []imagePreviewOverlay `json:"overlays"`
	View     string                `json:"view"`
	Theme    string                `json:"theme"`
	Lang     string                `json:"lang"`
	Revision uint64                `json:"revision"`
	Trace    string                `json:"trace"`
}

type imagePreviewState struct {
	window   *application.WebviewWindow
	snapshot *imagePreviewSnapshot
	revision uint64
}

func parseImagePreview(value string) (*imagePreviewSnapshot, error) {
	invalid := errors.New("image_preview_failed")
	if len(value) > maxPreviewPayload {
		return nil, invalid
	}
	snapshot := &imagePreviewSnapshot{}
	if err := json.Unmarshal([]byte(value), snapshot); err != nil {
		return nil, invalid
	}
	header, encoded, ok := strings.Cut(snapshot.Src, ",")
	if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") || encoded == "" || len(header) > 127 || len(snapshot.Src) > maxPreviewDataURL || len(snapshot.Overlays) > 128 {
		return nil, invalid
	}
	if snapshot.View != "original" && snapshot.View != "translated" {
		return nil, invalid
	}
	for _, overlay := range snapshot.Overlays {
		box := overlay.Box
		if box[0] < 0 || box[1] < 0 || box[2] > 1 || box[3] > 1 || box[0] >= box[2] || box[1] >= box[3] || overlay.Text == "" || overlay.SourceLines < 1 {
			return nil, invalid
		}
	}
	if len(snapshot.Overlays) == 0 {
		snapshot.Overlays = []imagePreviewOverlay{}
		snapshot.View = "original"
	}
	if snapshot.Theme != "light" && snapshot.Theme != "dark" {
		snapshot.Theme = "system"
	}
	return snapshot, nil
}

func (s *Service) openImagePreview(ctx context.Context, value, trace string) error {
	if s.window == nil || ctx.Value(application.WindowKey) != s.window {
		return errors.New("desktop_action_required")
	}
	snapshot, err := parseImagePreview(value)
	if err != nil {
		return err
	}
	snapshot.Trace = trace
	application.InvokeSync(func() {
		preview := &s.imagePreview
		preview.revision++
		snapshot.Revision = preview.revision
		preview.snapshot = snapshot
		if preview.window != nil {
			preview.window.Show()
			if preview.window.IsMinimised() {
				preview.window.UnMinimise()
			}
			preview.window.Focus()
			preview.window.EmitEvent(imagePreviewChanged)
			return
		}
		window := s.host.Window.NewWithOptions(application.WebviewWindowOptions{
			Name: "image-preview", Title: s.text("图片预览", "Image Preview"),
			URL: "/manage?desktop=1&view=image-preview", Width: 961, Height: 671,
			MinWidth: 419, MinHeight: 301,
		})
		preview.window = window
		window.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
			application.InvokeSync(func() {
				if preview.window == window {
					s.manager.NativeTrace("image.preview.closed", preview.snapshot.Trace)
					preview.window = nil
					preview.snapshot = nil
				}
			})
		})
	})
	return nil
}

func (s *Service) readImagePreview(ctx context.Context) (*imagePreviewSnapshot, error) {
	snapshot := application.InvokeSyncWithResult(func() *imagePreviewSnapshot {
		preview := &s.imagePreview
		if preview.window == nil || ctx.Value(application.WindowKey) != preview.window {
			return nil
		}
		return preview.snapshot
	})
	if snapshot == nil {
		return nil, errors.New("image_preview_failed")
	}
	s.manager.NativeTrace("image.preview.ready", snapshot.Trace)
	return snapshot, nil
}
