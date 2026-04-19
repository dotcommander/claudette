package index

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEntry_JSONRoundtrip_SuggestedAliases(t *testing.T) {
	t.Parallel()

	orig := Entry{
		Type:             TypeSkill,
		Name:             "lang-go-dev",
		Title:            "Go Development",
		Category:         "go",
		FilePath:         "skills/lang-go-dev.md",
		FileMtime:        time.Time{},
		Keywords:         map[string]int{"golang": 3, "goroutine": 2},
		SuggestedAliases: []string{"concurrent", "async"},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got Entry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(got.SuggestedAliases) != 2 {
		t.Fatalf("SuggestedAliases: want 2, got %d (%v)", len(got.SuggestedAliases), got.SuggestedAliases)
	}
	if got.SuggestedAliases[0] != "concurrent" || got.SuggestedAliases[1] != "async" {
		t.Errorf("SuggestedAliases mismatch: got %v, want [concurrent async]", got.SuggestedAliases)
	}
}

func TestEntry_JSONRoundtrip_NilSuggestedAliases_OmitEmpty(t *testing.T) {
	t.Parallel()

	orig := Entry{
		Name:     "kb-entry",
		Keywords: map[string]int{"foo": 1},
		// SuggestedAliases intentionally nil
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// omitempty: the key must not appear in the JSON output when nil.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal into map: %v", err)
	}
	if _, exists := raw["suggested_aliases"]; exists {
		t.Error("suggested_aliases key must be absent when SuggestedAliases is nil")
	}
}
