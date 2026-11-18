//go:build windows

package branding

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

func TestShortcutMigrationPreservesTargetsAndRejectsConflicts(t *testing.T) {
	for _, mode := range []string{"owned", "foreign", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			if e := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); e != nil {
				t.Fatal(e)
			}
			defer ole.CoUninitialize()
			u, e := oleutil.CreateObject("WScript.Shell")
			if e != nil {
				t.Fatal(e)
			}
			defer u.Release()
			shell, e := u.QueryInterface(ole.IID_IDispatch)
			if e != nil {
				t.Fatal(e)
			}
			defer shell.Release()
			dir := t.TempDir()
			old := filepath.Join(dir, "old.lnk")
			next := filepath.Join(dir, "new.lnk")
			exe := filepath.Join(dir, "mistral-gui.exe")
			target := exe
			if mode == "foreign" {
				target = filepath.Join(dir, "other.exe")
			}
			link, e := shortcut(shell, old)
			if e != nil {
				t.Fatal(e)
			}
			oleutil.PutProperty(link, "TargetPath", target)
			oleutil.PutProperty(link, "Arguments", "--no-open")
			_, e = oleutil.CallMethod(link, "Save")
			link.Release()
			if e != nil {
				t.Fatal(e)
			}
			if mode == "conflict" {
				l, e := shortcut(shell, next)
				if e != nil {
					t.Fatal(e)
				}
				oleutil.PutProperty(l, "TargetPath", filepath.Join(dir, "other.exe"))
				oleutil.CallMethod(l, "Save")
				l.Release()
			}
			err := migrateShortcut(shell, old, next, exe)
			if mode == "conflict" {
				if err == nil {
					t.Fatal("conflict overwritten")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "foreign" {
				if _, e := os.Stat(old); e != nil {
					t.Fatal("foreign shortcut removed")
				}
				return
			}
			if _, e := os.Stat(old); !os.IsNotExist(e) {
				t.Fatal("legacy shortcut left")
			}
			l, e := shortcut(shell, next)
			if e != nil {
				t.Fatal(e)
			}
			defer l.Release()
			args, _ := shortcutString(l, "Arguments")
			if args != "--no-open" {
				t.Fatal("arguments lost")
			}
			actual, _ := shortcutString(l, "TargetPath")
			if !samePath(actual, exe) {
				t.Fatal("target changed")
			}
			if e = migrateShortcut(shell, old, next, exe); e != nil {
				t.Fatal("not idempotent", e)
			}
		})
	}
}
