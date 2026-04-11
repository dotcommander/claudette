package search

import (
	"testing"

	"github.com/dotcommander/claudette/internal/index"
)

func makeEntry(name, title, category string, keywords map[string]int, bigrams []string) index.Entry {
	return index.Entry{
		Type:     index.TypeKB,
		Name:     name,
		Title:    title,
		Category: category,
		Keywords: keywords,
		Bigrams:  bigrams,
	}
}

// baseEntries returns a stable set of 5 entries used across tests that need IDF.
// Having >= 2 entries is required for ComputeIDF to return non-nil.
func baseEntries() []index.Entry {
	return []index.Entry{
		makeEntry("go-concurrency", "Go Concurrency Patterns", "go",
			map[string]int{"goroutine": 3, "channel": 2, "concurrency": 2}, nil),
		makeEntry("go-iface", "Go Interfaces", "go",
			map[string]int{"interface": 3, "go": 2, "polymorphism": 1}, nil),
		makeEntry("refactor-extract", "Extract Method Refactoring", "refactoring",
			map[string]int{"refactor": 3, "method": 2, "extract": 2}, nil),
		makeEntry("bash-pipes", "Bash Pipes and Redirects", "bash",
			map[string]int{"pipe": 3, "redirect": 2, "bash": 2}, nil),
		makeEntry("llm-tokens", "LLM Token Limits", "llm",
			map[string]int{"token": 3, "llm": 2, "limit": 1}, nil),
	}
}

