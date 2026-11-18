package app

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

//go:embed rules/translation-rules.json
var embeddedTranslationRules []byte

type PolicySpec struct {
	ID              string `json:"id"`
	TemplateVersion int    `json:"templateVersion"`
}

type TargetLanguageRule struct {
	Code        string `json:"code"`
	ZhLabel     string `json:"zhLabel"`
	EnLabel     string `json:"enLabel"`
	HunyuanName string `json:"hunyuanName"`
	PromptName  string `json:"promptName"`
}

type TranslationRules struct {
	Version                int    `json:"version"`
	SingleTextModelPattern string `json:"singleTextModelPattern"`
	SplitLimit             int    `json:"splitLimit"`
	PlaceholderPattern     string `json:"placeholderPattern"`
	Policies               struct {
		Default   PolicySpec `json:"default"`
		HunyuanMt PolicySpec `json:"hunyuanMt"`
	} `json:"policies"`
	TargetLanguages []TargetLanguageRule `json:"targetLanguages"`
	Templates       struct {
		HunyuanSingle string `json:"hunyuanSingle"`
		DefaultSingle string `json:"defaultSingle"`
		DefaultBatch  string `json:"defaultBatch"`
	} `json:"templates"`
}

var (
	rulesOnce     sync.Once
	parsedRules   TranslationRules
	singleModelRe *regexp.Regexp
	placeholderRe *regexp.Regexp
	rulesLoadErr  error
)

func LoadTranslationRules() (*TranslationRules, error) {
	rulesOnce.Do(func() {
		if err := json.Unmarshal(embeddedTranslationRules, &parsedRules); err != nil {
			rulesLoadErr = err
			return
		}
		singleModelRe, rulesLoadErr = regexp.Compile("(?i)" + parsedRules.SingleTextModelPattern)
		if rulesLoadErr != nil {
			return
		}
		placeholderRe, rulesLoadErr = regexp.Compile(parsedRules.PlaceholderPattern)
	})
	if rulesLoadErr != nil {
		return nil, rulesLoadErr
	}
	return &parsedRules, nil
}

func SingleTextModel(model string) bool {
	if _, err := LoadTranslationRules(); err != nil || singleModelRe == nil {
		return false
	}
	return singleModelRe.MatchString(model)
}

func ResolveTargetLanguageRule(code string) TargetLanguageRule {
	r, err := LoadTranslationRules()
	if err != nil || len(r.TargetLanguages) == 0 {
		return TargetLanguageRule{Code: "zh-CN", ZhLabel: "简体中文", EnLabel: "Simplified Chinese", HunyuanName: "Chinese", PromptName: "简体中文"}
	}
	for _, lang := range r.TargetLanguages {
		if lang.Code == code {
			return lang
		}
	}
	return r.TargetLanguages[0]
}

func TranslationPolicyID(model string) string {
	r, err := LoadTranslationRules()
	if err != nil {
		return "qingyi-paragraph-v3"
	}
	spec := r.Policies.Default
	if SingleTextModel(model) {
		spec = r.Policies.HunyuanMt
	}
	if spec.TemplateVersion > 1 {
		return fmt.Sprintf("%s-tv%d", spec.ID, spec.TemplateVersion)
	}
	return spec.ID
}

func BuildTranslationPrompt(model string, sources []string, targetLang string) (string, error) {
	r, err := LoadTranslationRules()
	if err != nil {
		return "", err
	}
	if len(sources) == 0 {
		return "", errors.New("empty_source")
	}
	lang := ResolveTargetLanguageRule(targetLang)
	if SingleTextModel(model) {
		if len(sources) != 1 {
			return "", errors.New("single_text_model")
		}
		out := strings.Replace(r.Templates.HunyuanSingle, "{{hunyuanName}}", lang.HunyuanName, 1)
		out = strings.Replace(out, "{{source}}", sources[0], 1)
		return out, nil
	}
	if len(sources) > 1 {
		type item struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		items := make([]item, len(sources))
		for i, s := range sources {
			items[i] = item{ID: fmt.Sprintf("t%d", i), Text: s}
		}
		encoded, err := json.Marshal(items)
		if err != nil {
			return "", err
		}
		out := strings.Replace(r.Templates.DefaultBatch, "{{promptName}}", lang.PromptName, 1)
		out = strings.Replace(out, "{{jsonSources}}", string(encoded), 1)
		return out, nil
	}
	out := strings.Replace(r.Templates.DefaultSingle, "{{promptName}}", lang.PromptName, 1)
	out = strings.Replace(out, "{{source}}", sources[0], 1)
	return out, nil
}

func ExtractPlaceholders(s string) []string {
	if _, err := LoadTranslationRules(); err != nil || placeholderRe == nil {
		return nil
	}
	matches := placeholderRe.FindAllString(s, -1)
	if matches == nil {
		return []string{}
	}
	return matches
}

func ValidTranslatedSegment(source, value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	maxLen := utf8.RuneCountInString(source) * 12
	if maxLen < 4000 {
		maxLen = 4000
	}
	if utf8.RuneCountInString(value) >= maxLen {
		return false
	}
	srcPh := ExtractPlaceholders(source)
	valPh := ExtractPlaceholders(value)
	if len(srcPh) != len(valPh) {
		return false
	}
	for i := range srcPh {
		if srcPh[i] != valPh[i] {
			return false
		}
	}
	return true
}

func NormalizeRuleLang(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "auto"
	}
	if trimmed == "*" || strings.EqualFold(trimmed, "auto") {
		if trimmed == "*" {
			return "*"
		}
		return "auto"
	}
	lower := strings.ToLower(strings.ReplaceAll(trimmed, "_", "-"))
	if strings.HasPrefix(lower, "zh") {
		if strings.Contains(lower, "tw") || strings.Contains(lower, "hk") || strings.Contains(lower, "mo") || strings.Contains(lower, "hant") {
			return "zh-TW"
		}
		return "zh-CN"
	}
	parts := strings.SplitN(lower, "-", 2)
	return parts[0]
}
