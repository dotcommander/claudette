package index

import (
	"testing"
)

func TestBuildSuggestionSuppressSet_PopulatesFromKeywords(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		{Name: "code-clean-code", Keywords: map[string]int{"solid": 2, "dry": 1, "refactor": 1}},
		{Name: "lang-go-dev", Keywords: map[string]int{"golang": 3, "goroutine": 1}},
	}
	set := buildSuggestionSuppressSet(entries)

	if len(set) != 2 {
		t.Fatalf("expected 2 entries in set, got %d", len(set))
	}

	// code-clean-code should have solid, dry, refactor suppressed.
	for _, kw := range []string{"solid", "dry", "refactor"} {
		if !set["code-clean-code"][kw] {
			t.Errorf("code-clean-code: expected %q to be suppressed", kw)
		}
	}

	// lang-go-dev should have golang, goroutine suppressed.
	for _, kw := range []string{"golang", "goroutine"} {
		if !set["lang-go-dev"][kw] {
			t.Errorf("lang-go-dev: expected %q to be suppressed", kw)
		}
	}
}

func TestBuildSuggestionSuppressSet_Lowercases(t *testing.T) {
	t.Parallel()
	// Keywords stored as mixed-case must appear lowercased in the suppress set.
	entries := []Entry{
		{Name: "entry-a", Keywords: map[string]int{"SOLID": 2, "DRY": 1}},
	}
	set := buildSuggestionSuppressSet(entries)

	if !set["entry-a"]["solid"] {
		t.Error("expected 'solid' (lowercased) to be suppressed")
	}
	if !set["entry-a"]["dry"] {
		t.Error("expected 'dry' (lowercased) to be suppressed")
	}
	// Original case must not appear — only lowercased.
	if set["entry-a"]["SOLID"] {
		t.Error("expected 'SOLID' (original case) NOT to be suppressed directly")
	}
}

func TestBuildSuggestionSuppressSet_EmptyEntries(t *testing.T) {
	t.Parallel()
	set := buildSuggestionSuppressSet(nil)
	if len(set) != 0 {
		t.Errorf("expected empty set for nil entries, got %v", set)
	}
}

func TestBuildSuggestionSuppressSet_ZeroKeywordEntry(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		{Name: "entry-no-kw", Keywords: nil},
	}
	set := buildSuggestionSuppressSet(entries)
	// Entry still gets an (empty) set — doesn't panic.
	if set["entry-no-kw"] == nil {
		t.Error("expected empty map (not nil) for zero-keyword entry")
	}
	if len(set["entry-no-kw"]) != 0 {
		t.Errorf("expected 0 suppressed keywords, got %d", len(set["entry-no-kw"]))
	}
}
