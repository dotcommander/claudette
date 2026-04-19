package search

import (
	"math"
	"testing"

	"github.com/dotcommander/claudette/internal/index"
)

// TestScoreExplained_EmptyTokens_ReturnsNil mirrors the Score contract.
func TestScoreExplained_EmptyTokens_ReturnsNil(t *testing.T) {
	t.Parallel()

	diags := ScoreExplained(baseEntries(), nil, DefaultThreshold, nil, 0)
	if diags != nil {
		t.Errorf("expected nil for empty tokens, got %d entries", len(diags))
	}
}

// TestScoreExplained_MatchesScore verifies that filtering ScoreExplained results
// to Suppressed=="" produces byte-identical output to Score.
// This is the invariant test — any divergence indicates a parallel implementation bug.
func TestScoreExplained_MatchesScore(t *testing.T) {
	t.Parallel()

	corpora := []struct {
		name    string
		entries []index.Entry
		tokens  []string
		idf     map[string]float64
		avgdl   float64
	}{
		{
			name:    "base entries channel no idf",
			entries: baseEntries(),
			tokens:  []string{"channel"},
		},
		{
			name:    "base entries concurrency idf",
			entries: baseEntries(),
			tokens:  []string{"concurrency"},
			idf:     index.ComputeIDF(baseEntries()),
			avgdl:   index.ComputeAvgFieldLen(baseEntries()),
		},
		{
			name: "bigram match",
			entries: []index.Entry{
				makeEntry("race-entry", "Race Condition Detection", "misc",
					map[string]int{"race": 1, "condition": 1},
					[]string{"race condition"}),
				makeEntry("other", "Other Entry", "misc",
					map[string]int{"pipe": 1}, nil),
			},
			tokens: []string{"race", "condition"},
		},
		{
			name:    "alias match golang to go",
			entries: baseEntries(),
			tokens:  []string{"golang"},
		},
	}

	for _, c := range corpora {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			scoreResults := Score(c.entries, c.tokens, DefaultThreshold, c.idf, c.avgdl)
			diags := ScoreExplained(c.entries, c.tokens, DefaultThreshold, c.idf, c.avgdl)

			// Filter diags to kept entries only.
			var kept []EntryDiagnostics
			for _, d := range diags {
				if d.Suppressed == "" {
					kept = append(kept, d)
				}
			}

			if len(kept) != len(scoreResults) {
				t.Errorf("kept=%d, Score=%d; must be equal", len(kept), len(scoreResults))
				for i, r := range scoreResults {
					t.Logf("Score[%d] name=%s score=%d", i, r.Entry.Name, r.Score)
				}
				for i, d := range kept {
					t.Logf("Kept[%d] name=%s final=%d", i, d.Entry.Name, d.FinalScore)
				}
				return
			}

			for i := range kept {
				if kept[i].Entry.Name != scoreResults[i].Entry.Name {
					t.Errorf("[%d] name mismatch: explain=%q, score=%q",
						i, kept[i].Entry.Name, scoreResults[i].Entry.Name)
				}
				if kept[i].FinalScore != scoreResults[i].Score {
					t.Errorf("[%d] %s: final score explain=%d, score=%d",
						i, kept[i].Entry.Name, kept[i].FinalScore, scoreResults[i].Score)
				}
			}
		})
	}
}

// TestScoreExplained_SuppressedBelowThreshold verifies entries below threshold appear
// with Suppressed="below threshold".
// Uses category "misc" and token "pipe" — neither is in the alias table, so the score
// is purely the BM25 weight and is predictable.
func TestScoreExplained_SuppressedBelowThreshold(t *testing.T) {
	t.Parallel()

	entries := []index.Entry{
		// "pipe" weight=1, no alias → score=1. threshold=2 → suppressed.
		makeEntry("low", "Low Scorer", "misc", map[string]int{"pipe": 1}, nil),
	}
	diags := ScoreExplained(entries, []string{"pipe"}, 2, nil, 0)

	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Suppressed != "below threshold" {
		t.Errorf("expected Suppressed=%q, got %q", "below threshold", diags[0].Suppressed)
	}
}

