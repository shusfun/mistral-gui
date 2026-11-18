//go:build windows

package desktop

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	modUser32              = syscall.NewLazyDLL("user32.dll")
	modGDI32               = syscall.NewLazyDLL("gdi32.dll")
	procGetSystemMetrics   = modUser32.NewProc("GetSystemMetrics")
	procGetDC              = modUser32.NewProc("GetDC")
	procReleaseDC          = modUser32.NewProc("ReleaseDC")
	procRegisterHotKey     = modUser32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = modUser32.NewProc("UnregisterHotKey")
	procPeekMessageW       = modUser32.NewProc("PeekMessageW")
	procCreateCompatibleDC = modGDI32.NewProc("CreateCompatibleDC")
	procDeleteDC           = modGDI32.NewProc("DeleteDC")
	procCreateDIBSection   = modGDI32.NewProc("CreateDIBSection")
	procSelectObject       = modGDI32.NewProc("SelectObject")
	procDeleteObject       = modGDI32.NewProc("DeleteObject")
	procBitBlt             = modGDI32.NewProc("BitBlt")
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
	srccopy           = 0x00CC0020
	dibRGBColors      = 0
	biRGB             = 0
	wmHotkey          = 0x0312
	pmRemove          = 0x0001
	modAlt            = 0x0001
	modControl        = 0x0002
	modShift          = 0x0004
	modWin            = 0x0008
	modNoRepeat       = 0x4000
)

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
	BmiColors [1]uint32
}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

func captureVirtualScreenPNG() (string, int, int, error) {
	x, _, _ := procGetSystemMetrics.Call(smXVirtualScreen)
	y, _, _ := procGetSystemMetrics.Call(smYVirtualScreen)
	w, _, _ := procGetSystemMetrics.Call(smCXVirtualScreen)
	h, _, _ := procGetSystemMetrics.Call(smCYVirtualScreen)
	width := int(int32(w))
	height := int(int32(h))
	if width <= 0 || height <= 0 {
		width = 1920
		height = 1080
	}
	if width > 16384 || height > 16384 {
		return "", 0, 0, errors.New("ocr_failed")
	}

	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return "", 0, 0, errors.New("ocr_failed")
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return "", 0, 0, errors.New("ocr_failed")
	}
	defer procDeleteDC.Call(hdcMem)

	var bmi bitmapInfo
	bmi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bmi.BmiHeader))
	bmi.BmiHeader.BiWidth = int32(width)
	bmi.BmiHeader.BiHeight = -int32(height) // top-down DIB
	bmi.BmiHeader.BiPlanes = 1
	bmi.BmiHeader.BiBitCount = 32
	bmi.BmiHeader.BiCompression = biRGB

	var bits unsafe.Pointer
	hbmp, _, _ := procCreateDIBSection.Call(
		hdcScreen,
		uintptr(unsafe.Pointer(&bmi)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hbmp == 0 || bits == nil {
		return "", 0, 0, errors.New("ocr_failed")
	}
	defer procDeleteObject.Call(hbmp)

	oldObj, _, _ := procSelectObject.Call(hdcMem, hbmp)
	defer procSelectObject.Call(hdcMem, oldObj)

	ok, _, _ := procBitBlt.Call(
		hdcMem,
		0,
		0,
		uintptr(width),
		uintptr(height),
		hdcScreen,
		x,
		y,
		srccopy,
	)
	if ok == 0 {
		return "", 0, 0, errors.New("ocr_failed")
	}

	byteLen := width * height * 4
	rawBGRA := unsafe.Slice((*byte)(bits), byteLen)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	dst := img.Pix
	for i := 0; i < byteLen; i += 4 {
		dst[i+0] = rawBGRA[i+2] // R
		dst[i+1] = rawBGRA[i+1] // G
		dst[i+2] = rawBGRA[i+0] // B
		dst[i+3] = 255          // A
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", 0, 0, errors.New("ocr_failed")
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), width, height, nil
}

func parseWinHotkey(spec string) (uintptr, uintptr, bool) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return 0, 0, false
	}
	parts := strings.Split(trimmed, "+")
	var mods uintptr
	var vk uintptr
	for _, p := range parts {
		tok := strings.ToUpper(strings.TrimSpace(p))
		switch tok {
		case "ALT", "OPTION":
			mods |= modAlt
		case "CTRL", "CONTROL":
			mods |= modControl
		case "SHIFT":
			mods |= modShift
		case "WIN", "META", "SUPER", "CMD":
			mods |= modWin
		default:
			if len(tok) == 1 && tok[0] >= 'A' && tok[0] <= 'Z' {
				vk = uintptr(tok[0])
			} else if len(tok) == 1 && tok[0] >= '0' && tok[0] <= '9' {
				vk = uintptr(tok[0])
			}
		}
	}
	if mods == 0 || vk == 0 {
		return 0, 0, false
	}
	return mods | modNoRepeat, vk, true
}

func startPlatformHotkeyLoop(closed <-chan struct{}, getSpec func() string, onTrigger func()) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		currentSpec := ""
		registered := false
		defer func() {
			if registered {
				_, _, _ = procUnregisterHotKey.Call(0, 1)
			}
		}()

		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-closed:
				return
			case <-ticker.C:
				desired := strings.TrimSpace(getSpec())
				if desired != currentSpec {
					if registered {
						_, _, _ = procUnregisterHotKey.Call(0, 1)
						registered = false
					}
					currentSpec = desired
					if mods, vk, ok := parseWinHotkey(desired); ok {
						r, _, _ := procRegisterHotKey.Call(0, 1, mods, vk)
						registered = r != 0
					}
				}
				var msg winMsg
				for {
					has, _, _ := procPeekMessageW.Call(
						uintptr(unsafe.Pointer(&msg)),
						0,
						0,
						0,
						pmRemove,
					)
					if has == 0 {
						break
					}
					if msg.Message == wmHotkey && msg.WParam == 1 {
						go onTrigger()
					}
				}
			}
		}
	}()
}
