package app

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"testing"
)

//go:embed rules/paragraph-fixtures.json
var embeddedParagraphFixtures []byte

type paragraphFixture struct {
	ID                 string             `json:"id"`
	Description        string             `json:"description"`
	Input              string             `json:"input"`
	PreserveLineBreaks bool               `json:"preserveLineBreaks"`
	Expected           []RebuiltParagraph `json:"expected"`
}

func TestSharedParagraphRebuildFixtures(t *testing.T) {
	var fixtures []paragraphFixture
	if err := json.Unmarshal(embeddedParagraphFixtures, &fixtures); err != nil {
		t.Fatalf("failed to parse shared paragraph-fixtures.json: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("expected non-empty shared paragraph fixtures")
	}
	for _, fx := range fixtures {
		t.Run(fx.ID, func(t *testing.T) {
			got := RebuildParagraphs(fx.Input, fx.PreserveLineBreaks)
			if !reflect.DeepEqual(got, fx.Expected) {
				t.Fatalf("fixture %s mismatch:\ngot:  %#v\nwant: %#v", fx.ID, got, fx.Expected)
			}
		})
	}
}
