package main

import (
	"archive/tar"
	"archive/zip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/klauspost/compress/zstd"
	"github.com/shusfun/mistral-gui/internal/update"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	dir := flag.String("dir", "dist", "artifact directory")
	version := flag.String("version", "", "release tag")
	repo := flag.String("repository", "", "owner/repository")
	goos := flag.String("os", "", "package one target")
	arch := flag.String("arch", "", "amd64 or arm64")
	flag.Parse()
	var e error
	if *goos != "" {
		e = pack(*dir, *goos, *arch)
	} else {
		e = manifest(*dir, *version, *repo)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func pack(dir, goos, arch string) error {
	label := "x64"
	if arch == "arm64" {
		label = "arm64"
	} else if arch != "amd64" {
		return fmt.Errorf("unsupported arch")
	}
	if goos == "windows" && arch == "amd64" {
		f, e := os.Create(filepath.Join(dir, "mistral-gui-windows-x64.update.zip"))
		if e != nil {
			return e
		}
		defer f.Close()
		w := zip.NewWriter(f)
		for _, name := range []string{"mistral-gui.exe", "mistral-gui-updater.exe"} {
			b, e := os.ReadFile(filepath.Join(dir, name))
			if e != nil {
				return e
			}
			out, e := w.Create(name)
			if e != nil {
				return e
			}
			if _, e = out.Write(b); e != nil {
				return e
			}
		}
		return w.Close()
	}
	if goos != "darwin" {
		return fmt.Errorf("unsupported platform")
	}
	f, e := os.Create(filepath.Join(dir, "mistral-gui-macos-"+label+".update.tar.zst"))
	if e != nil {
		return e
	}
	defer f.Close()
	z, e := zstd.NewWriter(f)
	if e != nil {
		return e
	}
	defer z.Close()
	w := tar.NewWriter(z)
	for _, name := range []string{"mistral-gui.app", "mistral-gui-updater"} {
		e = filepath.Walk(filepath.Join(dir, name), func(path string, info os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() && !info.IsDir() {
				return fmt.Errorf("unsupported link: %s", path)
			}
			h, e := tar.FileInfoHeader(info, "")
			if e != nil {
				return e
			}
			h.Name, _ = filepath.Rel(dir, path)
			h.Name = filepath.ToSlash(h.Name)
			if e = w.WriteHeader(h); e != nil {
				return e
			}
			if info.IsDir() {
				return nil
			}
			in, e := os.Open(path)
			if e != nil {
				return e
			}
			defer in.Close()
			_, e = io.Copy(w, in)
			return e
		})
		if e != nil {
			return e
		}
	}
	return w.Close()
}
func manifest(dir, version, repo string) error {
	if version == "" || strings.ContainsAny(version, "/\\:") || len(strings.Split(repo, "/")) != 2 {
		return fmt.Errorf("version and repository required")
	}
	key, e := base64.StdEncoding.DecodeString(os.Getenv("UPDATE_SIGNING_KEY"))
	if e != nil || len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("UPDATE_SIGNING_KEY must be a base64 Ed25519 private key")
	}
	public := base64.StdEncoding.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey))
	if public != os.Getenv("UPDATE_PUBLIC_KEY") {
		return fmt.Errorf("UPDATE_PUBLIC_KEY does not match signing key")
	}
	m := update.Manifest{Version: version}
	var sums []string
	targets := []struct{ os, arch, install, asset string }{
		{"windows", "amd64", "mistral-gui-windows-x64-setup.exe", "mistral-gui-windows-x64.update.zip"},
		{"darwin", "amd64", "mistral-gui-macos-x64.dmg", "mistral-gui-macos-x64.update.tar.zst"},
		{"darwin", "arm64", "mistral-gui-macos-arm64.dmg", "mistral-gui-macos-arm64.update.tar.zst"},
	}
	for _, t := range targets {
		for _, name := range []string{t.install, t.asset} {
			f, e := os.Open(filepath.Join(dir, name))
			if e != nil {
				return e
			}
			h := sha256.New()
			n, e := io.Copy(h, f)
			f.Close()
			if e != nil {
				return e
			}
			sum := hex.EncodeToString(h.Sum(nil))
			sums = append(sums, sum+"  "+name)
			if name == t.asset {
				m.Assets = append(m.Assets, update.Asset{OS: t.os, Arch: t.arch, Kind: "update", URL: "https://github.com/" + repo + "/releases/download/" + version + "/" + name, SHA256: sum, Size: n})
			}
		}
	}
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	sort.Strings(sums)
	for name, data := range map[string][]byte{"release-manifest.json": append(b, '\n'), "SHA256SUMS": []byte(strings.Join(sums, "\n") + "\n")} {
		if e = os.WriteFile(filepath.Join(dir, name), data, 0644); e != nil {
			return e
		}
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, data)) + "\n"
		if e = os.WriteFile(filepath.Join(dir, name+".sig"), []byte(sig), 0644); e != nil {
			return e
		}
	}
	return nil
}
