//go:build windows

package desktop

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows/registry"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func hasWebView() bool {
	for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, prefix := range []string{`SOFTWARE\Microsoft\EdgeUpdate\Clients`, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients`} {
			key, e := registry.OpenKey(hive, prefix, registry.READ)
			if e != nil {
				continue
			}
			names, _ := key.ReadSubKeyNames(-1)
			key.Close()
			for _, name := range names {
				child, e := registry.OpenKey(hive, prefix+`\`+name, registry.READ)
				if e != nil {
					continue
				}
				label, _, _ := child.GetStringValue("name")
				version, _, _ := child.GetStringValue("pv")
				child.Close()
				if strings.Contains(strings.ToLower(label), "webview2") && version != "" && version != "0.0.0.0" {
					return true
				}
			}
		}
	}
	return false
}
func EnsureWebView(language string) error {
	if hasWebView() {
		return nil
	}
	message := "Microsoft Edge WebView2 Runtime is required. Download and install it from Microsoft now?"
	if language != "en-US" {
		message = "需要 Microsoft Edge WebView2 Runtime。现在从微软下载并安装吗？"
	}
	title, _ := syscall.UTF16PtrFromString("mistral-gui")
	body, _ := syscall.UTF16PtrFromString(message)
	button, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), 0x00000004|0x00000020|0x00000100)
	if button != 6 {
		return errors.New("webview2_install_cancelled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", "https://go.microsoft.com/fwlink/p/?LinkId=2124703", nil)
	if e != nil {
		return e
	}
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("webview2_download_http_%d", resp.StatusCode)
	}
	dir, e := os.MkdirTemp("", "mistral-webview2-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	installer := filepath.Join(dir, "MicrosoftEdgeWebview2Setup.exe")
	f, e := os.OpenFile(installer, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	n, e := io.Copy(f, io.LimitReader(resp.Body, 32<<20))
	f.Close()
	if e != nil {
		return e
	}
	if n >= 32<<20 {
		return errors.New("webview2_installer_too_large")
	}
	verify := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$s=Get-AuthenticodeSignature -LiteralPath $env:MISTRAL_GUI_WEBVIEW2_INSTALLER; if($s.Status -ne 'Valid' -or $s.SignerCertificate.Subject -notmatch 'O=Microsoft Corporation'){exit 1}`)
	verify.Env = append(os.Environ(), "MISTRAL_GUI_WEBVIEW2_INSTALLER="+installer)
	detach(verify)
	if e = verify.Run(); e != nil {
		return errors.New("webview2_signature_invalid")
	}
	cmd := exec.CommandContext(ctx, installer, "/silent", "/install")
	detach(cmd)
	e = cmd.Run()
	if e != nil {
		return fmt.Errorf("webview2_install_failed: %w", e)
	}
	if !hasWebView() {
		return errors.New("webview2_install_not_detected")
	}
	return nil
}
