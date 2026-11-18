//go:build windows

package branding

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Migrate only touches entries owned by this registered installation.
func Migrate(version string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\mistral-gui`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	installed, _, err := k.GetStringValue("InstallDir")
	if err != nil || !samePath(exe, filepath.Join(installed, "mistral-gui.exe")) {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if code, ok := err.(*ole.OleError); !ok || code.Code() != 1 {
			return err
		}
	}
	defer ole.CoUninitialize()
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()
	desktop, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil {
		return err
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range []struct{ old, new, target string }{
		{filepath.Join(desktop, "mistral-gui.lnk"), filepath.Join(desktop, "mistral.lnk"), exe},
		{filepath.Join(programs, "mistral-gui", "mistral-gui.lnk"), filepath.Join(programs, "mistral", "mistral.lnk"), exe},
		{filepath.Join(programs, "mistral-gui", "Uninstall.lnk"), filepath.Join(programs, "mistral", "Uninstall.lnk"), filepath.Join(installed, "uninstall.exe")},
	} {
		if e := migrateShortcut(shell, entry.old, entry.new, entry.target); e != nil {
			errs = append(errs, e)
		}
	}
	_ = os.Remove(filepath.Join(programs, "mistral-gui")) // Only remove an empty legacy folder.
	uninstall, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall\mistral-gui`, registry.SET_VALUE)
	if err == nil {
		defer uninstall.Close()
		for name, value := range map[string]string{"DisplayName": "mistral", "DisplayIcon": exe + ",0", "DisplayVersion": strings.TrimPrefix(version, "v")} {
			if e := uninstall.SetStringValue(name, value); e != nil {
				errs = append(errs, e)
			}
		}
	}
	return errors.Join(errs...)
}
func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
func shortcut(shell *ole.IDispatch, path string) (*ole.IDispatch, error) {
	v, e := oleutil.CallMethod(shell, "CreateShortcut", path)
	if e != nil {
		return nil, e
	}
	return v.ToIDispatch(), nil
}
func shortcutString(link *ole.IDispatch, name string) (string, error) {
	v, e := oleutil.GetProperty(link, name)
	if e != nil {
		return "", e
	}
	defer v.Clear()
	return v.ToString(), nil
}
func migrateShortcut(shell *ole.IDispatch, old, new, target string) error {
	if _, err := os.Stat(old); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	link, err := shortcut(shell, old)
	if err != nil {
		return err
	}
	defer link.Release()
	actual, err := shortcutString(link, "TargetPath")
	if err != nil {
		return err
	}
	if !samePath(actual, target) {
		return nil
	}
	if _, err = os.Stat(new); err == nil {
		existing, e := shortcut(shell, new)
		if e != nil {
			return e
		}
		defer existing.Release()
		actual, e = shortcutString(existing, "TargetPath")
		if e != nil {
			return e
		}
		if !samePath(actual, target) {
			return errors.New("branding_shortcut_conflict")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(new), 0755); err != nil {
		return err
	}
	next, err := shortcut(shell, new)
	if err != nil {
		return err
	}
	defer next.Release()
	for _, prop := range []string{"TargetPath", "Arguments", "WorkingDirectory", "WindowStyle", "Hotkey"} {
		v, e := oleutil.GetProperty(link, prop)
		if e != nil {
			return e
		}
		_, e = oleutil.PutProperty(next, prop, v.Value())
		v.Clear()
		if e != nil {
			return e
		}
	}
	if _, err = oleutil.PutProperty(next, "IconLocation", target+",0"); err != nil {
		return err
	}
	if _, err = oleutil.PutProperty(next, "Description", "mistral"); err != nil {
		return err
	}
	if _, err = oleutil.CallMethod(next, "Save"); err != nil {
		return err
	}
	return os.Remove(old)
}
