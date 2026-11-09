package app

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/shusfun/mistral-gui/internal/catalog"
	"os"
	"path/filepath"
	"testing"
)

func TestModelSnapshotRejectsCorruptionAndMissingShard(t *testing.T) {
	dir := t.TempDir()
	weight := []byte("weights")
	sum := sha256.Sum256(weight)
	index := []byte(`{"weight_map":{"layer":"missing.safetensors"}}`)
	blob := sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(index))), index...))
	files := []catalog.ModelFile{{Path: "model.safetensors", SHA256: hex.EncodeToString(sum[:])}, {Path: "model.safetensors.index.json", GitSHA1: hex.EncodeToString(blob[:])}}
	_ = os.WriteFile(filepath.Join(dir, files[0].Path), weight, 0600)
	_ = os.WriteFile(filepath.Join(dir, files[1].Path), index, 0600)
	if e := verifyModelFiles(dir, files); e == nil {
		t.Fatal("missing indexed shard accepted")
	}
	_ = os.WriteFile(filepath.Join(dir, "missing.safetensors"), weight, 0600)
	if e := verifyModelFiles(dir, files); e != nil {
		t.Fatal(e)
	}
	_ = os.WriteFile(filepath.Join(dir, files[0].Path), []byte("corrupt"), 0600)
	if e := verifyModelFiles(dir, files); e == nil {
		t.Fatal("corrupt weights accepted")
	}
}