func TestScore(t *testing.T) {
	t.Parallel()

	// nilIDF is convenient for tests that do not need IDF weighting.
	var nilIDF map[string]float64

	tests := []struct {
		name      string
		entries   []index.Entry
		tokens    []string
		threshold int
		idf       map[string]float64
		wantNil   bool   // true when nil result expected
		wantCount int    // expected result length (when wantNil=false)
		wantFirst string // expected first result Name (when wantCount >= 1)
		wantScore int    // expected Score of first result (0 means skip check)
	}{
		{
			// Case 1: empty tokens → nil results regardless of entries.
			name:      "empty tokens returns nil",
			entries:   baseEntries(),
			tokens:    nil,
			threshold: 1,
			idf:       nilIDF,
			wantNil:   true,
		},
		{
			// Case 2: direct keyword match, weight 3, IDF=1 → score=3.
			// Use "channel" as the token — it is not a category alias, so score is
			// purely weight × IDF = 3 × 1.0 = 3.
			name: "direct keyword match",
			entries: []index.Entry{
				makeEntry("channel-entry", "Channel Basics", "go",
					map[string]int{"channel": 3}, nil),
				makeEntry("unrelated", "Something Else", "bash",
					map[string]int{"pipe": 3}, nil),
			},
			tokens:    []string{"channel"},
			threshold: 1,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 1,
			wantFirst: "channel-entry",
			wantScore: 3,
		},
		{
			// Case 3: higher weight beats lower weight in ranking.
			name: "higher weight wins ranking",
			entries: []index.Entry{
				makeEntry("low-weight", "Low Weight Entry", "go",
					map[string]int{"channel": 1}, nil),
				makeEntry("high-weight", "High Weight Entry", "go",
					map[string]int{"channel": 3}, nil),
			},
			tokens:    []string{"channel"},
			threshold: 1,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 2,
			wantFirst: "high-weight",
		},
		{
			// Case 4: "golang" is an alias for category "go" → alias boost applied.
			name: "category alias match golang→go",
			entries: []index.Entry{
				makeEntry("go-entry", "Go Patterns", "go",
					map[string]int{"pattern": 1}, nil),
				makeEntry("bash-entry", "Bash Tips", "bash",
					map[string]int{"pattern": 1}, nil),
			},
			tokens:    []string{"golang"},
			threshold: 1,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 1,
			wantFirst: "go-entry",
		},
		{
			// Case 5: plural +s normalization — "goroutines" should match keyword "goroutine"
			// via the tok+"s" path (entry.Keywords["goroutines"]).
			// We store "goroutines" as the keyword; prompt sends "goroutine".
			// Wait — scorer does: if Keywords[tok+"s"] match → plural strip.
			// Token = "goroutines", check Keywords["goroutines"+"s"] no, then
			// CutSuffix("goroutines","s") → "goroutine", check Keywords["goroutine"]. YES.
			name: "plural strip -s matches singular keyword",
			entries: []index.Entry{
				makeEntry("gr-entry", "Goroutine Entry", "go",
					map[string]int{"goroutine": 2}, nil),
				makeEntry("other", "Other", "bash",
					map[string]int{"pipe": 2}, nil),
			},
			tokens:    []string{"goroutines"},
			threshold: 1,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 1,
			wantFirst: "gr-entry",
		},
		{
			// Case 6: plural +s — token "test" matches keyword "tests".
			// scorer does: if entry.Keywords[tok+"s"] match → plural +s path.
			name: "plural +s normalization token matches keyword+s",
			entries: []index.Entry{
				makeEntry("test-entry", "Testing Patterns", "go",
					map[string]int{"tests": 2}, nil),
				makeEntry("other", "Other", "bash",
					map[string]int{"pipe": 2}, nil),
			},
			tokens:    []string{"test"},
			threshold: 1,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 1,
			wantFirst: "test-entry",
		},
		{
			// Case 7: stem match 0.6× — "refactoring" token matches keyword "refactor".
			// HasStemMatch("refactoring","refactor"): shared prefix "refactor" (8 chars),
			// minLen=8, 8*4=32 >= 8*3=24 → true. Score = 3 * 0.6 = 1.8 → rounds to 2.
			name: "stem match 0.6x multiplier",
			entries: []index.Entry{
				makeEntry("ref-entry", "Refactor Tips", "refactoring",
					map[string]int{"refactor": 3}, nil),
				makeEntry("other", "Other", "bash",
					map[string]int{"pipe": 3}, nil),
			},
			tokens:    []string{"refactoring"},
			threshold: 1,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 1,
			wantFirst: "ref-entry",
			wantScore: 2, // round(3 * 0.6 * 1.0) = round(1.8) = 2
		},
		{
			// Case 8: bigram +3 flat bonus — tokens ["race","condition"] build bigram
			// "race condition" which matches entry bigram.
			name: "bigram flat +3 bonus",
			entries: []index.Entry{
				makeEntry("race-entry", "Race Condition Detection", "go",
					map[string]int{"race": 1, "condition": 1},
					[]string{"race condition"}),
				makeEntry("other", "Other Entry", "bash",
					map[string]int{"pipe": 1}, nil),
			},
			tokens:    []string{"race", "condition"},
			threshold: 1,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 1, // "other" has keyword "pipe" not in tokens → score=0, filtered
			// race-entry: race(1) + condition(1) + bigram(3) = 5
			wantFirst: "race-entry",
			wantScore: 5,
		},
		{
			// Case 9: below threshold excluded — threshold=10, no entry can score that high.
			name: "below threshold excluded",
			entries: []index.Entry{
				makeEntry("low-score", "Low Score Entry", "go",
					map[string]int{"goroutine": 3}, nil),
			},
			tokens:    []string{"goroutine"},
			threshold: 10,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 0,
		},
		{
			// Case 10: ScoreTop limit capping — tested via Score returning all, then
			// checking ScoreTop respects limit. We use wantCount to verify Score returns many.
			// This case validates Score without limit (use threshold=1, 3 matching entries).
			name: "multiple results all above threshold",
			entries: []index.Entry{
				makeEntry("e1", "Entry One", "go", map[string]int{"goroutine": 3}, nil),
				makeEntry("e2", "Entry Two", "go", map[string]int{"goroutine": 2}, nil),
				makeEntry("e3", "Entry Three", "go", map[string]int{"goroutine": 1}, nil),
			},
			tokens:    []string{"goroutine"},
			threshold: 1,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 3,
			wantFirst: "e1",
		},
		{
			// Case 11: IDF weighting — a rare term (appears in 1 of 4 entries) scores
			// higher than a common term (appears in 4 of 4 entries).
			name: "IDF rare term scores higher than common term",
			entries: func() []index.Entry {
				// "common" appears in all 4, "rare" appears in only 1.
				e1 := makeEntry("rare-holder", "Rare Term Entry", "go",
					map[string]int{"rare": 3, "common": 3}, nil)
				e2 := makeEntry("common-only-1", "Common Only A", "bash",
					map[string]int{"common": 3}, nil)
				e3 := makeEntry("common-only-2", "Common Only B", "bash",
					map[string]int{"common": 3}, nil)
				e4 := makeEntry("common-only-3", "Common Only C", "bash",
					map[string]int{"common": 3}, nil)
				return []index.Entry{e1, e2, e3, e4}
			}(),
			tokens:    []string{"rare"},
			threshold: 1,
			idf:       nilIDF, // overwritten below via tc.name guard before Score is called
			wantNil:   false,
			wantCount: 1,
			wantFirst: "rare-holder",
		},
		{
			// Case 12: results sorted descending by score.
			name: "results sorted descending",
			entries: []index.Entry{
				makeEntry("weak", "Weak Match", "go", map[string]int{"goroutine": 1}, nil),
				makeEntry("strong", "Strong Match", "go", map[string]int{"goroutine": 3}, nil),
				makeEntry("mid", "Mid Match", "go", map[string]int{"goroutine": 2}, nil),
			},
			tokens:    []string{"goroutine"},
			threshold: 1,
			idf:       nilIDF,
			wantNil:   false,
			wantCount: 3,
			wantFirst: "strong",
		},
		{
			// Case 13: alias + keyword combined — "golang" alias boosts "go" category (+2)
			// AND token "goroutine" directly matches keyword (weight 3). Scores are additive.
			name: "alias and keyword combined additive",
			entries: []index.Entry{
				makeEntry("go-full", "Go Full Entry", "go",
					map[string]int{"goroutine": 3}, nil),
				makeEntry("go-nokey", "Go No Keyword", "go",
					map[string]int{"other": 1}, nil),
			},
			tokens:    []string{"golang", "goroutine"},
			threshold: 1,
			idf:       nilIDF,
			// go-full: golang alias→go(+2) + goroutine keyword(+3) = 5
			// go-nokey: golang alias→go(+2) = 2
			wantNil:   false,
			wantCount: 2,
			wantFirst: "go-full",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idf := tc.idf
			// For the IDF test case we compute it from the actual entries.
			if tc.name == "IDF rare term scores higher than common term" {
				idf = index.ComputeIDF(tc.entries)
			}

			results := Score(tc.entries, tc.tokens, tc.threshold, idf, 0)

			if tc.wantNil {
				if results != nil {
					t.Errorf("expected nil results, got %d entries", len(results))
				}
				return
			}

			if len(results) != tc.wantCount {
				t.Errorf("expected %d results, got %d", tc.wantCount, len(results))
				for i, r := range results {
					t.Logf("  [%d] name=%s score=%d", i, r.Entry.Name, r.Score)
				}
				return
			}

			if tc.wantCount == 0 {
				return
			}

			if tc.wantFirst != "" && results[0].Entry.Name != tc.wantFirst {
				t.Errorf("expected first result %q, got %q (score=%d)",
					tc.wantFirst, results[0].Entry.Name, results[0].Score)
				for i, r := range results {
					t.Logf("  [%d] name=%s score=%d", i, r.Entry.Name, r.Score)
				}
			}

			if tc.wantScore != 0 && results[0].Score != tc.wantScore {
				t.Errorf("expected first result score=%d, got %d (entry=%s)",
					tc.wantScore, results[0].Score, results[0].Entry.Name)
			}
		})
	}
}

