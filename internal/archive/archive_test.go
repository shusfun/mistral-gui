package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func zipFile(t *testing.T, name string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.zip")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	h := &zip.FileHeader{Name: name}
	h.SetMode(mode)
	out, e := w.CreateHeader(h)
	if e != nil {
		t.Fatal(e)
	}
	_, _ = out.Write([]byte("fixture"))
	_ = w.Close()
	_ = f.Close()
	return path
}
func TestRejectTraversalLinksAndOversize(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", "C:/outside", "..\\outside"} {
		t.Run(name, func(t *testing.T) {
			if e := Extract(zipFile(t, name, 0600), t.TempDir(), 100); e == nil {
				t.Fatal("unsafe path accepted")
			}
		})
	}
	if e := Extract(zipFile(t, "link", os.ModeSymlink|0777), t.TempDir(), 100); e == nil {
		t.Fatal("symlink accepted")
	}
	if e := Extract(zipFile(t, "large", 0600), t.TempDir(), 2); e == nil {
		t.Fatal("oversize accepted")
	}
	dest := t.TempDir()
	if e := Extract(zipFile(t, "bin/engine", 0700), dest, 100); e != nil {
		t.Fatal(e)
	}
	if b, e := os.ReadFile(filepath.Join(dest, "bin", "engine")); e != nil || string(b) != "fixture" {
		t.Fatal("bad extraction")
	}
}
