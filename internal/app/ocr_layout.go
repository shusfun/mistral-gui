package app

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"sort"
	"strings"
)

type OCRParagraph struct {
	Merged string
	Raw    string
	Box    *[4]float64
}

type ocrLineGeom struct {
	text   string
	x0, y0 float64
	x1, y1 float64
	w, h   float64
	cx, cy float64
	hasBox bool
}

func decodeImageDimensions(imgBytes []byte) (int, int) {
	if len(imgBytes) == 0 {
		return 0, 0
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(imgBytes))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func toLineGeom(ln OCRLine) (ocrLineGeom, bool) {
	txt := strings.TrimSpace(ln.Text)
	if txt == "" {
		return ocrLineGeom{}, false
	}
	if len(ln.Box) < 2 {
		return ocrLineGeom{text: txt, hasBox: false}, true
	}
	x0, y0 := ln.Box[0][0], ln.Box[0][1]
	x1, y1 := x0, y0
	for _, pt := range ln.Box[1:] {
		if pt[0] < x0 {
			x0 = pt[0]
		}
		if pt[0] > x1 {
			x1 = pt[0]
		}
		if pt[1] < y0 {
			y0 = pt[1]
		}
		if pt[1] > y1 {
			y1 = pt[1]
		}
	}
	w := math.Max(1, x1-x0)
	h := math.Max(1, y1-y0)
	return ocrLineGeom{
		text:   txt,
		x0:     x0,
		y0:     y0,
		x1:     x1,
		y1:     y1,
		w:      w,
		h:      h,
		cx:     (x0 + x1) / 2,
		cy:     (y0 + y1) / 2,
		hasBox: true,
	}, true
}

func medianFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

func percentileFloat(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	idx := int(math.Round(p * float64(len(cp)-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

// mergeSameVisualRow combines OCR boxes that sit on the same horizontal text line within a column.
func mergeSameVisualRow(lines []ocrLineGeom) []ocrLineGeom {
	if len(lines) <= 1 {
		return lines
	}
	sorted := append([]ocrLineGeom(nil), lines...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if math.Abs(sorted[i].cy-sorted[j].cy) <= 0.45*math.Min(sorted[i].h, sorted[j].h) {
			return sorted[i].x0 < sorted[j].x0
		}
		return sorted[i].cy < sorted[j].cy
	})

	var rows [][]ocrLineGeom
	for _, item := range sorted {
		placed := false
		if len(rows) > 0 {
			lastRow := rows[len(rows)-1]
			ref := lastRow[len(lastRow)-1]
			overlap := math.Min(ref.y1, item.y1) - math.Max(ref.y0, item.y0)
			if overlap > 0.5*math.Min(ref.h, item.h) {
				rows[len(rows)-1] = append(lastRow, item)
				placed = true
			}
		}
		if !placed {
			rows = append(rows, []ocrLineGeom{item})
		}
	}

	merged := make([]ocrLineGeom, 0, len(rows))
	for _, row := range rows {
		sort.SliceStable(row, func(i, j int) bool { return row[i].x0 < row[j].x0 })
		acc := row[0]
		for _, nxt := range row[1:] {
			acc.text = MergeTwoLines(acc.text, nxt.text)
			if nxt.x0 < acc.x0 {
				acc.x0 = nxt.x0
			}
			if nxt.y0 < acc.y0 {
				acc.y0 = nxt.y0
			}
			if nxt.x1 > acc.x1 {
				acc.x1 = nxt.x1
			}
			if nxt.y1 > acc.y1 {
				acc.y1 = nxt.y1
			}
			acc.w = math.Max(1, acc.x1-acc.x0)
			acc.h = math.Max(1, acc.y1-acc.y0)
			acc.cx = (acc.x0 + acc.x1) / 2
			acc.cy = (acc.y0 + acc.y1) / 2
		}
		merged = append(merged, acc)
	}
	return merged
}

// splitBandIntoColumns partitions a vertical band of non-spanning lines into left-to-right columns.
func splitBandIntoColumns(band []ocrLineGeom, medH float64) [][]ocrLineGeom {
	if len(band) <= 1 {
		return [][]ocrLineGeom{band}
	}
	type xInterval struct {
		x0, x1 float64
	}
	intervals := make([]xInterval, len(band))
	for i, ln := range band {
		intervals[i] = xInterval{x0: ln.x0, x1: ln.x1}
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].x0 < intervals[j].x0 })

	minGutter := math.Max(18, 2.0*medH)
	mergedCols := []xInterval{intervals[0]}
	for _, iv := range intervals[1:] {
		cur := &mergedCols[len(mergedCols)-1]
		if iv.x0-cur.x1 < minGutter {
			if iv.x1 > cur.x1 {
				cur.x1 = iv.x1
			}
		} else {
			mergedCols = append(mergedCols, iv)
		}
	}
	if len(mergedCols) == 1 {
		return [][]ocrLineGeom{band}
	}

	cols := make([][]ocrLineGeom, len(mergedCols))
	for _, ln := range band {
		bestIdx := 0
		bestDist := math.MaxFloat64
		for idx, c := range mergedCols {
			if ln.cx >= c.x0 && ln.cx <= c.x1 {
				bestIdx = idx
				bestDist = 0
				break
			}
			dist := math.Min(math.Abs(ln.cx-c.x0), math.Abs(ln.cx-c.x1))
			if dist < bestDist {
				bestDist = dist
				bestIdx = idx
			}
		}
		cols[bestIdx] = append(cols[bestIdx], ln)
	}

	nonEmpty := make([][]ocrLineGeom, 0, len(cols))
	for _, c := range cols {
		if len(c) > 0 {
			nonEmpty = append(nonEmpty, c)
		}
	}
	return nonEmpty
}

func normalizeUnionBox(group []ocrLineGeom, canvasW, canvasH float64) *[4]float64 {
	if len(group) == 0 || canvasW <= 0 || canvasH <= 0 {
		return nil
	}
	hasAny := false
	var x0, y0, x1, y1 float64
	for _, g := range group {
		if !g.hasBox {
			continue
		}
		if !hasAny {
			x0, y0, x1, y1 = g.x0, g.y0, g.x1, g.y1
			hasAny = true
			continue
		}
		if g.x0 < x0 {
			x0 = g.x0
		}
		if g.y0 < y0 {
			y0 = g.y0
		}
		if g.x1 > x1 {
			x1 = g.x1
		}
		if g.y1 > y1 {
			y1 = g.y1
		}
	}
	if !hasAny {
		return nil
	}
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return math.Round(v*10000) / 10000
	}
	box := [4]float64{
		clamp(x0 / canvasW),
		clamp(y0 / canvasH),
		clamp(x1 / canvasW),
		clamp(y1 / canvasH),
	}
	return &box
}

func segmentColumnBlocks(colLines []ocrLineGeom) [][]ocrLineGeom {
	rows := mergeSameVisualRow(colLines)
	if len(rows) <= 1 {
		if len(rows) == 0 {
			return nil
		}
		return [][]ocrLineGeom{rows}
	}

	heights := make([]float64, len(rows))
	lefts := make([]float64, len(rows))
	rights := make([]float64, len(rows))
	for i, r := range rows {
		heights[i] = r.h
		lefts[i] = r.x0
		rights[i] = r.x1
	}
	medH := medianFloat(heights)
	colLeft := percentileFloat(lefts, 0.15)
	colRight := percentileFloat(rights, 0.85)
	colSpan := math.Max(1, colRight-colLeft)

	var intraGaps []float64
	for i := 1; i < len(rows); i++ {
		gap := rows[i].y0 - rows[i-1].y1
		if gap >= -0.3*medH && gap <= 1.2*medH {
			intraGaps = append(intraGaps, math.Max(0, gap))
		}
	}
	baseGap := 0.35 * medH
	if len(intraGaps) > 0 {
		baseGap = medianFloat(intraGaps)
	}

	var blocks [][]ocrLineGeom
	cur := []ocrLineGeom{rows[0]}

	for i := 1; i < len(rows); i++ {
		prev := rows[i-1]
		nxt := rows[i]
		gap := nxt.y0 - prev.y1

		gapJump := gap > math.Max(0.75*medH, baseGap*1.85+0.15*medH)
		firstLineIndent := (nxt.x0-colLeft) >= 1.15*medH && (nxt.x0-prev.x0) >= 0.9*medH

		isUnindentAfterFirstLine := len(cur) == 1 && (prev.x0-colLeft) >= 1.15*medH && math.Abs(nxt.x0-colLeft) < 0.85*medH
		leftEdgeShift := !isUnindentAfterFirstLine && !firstLineIndent && math.Abs(nxt.x0-prev.x0) >= 1.5*medH

		prevEndedShort := colSpan > 5.5*medH && (colRight-prev.x1) > math.Max(2.8*medH, 0.25*colSpan)

		if gapJump || firstLineIndent || leftEdgeShift || prevEndedShort {
			blocks = append(blocks, cur)
			cur = []ocrLineGeom{nxt}
		} else {
			cur = append(cur, nxt)
		}
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}
	return blocks
}

// SegmentOCRLines groups OCR lines by column and reading order, segments paragraphs by layout cues,
// and applies the shared intra-paragraph reconstruction rules.
// Returns reconstructed paragraphs, reconstructed text, raw reading-order text, and raw line count.
func SegmentOCRLines(lines []OCRLine, imgWidth, imgHeight int, preserveLineBreaks bool) ([]OCRParagraph, string, string, int) {
	geoms := make([]ocrLineGeom, 0, len(lines))
	hasBoxes := true
	for _, ln := range lines {
		g, ok := toLineGeom(ln)
		if !ok {
			continue
		}
		if !g.hasBox {
			hasBoxes = false
		}
		geoms = append(geoms, g)
	}
	if len(geoms) == 0 {
		return nil, "", "", 0
	}

	// Fallback when OCR lines have no coordinates (e.g. legacy mock tests)
	if !hasBoxes {
		rawParts := make([]string, len(geoms))
		for i, g := range geoms {
			rawParts[i] = g.text
		}
		rawJoined := strings.Join(rawParts, "\n")
		rebuilt := RebuildParagraphs(rawJoined, preserveLineBreaks)
		out := make([]OCRParagraph, len(rebuilt))
		mergedParts := make([]string, len(rebuilt))
		for i, rp := range rebuilt {
			out[i] = OCRParagraph{Merged: rp.Merged, Raw: rp.Raw}
			mergedParts[i] = rp.Merged
		}
		sep := "\n\n"
		if preserveLineBreaks {
			sep = "\n"
		}
		return out, strings.Join(mergedParts, sep), rawJoined, len(geoms)
	}

	// Determine canvas dimensions (exact decoded image dimensions, or tight bounding canvas fallback)
	canvasW := float64(imgWidth)
	canvasH := float64(imgHeight)
	for _, g := range geoms {
		if g.x1 > canvasW {
			canvasW = g.x1
		}
		if g.y1 > canvasH {
			canvasH = g.y1
		}
	}

	// Sort preliminarily top-to-bottom
	sort.SliceStable(geoms, func(i, j int) bool {
		if math.Abs(geoms[i].y0-geoms[j].y0) < 2 {
			return geoms[i].x0 < geoms[j].x0
		}
		return geoms[i].y0 < geoms[j].y0
	})

	minX, maxX := geoms[0].x0, geoms[0].x1
	heights := make([]float64, len(geoms))
	for i, g := range geoms {
		if g.x0 < minX {
			minX = g.x0
		}
		if g.x1 > maxX {
			maxX = g.x1
		}
		heights[i] = g.h
	}
	totalSpan := math.Max(1, maxX-minX)
	midX := (minX + maxX) / 2
	medH := medianFloat(heights)

	isSpanning := func(g ocrLineGeom) bool {
		if len(geoms) < 4 || totalSpan < 8*medH {
			return false
		}
		return g.w > 0.68*totalSpan && g.x0 < midX-0.12*totalSpan && g.x1 > midX+0.12*totalSpan
	}

	// Partition into spanning banners and multi-column bands
	var orderedColumns [][]ocrLineGeom
	var currentBand []ocrLineGeom
	flushBand := func() {
		if len(currentBand) == 0 {
			return
		}
		cols := splitBandIntoColumns(currentBand, medH)
		orderedColumns = append(orderedColumns, cols...)
		currentBand = nil
	}

	for _, g := range geoms {
		if isSpanning(g) {
			flushBand()
			orderedColumns = append(orderedColumns, []ocrLineGeom{g})
		} else {
			currentBand = append(currentBand, g)
		}
	}
	flushBand()

	var out []OCRParagraph
	var rawAllLines []string
	var mergedAll []string

	for _, col := range orderedColumns {
		if preserveLineBreaks {
			rows := mergeSameVisualRow(col)
			for _, r := range rows {
				rawAllLines = append(rawAllLines, r.text)
				mergedAll = append(mergedAll, r.text)
				out = append(out, OCRParagraph{
					Merged: r.text,
					Raw:    r.text,
					Box:    normalizeUnionBox([]ocrLineGeom{r}, canvasW, canvasH),
				})
			}
			continue
		}

		blocks := segmentColumnBlocks(col)
		for _, blk := range blocks {
			blkLines := make([]string, len(blk))
			for i, l := range blk {
				blkLines[i] = l.text
				rawAllLines = append(rawAllLines, l.text)
			}
			blkRaw := strings.Join(blkLines, "\n")
			subRebuilt := RebuildParagraphs(blkRaw, false)
			if len(subRebuilt) == 1 {
				out = append(out, OCRParagraph{
					Merged: subRebuilt[0].Merged,
					Raw:    subRebuilt[0].Raw,
					Box:    normalizeUnionBox(blk, canvasW, canvasH),
				})
				mergedAll = append(mergedAll, subRebuilt[0].Merged)
				continue
			}
			// Map sub-paragraphs back to their line slices inside blk for accurate per-paragraph boxes
			lineCursor := 0
			for _, sp := range subRebuilt {
				cnt := strings.Count(sp.Raw, "\n") + 1
				end := lineCursor + cnt
				if end > len(blk) {
					end = len(blk)
				}
				subSlice := blk[lineCursor:end]
				lineCursor = end
				out = append(out, OCRParagraph{
					Merged: sp.Merged,
					Raw:    sp.Raw,
					Box:    normalizeUnionBox(subSlice, canvasW, canvasH),
				})
				mergedAll = append(mergedAll, sp.Merged)
			}
		}
	}

	sep := "\n\n"
	if preserveLineBreaks {
		sep = "\n"
	}
	return out, strings.Join(mergedAll, sep), strings.Join(rawAllLines, "\n"), len(rawAllLines)
}