// TestScoreExplained_SuppressedIDFGate verifies entries matched only via a common
// term appear with Suppressed containing "idf gate".
func TestScoreExplained_SuppressedIDFGate(t *testing.T) {
	t.Parallel()

	// All entries share "common" → very low IDF. No bigram hit → IDF gate fires.
	entries := []index.Entry{
		makeEntry("target", "Target", "misc", map[string]int{"common": 3}, nil),
		makeEntry("e2", "E2", "misc", map[string]int{"common": 3}, nil),
		makeEntry("e3", "E3", "misc", map[string]int{"common": 3}, nil),
		makeEntry("e4", "E4", "misc", map[string]int{"common": 3}, nil),
		makeEntry("e5", "E5", "misc", map[string]int{"common": 3}, nil),
	}
	idf := index.ComputeIDF(entries)
	diags := ScoreExplained(entries, []string{"common"}, 1, idf, 0)

	for _, d := range diags {
		if d.Suppressed == "" {
			t.Errorf("entry %q should be suppressed by IDF gate, got Suppressed=%q", d.Entry.Name, d.Suppressed)
		}
		if d.Suppressed != "" && d.Suppressed != "idf gate: low-idf" && d.Suppressed != "below threshold" {
			t.Errorf("entry %q unexpected Suppressed=%q", d.Entry.Name, d.Suppressed)
		}
	}
}

// TestScoreExplained_TokenHits_CoverAllKinds verifies TokenHit.Kind is populated
// correctly for each match mechanism. Uses category "misc" which is not in the
// alias table to avoid alias interference on non-alias test cases.
func TestScoreExplained_TokenHits_CoverAllKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entry     index.Entry
		token     string
		wantKind  string
		wantAlias string
	}{
		{
			// "pipe" is not an alias; category "misc" not in alias table.
			name:     "direct match",
			entry:    makeEntry("e", "E", "misc", map[string]int{"pipe": 2}, nil),
			token:    "pipe",
			wantKind: "direct",
		},
		{
			// "test" is not an alias; token matches keyword "tests" via plural +s path.
			name:     "plural +s: token matches keyword+s",
			entry:    makeEntry("e", "E", "misc", map[string]int{"tests": 2}, nil),
			token:    "test",
			wantKind: "plural",
		},
		{
			// "channels" is not an alias; token with suffix -s matches bare keyword "channel".
			name:     "plural strip-s: token with suffix matches bare keyword",
			entry:    makeEntry("e", "E", "misc", map[string]int{"channel": 2}, nil),
			token:    "channels",
			wantKind: "plural",
		},
		{
			// "refactoring" not a direct keyword match; HasStemMatch("refactoring","refactor")=true.
			// "refactoring" is also a category alias for "refactoring" category,
			// but the entry category here is "misc" so alias does not fire.
			name:     "stem match",
			entry:    makeEntry("e", "E", "misc", map[string]int{"refactor": 3}, nil),
			token:    "refactoring",
			wantKind: "stem",
		},
		{
			// "golang" aliases to "go"; entry category "go" has no matching keyword → alias only.
			name:      "alias only",
			entry:     makeEntry("e", "E", "go", map[string]int{"other": 1}, nil),
			token:     "golang",
			wantKind:  "alias",
			wantAlias: "go",
		},
		{
			// "golang" aliases to "go"; entry has keyword "golang" → alias+direct.
			name:      "alias+direct combined",
			entry:     makeEntry("e", "E", "go", map[string]int{"golang": 2}, nil),
			token:     "golang",
			wantKind:  "alias+direct",
			wantAlias: "go",
		},
		{
			// "xyzzy" is not a keyword, not an alias, no stem match → miss.
			name:     "miss",
			entry:    makeEntry("e", "E", "misc", map[string]int{"other": 1}, nil),
			token:    "xyzzy",
			wantKind: "miss",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diags := ScoreExplained([]index.Entry{tc.entry}, []string{tc.token}, 0, nil, 0)
			if len(diags) != 1 {
				t.Fatalf("expected 1 diagnostic, got %d", len(diags))
			}
			hits := diags[0].TokenHits
			if len(hits) != 1 {
				t.Fatalf("expected 1 token hit, got %d", len(hits))
			}
			if hits[0].Kind != tc.wantKind {
				t.Errorf("Kind=%q, want %q", hits[0].Kind, tc.wantKind)
			}
			if tc.wantAlias != "" && hits[0].AliasCat != tc.wantAlias {
				t.Errorf("AliasCat=%q, want %q", hits[0].AliasCat, tc.wantAlias)
			}
			if hits[0].Token != tc.token {
				t.Errorf("Token=%q, want %q", hits[0].Token, tc.token)
			}
		})
	}
}

