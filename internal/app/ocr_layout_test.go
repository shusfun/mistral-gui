package app

import (
	"testing"
)

func makeRectBox(x0, y0, x1, y1 float64) [][2]float64 {
	return [][2]float64{
		{x0, y0},
		{x1, y0},
		{x1, y1},
		{x0, y1},
	}
}

func TestSegmentOCRLinesTwoColumnAndLayoutCues(t *testing.T) {
	// Simulated 1000x800 two-column paper page:
	// - Spanning banner title across x=120..880
	// - Left column (x=80..450) with 2 paragraphs separated by vertical gap + first-line indent
	// - Right column (x=540..910) interleaved in raw Y coordinates so naive top-to-bottom order would mix columns!
	lines := []OCRLine{
		{Text: "Unified Offline Document Translation Architecture", Score: 0.99, Box: makeRectBox(120, 40, 880, 72)},
		// Interleaved Y order between left & right columns:
		{Text: "Left column paragraph one introduces paralleliza-", Score: 0.98, Box: makeRectBox(80, 120, 448, 142)},
		{Text: "Right column paragraph starts with memory", Score: 0.98, Box: makeRectBox(540, 122, 908, 144)},
		{Text: "tion across multi-line PDF screenshots.", Score: 0.98, Box: makeRectBox(80, 148, 390, 170)},
		{Text: "budgeting for local GGUF models.", Score: 0.98, Box: makeRectBox(540, 150, 820, 172)},
		// Second paragraph in left column (indented + larger vertical gap)
		{Text: "Second left paragraph begins after a gap", Score: 0.97, Box: makeRectBox(112, 210, 448, 232)},
		{Text: "and continues back at the left margin.", Score: 0.97, Box: makeRectBox(80, 238, 410, 260)},
	}

	paras, mergedTxt, _, rawCount := SegmentOCRLines(lines, 1000, 800, false)
	if rawCount != 7 {
		t.Fatalf("expected 7 raw lines, got %d", rawCount)
	}
	if len(paras) != 4 {
		t.Fatalf("expected 4 reconstructed paragraphs (title + 2 left + 1 right), got %d: %+v", len(paras), paras)
	}

	wantMerged := []string{
		"Unified Offline Document Translation Architecture",
		"Left column paragraph one introduces parallelization across multi-line PDF screenshots.",
		"Second left paragraph begins after a gap and continues back at the left margin.",
		"Right column paragraph starts with memory budgeting for local GGUF models.",
	}
	for i, w := range wantMerged {
		if paras[i].Merged != w {
			t.Fatalf("paragraph %d merged mismatch:\ngot:  %q\nwant: %q", i, paras[i].Merged, w)
		}
		if paras[i].Box == nil {
			t.Fatalf("paragraph %d expected non-nil normalized bounding box", i)
		}
	}
	if paras[1].Raw != "Left column paragraph one introduces paralleliza-\ntion across multi-line PDF screenshots." {
		t.Fatalf("expected raw line breaks preserved in Raw, got %q", paras[1].Raw)
	}
	if mergedTxt == "" {
		t.Fatal("expected non-empty merged OCR text")
	}

	// Verify preserveLineBreaks mode keeps all 7 reading-order lines unmerged
	lineModeParas, _, _, _ := SegmentOCRLines(lines, 1000, 800, true)
	if len(lineModeParas) != 7 {
		t.Fatalf("expected 7 lines in preserveLineBreaks mode, got %d", len(lineModeParas))
	}
}
