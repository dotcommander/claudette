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
		autoIDF   bool   // if true, ComputeIDF(entries) is used instead of idf
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
			// Case 5: plural strip-s normalization — token "goroutines" (plural) should
			// match keyword "goroutine" (singular) via the CutSuffix path.
			// scoreToken: Keywords["goroutines"+"s"] miss → CutSuffix("goroutines","s")
			// → "goroutine" → Keywords["goroutine"] hit. Score = weight × 0.9.
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
			// Case 8: bigram IDF-weighted bonus (floor=1.5) — tokens ["race","condition"]
			// build bigram "race condition" which matches entry bigram.
			// With nil IDF, idfMul returns 1.0 for all tokens, so:
			//   bonus = max(1.5, (1.0+1.0)/2) = max(1.5, 1.0) = 1.5
			//   score = race(1) + condition(1) + bigram(1.5) = 3.5 → rounds to 4
			name: "bigram IDF-weighted bonus floor",
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
			wantFirst: "race-entry",
			wantScore: 4,
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
			idf:       nilIDF,
			autoIDF:   true, // ComputeIDF(entries) applied before Score is called
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
		{
			// Case 14: token "go" maps to category alias "go" and must surface kb/go entries
			// even when IDF is computed (i.e. "go" has high df, low IDF as a keyword).
			// The alias boost is flat +2 — not multiplied by IDF — so it must always reach
			// threshold=2 regardless of how common "go" is as a keyword across the corpus.
			// This is the regression case: `claudette search go` returned zero results before
			// "go" was added to categoryAliases.
			name: "token go aliases to category go with real IDF",
			entries: func() []index.Entry {
				// "go" appears as a keyword in all entries → very low IDF.
				// Alias boost must still reach threshold=2.
				entries := make([]index.Entry, 10)
				for i := range 10 {
					entries[i] = makeEntry(
						"go-entry-"+string(rune('a'+i)),
						"Go Entry",
						"go",
						map[string]int{"go": 2, "goroutine": 1},
						nil,
					)
				}
				return entries
			}(),
			tokens:    []string{"go"},
			threshold: 2,
			autoIDF:   true,
			wantNil:   false,
			wantCount: 10,
			wantFirst: "go-entry-a",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idf := tc.idf
			// For test cases that need IDF weighting, compute it from the actual entries.
			if tc.autoIDF {
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

func TestScore_SuppressesLowIDFOnlyMatches(t *testing.T) {
	t.Parallel()

	// Build a corpus where the query token "common" appears in every entry.
	// ComputeIDF will assign it a very low IDF (close to 0 — log(N/df) with df≈N).
	// A doc that matches only via "common" should be suppressed by the IDF gate.
	entries := []index.Entry{
		makeEntry("target", "Target Entry", "go", map[string]int{"common": 3}, nil),
		makeEntry("e2", "Entry 2", "go", map[string]int{"common": 3}, nil),
		makeEntry("e3", "Entry 3", "go", map[string]int{"common": 3}, nil),
		makeEntry("e4", "Entry 4", "go", map[string]int{"common": 3}, nil),
		makeEntry("e5", "Entry 5", "go", map[string]int{"common": 3}, nil),
	}

	idf := index.ComputeIDF(entries)
	results := Score(entries, []string{"common"}, 1, idf, 0)

	// All entries match only via a very-common term — all should be suppressed.
	if len(results) != 0 {
		t.Errorf("expected 0 results after IDF gate for low-IDF-only match, got %d", len(results))
		for i, r := range results {
			t.Logf("  [%d] name=%s score=%d matched=%v", i, r.Entry.Name, r.Score, r.Matched)
		}
	}
}

func TestScore_MatchedSortedByContribution(t *testing.T) {
	t.Parallel()

	// Build an entry where "goroutine" has weight 3 and "channel" has weight 1.
	// With nilIDF (defaults to 1.0), goroutine contributes 3× more than channel.
	// The IDF gate is bypassed (idf==nil), so results appear unconditionally.
	entries := []index.Entry{
		makeEntry("target", "Target Entry", "go",
			map[string]int{"goroutine": 3, "channel": 1}, nil),
	}

	results := Score(entries, []string{"channel", "goroutine"}, 1, nil, 0)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	matched := results[0].Matched
	if len(matched) < 2 {
		t.Fatalf("expected at least 2 matched terms, got %v", matched)
	}

	// goroutine (delta=3) should sort before channel (delta=1).
	if matched[0] != "goroutine" {
		t.Errorf("expected Matched[0]=%q (highest delta), got %q; full matched=%v",
			"goroutine", matched[0], matched)
	}
	if matched[1] != "channel" {
		t.Errorf("expected Matched[1]=%q (lower delta), got %q; full matched=%v",
			"channel", matched[1], matched)
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

// TestScore_AliasOnlyTokensMatch verifies that an entry whose keywords were
// populated solely from alias phrases (at index time) is still matched by the
// scorer when the prompt contains only alias-derived tokens.
func TestScore_AliasOnlyTokensMatch(t *testing.T) {
	t.Parallel()

	// Simulate an entry whose "pulling" and "extracting" keywords came from
	// aliases — they would not appear in name, title, or category.
	entry := makeEntry(
		"maps-keys-returns-iterator",
		"maps.Keys returns iterator, not slice",
		"go",
		// keywords a real parseEntry would produce for the alias fixture
		map[string]int{
			"maps":       3, // from name
			"keys":       3, // from name
			"returns":    3, // from name
			"iterator":   3, // from name
			"slice":      2, // from title
			"pulling":    1, // from alias: "pulling keys out of a go map"
			"extracting": 1, // from alias: "extracting map keys"
			"seq":        1, // from alias: "iter.Seq instead of slice"
		},
		nil,
	)

	// Tokens from a prompt like "pulling just the keys out of a 1.22 map" —
	// only alias-derived tokens should be enough to surface the entry.
	results := Score(
		[]index.Entry{entry},
		[]string{"pulling", "keys"},
		2, // threshold
		nil,
		0,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result from alias-only tokens, got %d", len(results))
	}
	if results[0].Entry.Name != "maps-keys-returns-iterator" {
		t.Errorf("expected maps-keys-returns-iterator, got %q", results[0].Entry.Name)
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

func TestApplyUsageBoost(t *testing.T) {
	t.Parallel()

	const epsilon = 0.001

	tests := []struct {
		name     string
		score    float64
		hitCount float64
		want     float64
	}{
		{name: "zero hits returns score unchanged", score: 10.0, hitCount: 0, want: 10.0},
		{name: "negative hits returns score unchanged", score: 10.0, hitCount: -5, want: 10.0},
		{name: "one hit ~ 1.069x", score: 10.0, hitCount: 1, want: 10.693},
		{name: "ten hits ~ 1.240x", score: 10.0, hitCount: 10, want: 12.398},
		{name: "hundred hits ~ 1.462x", score: 10.0, hitCount: 100, want: 14.615},
		{name: "thousand hits ~ 1.691x", score: 10.0, hitCount: 1000, want: 16.909},
		{name: "zero score stays zero", score: 0.0, hitCount: 1000, want: 0.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := applyUsageBoost(tc.score, tc.hitCount)
			if diff := got - tc.want; diff > epsilon || diff < -epsilon {
				t.Errorf("applyUsageBoost(%v, %v) = %v, want ~%v (±%v)",
					tc.score, tc.hitCount, got, tc.want, epsilon)
			}
		})
	}
}

// TestScore_UsageBoostReRanks verifies that HitCount causes re-ranking when
// raw scores are close enough that the boost crosses a rounding boundary.
func TestScore_UsageBoostReRanks(t *testing.T) {
	t.Parallel()

	// Two entries with identical raw scoring: both match "goroutine" weight 3.
	// Entry "popular" has HitCountDecayed=100 → boost ~1.462 → raw 3 × 1.462 = 4.386 → rounds to 4.
	// Entry "unused"  has HitCountDecayed=0   → boost 1.0     → raw 3 × 1.0   = 3.000 → rounds to 3.
	// "popular" MUST sort before "unused".
	popular := makeEntry("popular", "Popular Entry", "go",
		map[string]int{"goroutine": 3}, nil)
	popular.HitCountDecayed = 100

	unused := makeEntry("unused", "Unused Entry", "go",
		map[string]int{"goroutine": 3}, nil)
	// HitCountDecayed left at zero value.

	// Order the input deliberately with "unused" first so a passing test proves
	// the re-rank is caused by the boost, not input order.
	entries := []index.Entry{unused, popular}
	results := Score(entries, []string{"goroutine"}, 1, nil, 0)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Entry.Name != "popular" {
		t.Errorf("expected popular first (HitCount=100), got %q first", results[0].Entry.Name)
		for i, r := range results {
			t.Logf("  [%d] name=%s score=%d hitCount=%d",
				i, r.Entry.Name, r.Score, r.Entry.HitCount)
		}
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("expected boosted score strictly greater than unboosted: popular=%d unused=%d",
			results[0].Score, results[1].Score)
	}
}

// TestScore_UsageBoostZeroHitIsIdentity verifies that entries with HitCount=0
// score identically to the pre-boost behavior. This is the regression guard:
// introducing the boost must not perturb results for entries that have never
// been hit (which is the common case in a fresh install).
func TestScore_UsageBoostZeroHitIsIdentity(t *testing.T) {
	t.Parallel()

	// Reuse the exact fixture from TestScore case 2 ("direct keyword match")
	// which expects score=3 for a weight-3 direct match with nil IDF.
	entries := []index.Entry{
		makeEntry("channel-entry", "Channel Basics", "go",
			map[string]int{"channel": 3}, nil),
	}

	results := Score(entries, []string{"channel"}, 1, nil, 0)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score != 3 {
		t.Errorf("zero-HitCount entry scored %d, want 3 (pre-boost behavior must be preserved)",
			results[0].Score)
	}
}

// TestBigramBonus_BothLowIDF_HigherScore verifies that a bigram whose both tokens are
// rare (high IDF) earns a higher bonus than a bigram whose both tokens are common (low IDF).
// The IDF-weighted average formula: bonus = max(bigramFloor, (idf1+idf2)/2)
func TestBigramBonus_BothLowIDF_HigherScore(t *testing.T) {
	t.Parallel()

	// Build a corpus: "errgroup" and "context" appear in only 1 of 6 entries → rare → high IDF.
	// "code" and "debug" appear in all 6 entries → common → low IDF.
	//
	// rare-entry has bigram "errgroup context"; common-entry has bigram "code debug".
	// Both have the same keyword weights, so the bigram bonus is the discriminating factor.
	rareEntry := makeEntry("rare-entry", "Errgroup Context Usage", "go",
		map[string]int{"errgroup": 2, "context": 2},
		[]string{"errgroup context"})
	commonEntry := makeEntry("common-entry", "Code Debug Guide", "go",
		map[string]int{"code": 2, "debug": 2},
		[]string{"code debug"})
	// Filler entries that make "code" and "debug" high-df (appear in all).
	filler := func(n string) index.Entry {
		return makeEntry(n, "Filler", "bash",
			map[string]int{"code": 1, "debug": 1}, nil)
	}
	entries := []index.Entry{
		rareEntry, commonEntry,
		filler("f1"), filler("f2"), filler("f3"), filler("f4"),
	}

	idf := index.ComputeIDF(entries)

	// Score rare-entry against "errgroup context"
	rareResults := Score([]index.Entry{rareEntry}, []string{"errgroup", "context"}, 1, idf, 0)
	// Score common-entry against "code debug"
	commonResults := Score([]index.Entry{commonEntry}, []string{"code", "debug"}, 1, idf, 0)

	if len(rareResults) != 1 {
		t.Fatalf("rare-entry: expected 1 result, got %d", len(rareResults))
	}
	if len(commonResults) != 1 {
		t.Fatalf("common-entry: expected 1 result, got %d", len(commonResults))
	}

	rareScore := rareResults[0].Score
	commonScore := commonResults[0].Score
	if rareScore <= commonScore {
		t.Errorf("rare-pair score (%d) should be > common-pair score (%d); errgroup+context are rare, code+debug are ubiquitous",
			rareScore, commonScore)
	}
}

// TestBigramBonus_BothHighIDF_AtOrNearFloor verifies that a bigram where both tokens
// are extremely common (IDF near 0) still awards at least bigramFloor (1.5).
func TestBigramBonus_BothHighIDF_AtOrNearFloor(t *testing.T) {
	t.Parallel()

	// "the" and "code" appear in every entry → near-zero IDF. The bigram should
	// still yield at least bigramFloor contribution to the score.
	entries := make([]index.Entry, 10)
	for i := range 10 {
		entries[i] = makeEntry(
			"e"+string(rune('a'+i)), "Entry", "go",
			map[string]int{"the": 1, "code": 1}, nil,
		)
	}
	// Target has the bigram registered.
	target := makeEntry("target", "The Code Guide", "go",
		map[string]int{"the": 1, "code": 1},
		[]string{"the code"})
	entries[0] = target

	idf := index.ComputeIDF(entries)

	// Compute what idfMul returns for "the" and "code" with this corpus.
	idfThe := idfMulExported(idf, "the")
	idfCode := idfMulExported(idf, "code")
	expectedFloor := 1.5 // bigramFloor constant
	avgIDF := (idfThe + idfCode) / 2

	results := Score([]index.Entry{target}, []string{"the", "code"}, 1, idf, 0)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// The bigram bonus must be at least bigramFloor regardless of how low the IDF is.
	// With weight=1 per token, pre-bigram score = idfThe + idfCode (approx 0).
	// bigramBonus ≥ bigramFloor → total pre-boost ≥ bigramFloor.
	// We verify floor by checking: if avgIDF < bigramFloor, score must still be >= 1.
	if avgIDF < expectedFloor {
		// Common case: IDF average is below floor, floor kicks in.
		// Score must be round(preBoost + bigramFloor) ≥ round(bigramFloor) = 2.
		if results[0].Score < 2 {
			t.Errorf("floor not applied: score=%d, avgIDF=%f < floor=%f; expected score >= 2",
				results[0].Score, avgIDF, expectedFloor)
		}
	}
	// Invariant: bonus is never negative.
	if results[0].Score < 0 {
		t.Errorf("negative score %d — bigram bonus must be non-negative", results[0].Score)
	}
}

// TestBigramBonus_MixedIDF_BetweenFloorAndHigh verifies that a bigram with one rare
// token and one common token scores between the floor (both-common) and the high
// (both-rare) cases.
func TestBigramBonus_MixedIDF_BetweenFloorAndHigh(t *testing.T) {
	t.Parallel()

	// "errgroup" and "context" appear in only 1 of 8 entries → rare → high IDF.
	// "code" and "debug" appear in all 8 entries → common → low IDF.
	// "errgroup code" is mixed: one rare + one common.
	//
	// Filler populates "code" and "debug" across the corpus so both have low IDF.
	filler := func(n string) index.Entry {
		return makeEntry(n, "Filler", "bash", map[string]int{"code": 1, "debug": 1}, nil)
	}
	rareEntry := makeEntry("rare-entry", "Errgroup Context", "go",
		map[string]int{"errgroup": 2, "context": 2},
		[]string{"errgroup context"})
	mixedEntry := makeEntry("mixed-entry", "Errgroup Code", "go",
		map[string]int{"errgroup": 2, "code": 2},
		[]string{"errgroup code"})
	commonEntry := makeEntry("common-entry", "Code Debug", "go",
		map[string]int{"code": 2, "debug": 2},
		[]string{"code debug"})

	entries := []index.Entry{
		rareEntry, mixedEntry, commonEntry,
		filler("f1"), filler("f2"), filler("f3"), filler("f4"), filler("f5"),
	}
	idf := index.ComputeIDF(entries)

	// Score each against its own matching bigram.
	rareResults := Score([]index.Entry{rareEntry}, []string{"errgroup", "context"}, 1, idf, 0)
	mixedResults := Score([]index.Entry{mixedEntry}, []string{"errgroup", "code"}, 1, idf, 0)
	commonResults := Score([]index.Entry{commonEntry}, []string{"code", "debug"}, 1, idf, 0)

	if len(rareResults) != 1 || len(mixedResults) != 1 || len(commonResults) != 1 {
		t.Fatalf("expected 1 result each; rare=%d mixed=%d common=%d",
			len(rareResults), len(mixedResults), len(commonResults))
	}

	rareScore := rareResults[0].Score
	mixedScore := mixedResults[0].Score
	commonScore := commonResults[0].Score

	// Ordering invariant: rare >= mixed >= common (may be equal at floor).
	if rareScore < mixedScore {
		t.Errorf("rare-pair score (%d) should be >= mixed-pair score (%d)", rareScore, mixedScore)
	}
	if mixedScore < commonScore {
		t.Errorf("mixed-pair score (%d) should be >= common-pair score (%d)", mixedScore, commonScore)
	}
}

// idfMulExported is a test-local wrapper to access the package-private idfMul function.
func idfMulExported(idf map[string]float64, token string) float64 {
	return idfMul(idf, token)
}

// --- SuggestedAliases scorer tests ---

// makeSuggestedEntry builds an Entry with only SuggestedAliases set (empty Keywords).
func makeSuggestedEntry(name, title, category string, suggestedAliases []string) index.Entry {
	return index.Entry{
		Type:             index.TypeSkill,
		Name:             name,
		Title:            title,
		Category:         category,
		Keywords:         map[string]int{},
		SuggestedAliases: suggestedAliases,
	}
}

// TestScore_SuggestedAlias_ScoresEntry verifies that an entry with a matching
// SuggestedAlias token but empty Keywords scores above zero.
func TestScore_SuggestedAlias_ScoresEntry(t *testing.T) {
	t.Parallel()

	entry := makeSuggestedEntry("lang-go-dev", "Go Development", "go", []string{"foo"})

	// Score against "foo" with threshold=1 and no IDF (nil).
	results := Score([]index.Entry{entry}, []string{"foo"}, 1, nil, 0)
	if len(results) == 0 {
		t.Fatal("expected SuggestedAlias match to produce a result, got none")
	}
	if results[0].Score <= 0 {
		t.Errorf("SuggestedAlias match: score should be > 0, got %d", results[0].Score)
	}
}

// TestScore_SuggestedAlias_NoDoubleCount verifies that a token matching both
// Keywords and SuggestedAliases is only scored once via Keywords (higher weight).
func TestScore_SuggestedAlias_NoDoubleCount(t *testing.T) {
	t.Parallel()

	// Entry with "foo" in both Keywords (weight=2) and SuggestedAliases.
	entryBoth := index.Entry{
		Type:             index.TypeSkill,
		Name:             "both",
		Title:            "Both",
		Category:         "go",
		Keywords:         map[string]int{"foo": 2},
		SuggestedAliases: []string{"foo"},
	}
	// Entry with "foo" in Keywords only (weight=2) — must produce identical score.
	entryKwOnly := index.Entry{
		Type:     index.TypeSkill,
		Name:     "kw-only",
		Title:    "KW Only",
		Category: "go",
		Keywords: map[string]int{"foo": 2},
	}

	resultsBoth := Score([]index.Entry{entryBoth}, []string{"foo"}, 1, nil, 0)
	resultsKwOnly := Score([]index.Entry{entryKwOnly}, []string{"foo"}, 1, nil, 0)

	if len(resultsBoth) == 0 || len(resultsKwOnly) == 0 {
		t.Fatal("expected results for both entries")
	}
	if resultsBoth[0].Score != resultsKwOnly[0].Score {
		t.Errorf("double-count detected: both=%d kw-only=%d (should be equal)",
			resultsBoth[0].Score, resultsKwOnly[0].Score)
	}
}

// TestScore_GenericPrompt_NoFortressMatch is the regression test for Item C:
// a generic prompt containing only low-signal nouns must not score above threshold.
// Mirrors the real-world case where "public product" surfaced fe-ui-svelte.
func TestScore_GenericPrompt_NoFortressMatch(t *testing.T) {
	t.Parallel()

	// Simulate the Fortress Architecture skill with "public" and "product" keywords.
	fortressEntry := makeEntry("fe-ui-svelte", "Fortress Architecture", "svelte",
		map[string]int{"fortress": 3, "architecture": 2, "public": 2, "product": 2}, nil)

	stops := DefaultStopWords()
	// "we would like this product to be public" — after stop-word filtering,
	// "product" and "public" are both removed, leaving no discriminating tokens.
	tokens := Tokenize("we would like this product to be public", stops)

	// Score against the fortress entry; result must be empty (threshold=2).
	results := Score([]index.Entry{fortressEntry}, tokens, 2, nil, 0)
	if len(results) > 0 {
		t.Errorf("generic prompt should produce zero matches above threshold 2; got %d result(s): %v",
			len(results), results)
	}
}
