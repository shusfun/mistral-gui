//go:build windows

package app

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	modAdvapi32        = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKeyEx = modAdvapi32.NewProc("RegCreateKeyExW")
	procRegSetValueEx  = modAdvapi32.NewProc("RegSetValueExW")
	procRegDeleteValue = modAdvapi32.NewProc("RegDeleteValueW")
	procRegCloseKey    = modAdvapi32.NewProc("RegCloseKey")
)

const (
	hkeyCurrentUser = uintptr(0x80000001)
	keyWriteAccess  = 0x20006
	regSZ           = 1
	errorFileNotFnd = 2
)

func setLaunchAtLogin(enabled bool) error {
	subKey, err := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Run`)
	if err != nil {
		return errors.New("autostart_failed")
	}
	var hKey uintptr
	var disp uint32
	ret, _, _ := procRegCreateKeyEx.Call(
		hkeyCurrentUser,
		uintptr(unsafe.Pointer(subKey)),
		0,
		0,
		0,
		keyWriteAccess,
		0,
		uintptr(unsafe.Pointer(&hKey)),
		uintptr(unsafe.Pointer(&disp)),
	)
	if ret != 0 {
		return errors.New("autostart_failed")
	}
	defer procRegCloseKey.Call(hKey)

	valName, _ := syscall.UTF16PtrFromString("mistral-gui")
	if !enabled {
		dRet, _, _ := procRegDeleteValue.Call(hKey, uintptr(unsafe.Pointer(valName)))
		if dRet != 0 && dRet != errorFileNotFnd {
			return errors.New("autostart_failed")
		}
		return nil
	}

	exePath, err := os.Executable()
	if err != nil || exePath == "" {
		return errors.New("autostart_failed")
	}
	cmdLine := fmt.Sprintf(`"%s" --tray`, exePath)
	utf16Val, err := syscall.UTF16FromString(cmdLine)
	if err != nil {
		return errors.New("autostart_failed")
	}
	sRet, _, _ := procRegSetValueEx.Call(
		hKey,
		uintptr(unsafe.Pointer(valName)),
		0,
		regSZ,
		uintptr(unsafe.Pointer(&utf16Val[0])),
		uintptr(len(utf16Val)*2),
	)
	if sRet != 0 {
		return errors.New("autostart_failed")
	}
	return nil
}
