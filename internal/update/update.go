package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Asset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Kind   string `json:"kind"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Manifest struct {
	Version string  `json:"version"`
	Notes   string  `json:"notes"`
	Assets  []Asset `json:"assets"`
}

func IsNewer(current, next string) bool {
	parse := func(s string) ([3]int, string, bool) {
		var parts [3]int
		s = strings.TrimPrefix(s, "v")
		v, pre, _ := strings.Cut(s, "-")
		items := strings.Split(v, ".")
		if len(items) != 3 {
			return parts, pre, false
		}
		for i, x := range items {
			n, e := strconv.Atoi(x)
			if e != nil || n < 0 {
				return parts, pre, false
			}
			parts[i] = n
		}
		return parts, pre, true
	}
	c, cp, ok := parse(current)
	n, np, nok := parse(next)
	if !ok || !nok {
		return false
	}
	for i := range c {
		if n[i] != c[i] {
			return n[i] > c[i]
		}
	}
	return cp != "" && np == ""
}

func Verify(path, want string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	if hex.EncodeToString(h.Sum(nil)) != strings.TrimPrefix(strings.ToLower(want), "sha256:") {
		return errors.New("digest_mismatch")
	}
	return nil
}
func FetchManifest(ctx context.Context, client *http.Client, raw string) (Manifest, error) {
	b, e := get(ctx, client, raw, 4<<20)
	if e != nil {
		return Manifest{}, e
	}
	var m Manifest
	e = json.Unmarshal(b, &m)
	if e == nil && m.Version == "" {
		e = errors.New("manifest_invalid")
	}
	return m, e
}
func FetchSigned(ctx context.Context, client *http.Client, raw, key string) (Manifest, error) {
	if !strings.HasPrefix(raw, "https://") {
		return Manifest{}, errors.New("https_required")
	}
	pub, e := base64.StdEncoding.DecodeString(key)
	if e != nil || len(pub) != ed25519.PublicKeySize {
		return Manifest{}, errors.New("update_key_unconfigured")
	}
	b, e := get(ctx, client, raw, 4<<20)
	if e != nil {
		return Manifest{}, e
	}
	sig, e := get(ctx, client, raw+".sig", 4096)
	if e != nil {
		return Manifest{}, e
	}
	signature, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if e != nil || !ed25519.Verify(pub, b, signature) {
		return Manifest{}, errors.New("update_signature_invalid")
	}
	var m Manifest
	if e = json.Unmarshal(b, &m); e != nil {
		return m, e
	}
	if m.Version == "" || strings.ContainsAny(m.Version, "/\\:") {
		return m, errors.New("version_invalid")
	}
	for _, a := range m.Assets {
		if !strings.HasPrefix(a.URL, "https://github.com/") || len(a.SHA256) != 64 || a.Size <= 0 {
			return m, errors.New("update_asset_invalid")
		}
	}
	return m, nil
}
func get(ctx context.Context, c *http.Client, raw string, n int64) ([]byte, error) {
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return nil, e
	}
	resp, e := c.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("metadata_http_%d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, n+1))
	if int64(len(b)) > n {
		return nil, errors.New("metadata_too_large")
	}
	return b, e
}
func Stage(path, version, root string) (string, error) {
	if version == "" || strings.ContainsAny(version, "/\\:") {
		return "", errors.New("version_invalid")
	}
	dir := filepath.Join(root, "versions", version)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	dest := filepath.Join(dir, filepath.Base(path))
	return dest, os.Rename(path, dest)
}
