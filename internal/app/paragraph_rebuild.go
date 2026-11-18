package app

import (
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

type RebuiltParagraph struct {
	Merged string `json:"merged"`
	Raw    string `json:"raw"`
}

var (
	rebuildOnce          sync.Once
	rebuildErr           error
	listMarkerRe         *regexp.Regexp
	headingSectionRe     *regexp.Regexp
	headingBlockedEndRe  *regexp.Regexp
	hyphenLineEndRe      *regexp.Regexp
	hyphenNextStartRe    *regexp.Regexp
	codeFenceRe          *regexp.Regexp
	codeLineRe           *regexp.Regexp
	cjkCharRe            *regexp.Regexp
	trailingAsciiHyphen  = regexp.MustCompile(`[A-Za-z]-$`)
	startsLowerAscii     = regexp.MustCompile(`^[a-z]`)
)

func ensureRebuildMatchers() (*ParagraphRebuildRule, error) {
	r, err := LoadTranslationRules()
	if err != nil {
		return nil, err
	}
	rebuildOnce.Do(func() {
		cfg := r.ParagraphRebuild
		compile := func(pat string) *regexp.Regexp {
			if rebuildErr != nil {
				return nil
			}
			var re *regexp.Regexp
			re, rebuildErr = regexp.Compile(pat)
			return re
		}
		listMarkerRe = compile(cfg.ListMarkerPattern)
		headingSectionRe = compile(cfg.HeadingSectionPattern)
		headingBlockedEndRe = compile(cfg.HeadingBlockedEndPattern)
		hyphenLineEndRe = compile(cfg.HyphenLineEndPattern)
		hyphenNextStartRe = compile(cfg.HyphenNextStartPattern)
		codeFenceRe = compile(cfg.CodeFencePattern)
		codeLineRe = compile(cfg.CodeLinePattern)
		cjkCharRe = compile(cfg.CjkCharPattern)
	})
	if rebuildErr != nil {
		return nil, rebuildErr
	}
	return &r.ParagraphRebuild, nil
}

func firstRuneStr(s string) string {
	if s == "" {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(s)
	return string(r)
}

func lastRuneStr(s string) string {
	if s == "" {
		return ""
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	return string(r)
}

func isStandaloneHeadingGo(cfg *ParagraphRebuildRule, prevTrimmed, currTrimmed string, accumulatedLines int) bool {
	if accumulatedLines > 1 {
		return false
	}
	if listMarkerRe.MatchString(prevTrimmed) {
		return false
	}
	if headingBlockedEndRe.MatchString(prevTrimmed) {
		return false
	}
	if startsLowerAscii.MatchString(currTrimmed) {
		return false
	}

	prevLen := utf8.RuneCountInString(prevTrimmed)
	if headingSectionRe.MatchString(prevTrimmed) && prevLen <= cfg.HeadingMaxChars*2 {
		return true
	}

	hasCjk := cjkCharRe.MatchString(prevTrimmed)
	maxLen := cfg.HeadingMaxChars
	shortLen := cfg.HeadingShortChars
	if hasCjk {
		maxLen = cfg.HeadingMaxCjkChars
		shortLen = cfg.HeadingShortCjkChars
	}
	if prevLen > maxLen {
		return false
	}
	if prevLen <= shortLen {
		return true
	}

	currLen := utf8.RuneCountInString(currTrimmed)
	return currLen > 0 && float64(prevLen)/float64(currLen) <= cfg.HeadingMaxRatio
}

func shouldKeepLineBreakGo(cfg *ParagraphRebuildRule, prevTrimmed, currTrimmed string, accumulatedLines int) bool {
	if codeLineRe.MatchString(prevTrimmed) || codeLineRe.MatchString(currTrimmed) {
		return true
	}
	if listMarkerRe.MatchString(currTrimmed) {
		return true
	}
	return isStandaloneHeadingGo(cfg, prevTrimmed, currTrimmed, accumulatedLines)
}

func MergeTwoLines(mergedSoFar, nextTrimmed string) string {
	if mergedSoFar == "" {
		return nextTrimmed
	}
	if nextTrimmed == "" {
		return mergedSoFar
	}
	if _, err := ensureRebuildMatchers(); err == nil {
		if hyphenLineEndRe.MatchString(mergedSoFar) && hyphenNextStartRe.MatchString(nextTrimmed) {
			return mergedSoFar[:len(mergedSoFar)-1] + nextTrimmed
		}
		if trailingAsciiHyphen.MatchString(mergedSoFar) {
			return mergedSoFar + nextTrimmed
		}
		lastCh := lastRuneStr(mergedSoFar)
		firstCh := firstRuneStr(nextTrimmed)
		if cjkCharRe.MatchString(lastCh) || cjkCharRe.MatchString(firstCh) {
			return mergedSoFar + nextTrimmed
		}
	}
	return mergedSoFar + " " + nextTrimmed
}

func RebuildParagraphs(input string, preserveLineBreaks bool) []RebuiltParagraph {
	norm := strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(norm, "\n")

	if preserveLineBreaks {
		out := make([]RebuiltParagraph, 0, len(lines))
		for _, line := range lines {
			t := strings.TrimSpace(line)
			if t != "" {
				out = append(out, RebuiltParagraph{Merged: t, Raw: t})
			}
		}
		return out
	}

	cfg, err := ensureRebuildMatchers()
	if err != nil {
		// Fallback to simple blank-line split if rules somehow fail to compile
		return RebuildParagraphs(input, true)
	}

	out := make([]RebuiltParagraph, 0)
	inCodeFence := false
	type accum struct {
		merged      string
		rawLines    []string
		prevTrimmed string
	}
	var acc *accum

	flush := func() {
		if acc == nil {
			return
		}
		out = append(out, RebuiltParagraph{
			Merged: acc.merged,
			Raw:    strings.Join(acc.rawLines, "\n"),
		})
		acc = nil
	}

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if codeFenceRe.MatchString(trimmed) {
			flush()
			out = append(out, RebuiltParagraph{Merged: trimmed, Raw: trimmed})
			inCodeFence = !inCodeFence
			continue
		}
		if trimmed == "" {
			flush()
			continue
		}
		if inCodeFence {
			flush()
			out = append(out, RebuiltParagraph{Merged: trimmed, Raw: trimmed})
			continue
		}
		if acc == nil {
			acc = &accum{merged: trimmed, rawLines: []string{trimmed}, prevTrimmed: trimmed}
			continue
		}
		if shouldKeepLineBreakGo(cfg, acc.prevTrimmed, trimmed, len(acc.rawLines)) {
			flush()
			acc = &accum{merged: trimmed, rawLines: []string{trimmed}, prevTrimmed: trimmed}
		} else {
			acc.merged = MergeTwoLines(acc.merged, trimmed)
			acc.rawLines = append(acc.rawLines, trimmed)
			acc.prevTrimmed = trimmed
		}
	}
	flush()
	return out
}