// TestScoreExplained_BigramHits verifies bigram matches populate BigramHits verbatim.
func TestScoreExplained_BigramHits(t *testing.T) {
	t.Parallel()

	entry := makeEntry("race-entry", "Race Condition Detection", "misc",
		map[string]int{"race": 1, "condition": 1},
		[]string{"race condition"})

	diags := ScoreExplained([]index.Entry{entry}, []string{"race", "condition"}, 0, nil, 0)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	hits := diags[0].BigramHits
	if len(hits) != 1 {
		t.Fatalf("expected 1 bigram hit, got %v", hits)
	}
	if hits[0] != "race condition" {
		t.Errorf("BigramHits[0]=%q, want %q", hits[0], "race condition")
	}
}

// TestScoreExplained_UsageBoost verifies that usage boost math is correctly captured.
func TestScoreExplained_UsageBoost(t *testing.T) {
	t.Parallel()

	const epsilon = 0.001

	// Use "pipe"/"misc" to avoid alias interference on the score math.
	entry := makeEntry("popular", "Popular Entry", "misc", map[string]int{"pipe": 3}, nil)
	entry.HitCount = 100

	diags := ScoreExplained([]index.Entry{entry}, []string{"pipe"}, 0, nil, 0)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	d := diags[0]

	// boost = 1 + log1p(100)/10 ≈ 1.461
	wantBoost := 1 + math.Log1p(100)/10
	if diff := d.UsageBoost - wantBoost; diff > epsilon || diff < -epsilon {
		t.Errorf("UsageBoost=%.4f, want ~%.4f", d.UsageBoost, wantBoost)
	}

	// RawScore must equal PreBoostSum * UsageBoost within float tolerance.
	expected := d.PreBoostSum * d.UsageBoost
	if diff := d.RawScore - expected; diff > epsilon || diff < -epsilon {
		t.Errorf("RawScore=%.4f, PreBoostSum*UsageBoost=%.4f; must be equal within %.3f",
			d.RawScore, expected, epsilon)
	}
}

// TestScoreExplained_SortOrder verifies kept entries appear before suppressed entries.
// Uses "misc" category and "pipe" token which have no alias interference.
func TestScoreExplained_SortOrder(t *testing.T) {
	t.Parallel()

	entries := []index.Entry{
		// "pipe" weight=3, no alias → score=3 → passes threshold=2.
		makeEntry("strong", "Strong Match", "misc", map[string]int{"pipe": 3}, nil),
		// "pipe" weight=1, no alias → score=1 → suppressed at threshold=2.
		makeEntry("weak", "Weak Match", "misc", map[string]int{"pipe": 1}, nil),
	}
	diags := ScoreExplained(entries, []string{"pipe"}, 2, nil, 0)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].Suppressed != "" {
		t.Errorf("expected first entry to be kept (Suppressed=%q), got Suppressed=%q",
			"", diags[0].Suppressed)
	}
	if diags[0].Entry.Name != "strong" {
		t.Errorf("expected first kept entry=strong, got %q", diags[0].Entry.Name)
	}
	if diags[1].Suppressed == "" {
		t.Errorf("expected second entry to be suppressed, got Suppressed=%q", diags[1].Suppressed)
	}
}
