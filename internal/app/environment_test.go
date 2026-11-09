package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEngineEnvironmentPreservesDriverSystemPathsWithoutCredentials(t *testing.T) {
	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("ProgramW6432", `C:\Program Files`)
	t.Setenv("UNRELATED_API_KEY", "must-not-reach-engine")
	t.Setenv("HF_TOKEN", "must-not-reach-engine")
	dir := t.TempDir()
	values := map[string]string{}
	for _, entry := range engineEnvironment("cuda", dir) {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
		if strings.Contains(value, "must-not-reach-engine") {
			t.Fatal("unrelated credential inherited")
		}
	}
	if values["ProgramFiles"] != `C:\Program Files` || values["ProgramW6432"] != `C:\Program Files` || values["HF_HOME"] != filepath.Join(dir, "hf-cache") {
		t.Fatal("required driver or model cache environment missing")
	}
}
