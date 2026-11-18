//go:build windows

package app

import (
	"encoding/base64"
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
	"unsafe"
)

func protectSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if strings.HasPrefix(plain, "dpapi:") {
		return plain, nil
	}
	raw := []byte(plain)
	in := windows.DataBlob{
		Size: uint32(len(raw)),
		Data: &raw[0],
	}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	enc := unsafe.Slice(out.Data, out.Size)
	copied := make([]byte, len(enc))
	copy(copied, enc)
	return "dpapi:" + base64.StdEncoding.EncodeToString(copied), nil
}

func unprotectSecret(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, "dpapi:") {
		// Transparently accept legacy plaintext secrets and re-encrypt on next save.
		return stored, nil
	}
	enc, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "dpapi:"))
	if err != nil || len(enc) == 0 {
		return "", errors.New("credential_decrypt_failed")
	}
	in := windows.DataBlob{
		Size: uint32(len(enc)),
		Data: &enc[0],
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return "", errors.New("credential_decrypt_failed")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	dec := unsafe.Slice(out.Data, out.Size)
	return string(dec), nil
}

func diskFreeBytes(dir string) (uint64, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return 0, err
	}
	ptr, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return 0, err
	}
	var freeAvail, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeAvail, &totalBytes, &totalFree); err != nil {
		return 0, err
	}
	return freeAvail, nil
}

func lockFileExclusive(path string) (func(), error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(
		ptr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	var ol windows.Overlapped
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol); err != nil {
		_ = windows.CloseHandle(h)
		return nil, errors.New("data_dir_locked")
	}
	return func() {
		var unlockOl windows.Overlapped
		_ = windows.UnlockFileEx(h, 0, 1, 0, &unlockOl)
		_ = windows.CloseHandle(h)
	}, nil
}
