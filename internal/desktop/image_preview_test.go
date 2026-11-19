package desktop

import (
	"strings"
	"testing"
)

func TestParseImagePreview(t *testing.T) {
	const translated = `{"src":"data:image/png;base64,aA==","view":"translated","theme":"dark","overlays":[{"box":[0.1,0.2,0.7,0.8],"text":"translation","sourceLines":3}]}`
	snapshot, err := parseImagePreview(translated)
	if err != nil || snapshot.View != "translated" || snapshot.Theme != "dark" || snapshot.Overlays[0].SourceLines != 3 {
		t.Fatalf("valid translated preview: %v", err)
	}
	snapshot, err = parseImagePreview(`{"src":"data:image/png;base64,aA==","view":"translated"}`)
	if err != nil || snapshot.View != "original" || snapshot.Overlays == nil || snapshot.Theme != "system" {
		t.Fatalf("valid original preview: %v", err)
	}
	for _, invalid := range []string{
		strings.Replace(translated, "data:image/png;base64,aA==", "https://example.invalid/image.png", 1),
		strings.Replace(translated, "0.7", "1.1", 1),
		strings.Replace(translated, "0.7", "0.1", 1),
		strings.Replace(translated, `"sourceLines":3`, `"sourceLines":0`, 1),
		strings.Repeat("x", maxPreviewPayload+1),
	} {
		if _, err := parseImagePreview(invalid); err == nil {
			t.Fatal("accepted an invalid preview snapshot")
		}
	}
}
