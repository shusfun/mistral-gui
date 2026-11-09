package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"github.com/klauspost/compress/zstd"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Extract rejects links and traversal and caps unpacked bytes independently of transport size.
func Extract(src, dest string, limit int64) error {
	if err := os.MkdirAll(dest, 0700); err != nil {
		return err
	}
	var used int64
	write := func(name string, mode os.FileMode, r io.Reader, size int64) error {
		name = strings.ReplaceAll(name, "\\", "/")
		if strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
			return errors.New("unsafe_archive_path")
		}
		clean := filepath.Clean(filepath.FromSlash(name))
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return errors.New("unsafe_archive_path")
		}
		if size < 0 || size > limit-used {
			return errors.New("archive_size_limit")
		}
		p := filepath.Join(dest, clean)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm()|0600)
		if err != nil {
			return err
		}
		n, err := io.CopyN(f, r, size)
		cerr := f.Close()
		used += n
		if err != nil {
			return err
		}
		return cerr
	}
	if strings.HasSuffix(src, ".zip") {
		z, err := zip.OpenReader(src)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, f := range z.File {
			if f.Mode()&os.ModeSymlink != 0 {
				return errors.New("archive_link_rejected")
			}
			if f.FileInfo().IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			err = write(f.Name, f.Mode(), rc, int64(f.UncompressedSize64))
			rc.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader
	if strings.HasSuffix(src, ".tar.zst") {
		z, err := zstd.NewReader(f, zstd.WithDecoderMaxMemory(256<<20))
		if err != nil {
			return err
		}
		defer z.Close()
		r = z
	} else {
		z, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer z.Close()
		r = z
	}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return errors.New("archive_link_rejected")
		}
		if err = write(h.Name, os.FileMode(h.Mode), tr, h.Size); err != nil {
			return err
		}
	}
}