func TestScoreTop_LimitCapping(t *testing.T) {
	t.Parallel()

	// Use "channel" — not a category alias — so score is purely weight × IDF = 3.
	// Category "misc" is not in the alias table, preventing any alias boost.
	entries := []index.Entry{
		makeEntry("e1", "Entry One", "misc", map[string]int{"channel": 3}, nil),
		makeEntry("e2", "Entry Two", "misc", map[string]int{"channel": 3}, nil),
		makeEntry("e3", "Entry Three", "misc", map[string]int{"channel": 3}, nil),
		makeEntry("e4", "Entry Four", "misc", map[string]int{"channel": 3}, nil),
		makeEntry("e5", "Entry Five", "misc", map[string]int{"channel": 3}, nil),
	}

	results := ScoreTop(entries, []string{"channel"}, 1, 3, nil, 0)
	if len(results) != 3 {
		t.Errorf("ScoreTop with limit=3 returned %d results, want 3", len(results))
	}

	// All score exactly 3 (weight=3, IDF=1.0, no alias contribution).
	for i, r := range results {
		if r.Score != 3 {
			t.Errorf("result[%d] score=%d, want 3", i, r.Score)
		}
	}
}

func TestHasStemMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{
			name: "identical strings returns false",
			a:    "refactor",
			b:    "refactor",
			want: false,
		},
		{
			name: "sufficient overlap returns true",
			a:    "refactoring",
			b:    "refactor",
			want: true,
		},
		{
			name: "too short minLen returns false",
			a:    "go",
			b:    "got",
			want: false,
		},
		{
			name: "insufficient overlap returns false",
			a:    "debugging",
			b:    "debunking",
			// shared prefix "deb" = 3 bytes — minLen=8, shared=3 < 4 → false
			want: false,
		},
		{
			name: "exact 75% overlap at boundary",
			// "goroutine" (9) vs "gorout" (6): shared=6, minLen=6, 6*4=24 >= 6*3=18 → true
			a:    "goroutine",
			b:    "gorout",
			want: true,
		},
		{
			name: "shared prefix exactly 4 bytes passes minimum",
			// "test" (4) vs "tester" (6): shared=4, minLen=4, 4*4=16 >= 4*3=12 → true
			a:    "test",
			b:    "tester",
			want: true,
		},
		{
			name: "completely different words returns false",
			a:    "goroutine",
			b:    "database",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := HasStemMatch(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("HasStemMatch(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
