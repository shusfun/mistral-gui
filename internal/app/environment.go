package app

import (
	"os"
	"path/filepath"
	"runtime"
)

func engineEnvironment(backend, dataDir string) []string {
	var env []string
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "SystemDrive", "ProgramFiles", "ProgramW6432", "ProgramData", "TEMP", "TMP", "HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "LANG"} {
		value := os.Getenv(key)
		if key == "PATH" && backend == "cuda" && runtime.GOOS == "windows" {
			if smi := nvidiaSMI(); filepath.IsAbs(smi) {
				value = filepath.Dir(smi) + string(os.PathListSeparator) + value
			}
		}
		if value != "" {
			env = append(env, key+"="+value)
		}
	}
	return append(env, "HF_HOME="+filepath.Join(dataDir, "hf-cache"))
}
