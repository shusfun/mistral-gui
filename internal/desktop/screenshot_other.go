//go:build !windows

package desktop

import "errors"

func captureVirtualScreenPNG() (string, int, int, error) {
	return "", 0, 0, errors.New("platform_ocr_unsupported")
}

func startPlatformHotkeyLoop(_ <-chan struct{}, _ func() string, _ func()) {
}
