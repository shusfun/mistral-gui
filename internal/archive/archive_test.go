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

	// M-S16: corrupting stored zip payload bytes must trigger CRC32 failure at EOF
	corruptZip := filepath.Join(t.TempDir(), "corrupt.zip")
	f, e := os.Create(corruptZip)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	out, e := w.CreateHeader(&zip.FileHeader{Name: "payload.bin", Method: zip.Store})
	if e != nil {
		t.Fatal(e)
	}
	_, _ = out.Write([]byte("0123456789abcdef"))
	_ = w.Close()
	_ = f.Close()
	raw, e := os.ReadFile(corruptZip)
	if e != nil {
		t.Fatal(e)
	}
	idx := -1
	for i := 0; i <= len(raw)-16; i++ {
		if string(raw[i:i+16]) == "0123456789abcdef" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("stored payload not found")
	}
	raw[idx] ^= 0xff
	_ = os.WriteFile(corruptZip, raw, 0600)
	if e := Extract(corruptZip, t.TempDir(), 1024); e == nil {
		t.Fatal("corrupt zip CRC accepted without error")
	}
}
