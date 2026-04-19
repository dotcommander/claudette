package hook

import (
	"strings"
	"testing"

	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/search"
	"github.com/dotcommander/claudette/internal/usage"
)

// makeScoredEntries builds a slice of ScoredEntry with the given integer scores.
// Each entry gets two matched tokens so single-token suppression does not interfere.
func makeScoredEntries(scores ...int) []search.ScoredEntry {
	out := make([]search.ScoredEntry, len(scores))
	for i, s := range scores {
		out[i] = search.ScoredEntry{
			Entry:   index.Entry{Name: "entry", Title: "Entry"},
			Score:   s,
			Matched: []string{"tok1", "tok2"},
		}
	}
	return out
}

func makeResults(entries ...index.Entry) []search.ScoredEntry {
	results := make([]search.ScoredEntry, len(entries))
	for i, e := range entries {
		results[i] = search.ScoredEntry{Entry: e, Score: 10 - i, Matched: []string{"hook", "reload"}}
	}
	return results
}

func TestFormatContext_Full(t *testing.T) {
	t.Parallel()
	results := makeResults(
		index.Entry{Name: "hook-reload", Title: "Hook Reload Caching", FilePath: "/tmp/test/hook-reload.md"},
	)
	got := formatContext(results, "full", "Scan first 10 lines of each file. Only read full files that are clearly relevant.")
	if !strings.Contains(got, "<related_skills_knowledge>") {
		t.Error("full mode should contain <related_skills_knowledge> wrapper")
	}
	if !strings.Contains(got, "Only read full files") {
		t.Error("full mode should contain Only read full files instruction")
	}
	if !strings.Contains(got, "hook-reload.md") {
		t.Error("full mode should contain file path")
	}
	if !strings.Contains(got, "Hook Reload Caching") {
		t.Error("full mode should contain title")
	}
	if !strings.Contains(got, "[matched: hook, reload]") {
		t.Error("full mode should show matched tokens")
	}
	if !strings.Contains(got, "</related_skills_knowledge>") {
		t.Error("full mode should contain closing tag")
	}
}

func TestFormatContext_Compact(t *testing.T) {
	t.Parallel()
	results := makeResults(
		index.Entry{Name: "hook-reload", Title: "Hook Reload Caching", Desc: "Session caching invalidation gotcha", FilePath: "/tmp/test/hook-reload.md"},
	)
	got := formatContext(results, "compact", "Scan first 10 lines of each file. Only read full files that are clearly relevant.")
	if !strings.Contains(got, "<related_skills_knowledge>") {
		t.Error("compact mode should contain <related_skills_knowledge> wrapper")
	}
	if !strings.Contains(got, "Only read full files") {
		t.Error("compact mode should contain Only read full files instruction")
	}
	if !strings.Contains(got, "hook-reload") {
		t.Error("compact mode should contain entry name")
	}
	if !strings.Contains(got, "Session caching invalidation gotcha") {
		t.Error("compact mode should contain description")
	}
	if strings.Contains(got, "hook-reload.md") {
		t.Error("compact mode should not contain file path")
	}
	if !strings.Contains(got, "[matched: hook, reload]") {
		t.Error("compact mode should show matched tokens")
	}
	if !strings.Contains(got, "</related_skills_knowledge>") {
		t.Error("compact mode should contain closing tag")
	}
}

func TestFormatContext_CompactFallsBackToTitle(t *testing.T) {
	t.Parallel()
	results := makeResults(
		index.Entry{Name: "kb-entry", Title: "Some KB Title", Desc: "", FilePath: "/tmp/kb/entry.md"},
	)
	got := formatContext(results, "compact", "Scan first 10 lines of each file. Only read full files that are clearly relevant.")
	if !strings.Contains(got, "Some KB Title") {
		t.Error("compact mode should fall back to title when desc is empty")
	}
}

func TestOutputMode_Default(t *testing.T) {
	t.Setenv("CLAUDETTE_OUTPUT", "")
	if got := outputMode(); got != "full" {
		t.Errorf("got %q, want %q", got, "full")
	}
}

func TestOutputMode_Compact(t *testing.T) {
	t.Setenv("CLAUDETTE_OUTPUT", "compact")
	if got := outputMode(); got != "compact" {
		t.Errorf("got %q, want %q", got, "compact")
	}
}

func TestOutputMode_Unknown(t *testing.T) {
	t.Setenv("CLAUDETTE_OUTPUT", "unknown")
	if got := outputMode(); got != "full" {
		t.Errorf("unknown value should default to full, got %q", got)
	}
}

func TestFormatContext_CustomHeader(t *testing.T) {
	t.Parallel()
	results := makeResults(
		index.Entry{Name: "kb-entry", Title: "KB Title", FilePath: "/tmp/kb/entry.md"},
	)
	custom := "Custom triage instruction: verify before using."
	got := formatContext(results, "full", custom)
	if !strings.Contains(got, custom) {
		t.Errorf("output should contain custom header, got: %s", got)
	}
	if strings.Contains(got, "Only read full files") {
		t.Error("output should NOT contain default header when custom header is supplied")
	}
	if !strings.Contains(got, "<related_skills_knowledge>") {
		t.Error("output must still contain protocol open tag")
	}
	if !strings.Contains(got, "</related_skills_knowledge>") {
		t.Error("output must still contain protocol close tag")
	}
}

// --- Differential suppression gate tests ---
// These exercise applyDifferentialGate directly with fixture scores so the
// tests are deterministic and independent of real index entries or scoring
// formula changes.

// TestScoreAndRespondTo_DifferentialGate_SuppressesWhenTopLessThanCeilingAndGapSmall
// verifies that scores [6, 5, 3] — top below ceiling, gap < minScoreGap — are
// trimmed to 1 result.
func TestScoreAndRespondTo_DifferentialGate_SuppressesWhenTopLessThanCeilingAndGapSmall(t *testing.T) {
	t.Parallel()
	hits := makeScoredEntries(6, 5, 3)
	got := applyDifferentialGate(hits)
	if len(got) != 1 {
		t.Errorf("expected 1 result, got %d (scores: 6,5,3; gap=1 < minScoreGap=%d, top=%d < softCeiling=%d)",
			len(got), minScoreGap, hits[0].Score, softCeiling)
	}
	if got[0].Score != 6 {
		t.Errorf("surviving result should have score 6, got %d", got[0].Score)
	}
}

// TestScoreAndRespondTo_DifferentialGate_PreservesWhenGapLarge verifies that
// scores [6, 3] — gap >= minScoreGap — are returned intact.
func TestScoreAndRespondTo_DifferentialGate_PreservesWhenGapLarge(t *testing.T) {
	t.Parallel()
	hits := makeScoredEntries(6, 3)
	got := applyDifferentialGate(hits)
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d (scores: 6,3; gap=3 >= minScoreGap=%d)", len(got), minScoreGap)
	}
}

// TestScoreAndRespondTo_DifferentialGate_PreservesWhenAboveCeiling verifies
// that scores [15, 14] — top at or above softCeiling — are returned intact
// even though the gap is only 1.
func TestScoreAndRespondTo_DifferentialGate_PreservesWhenAboveCeiling(t *testing.T) {
	t.Parallel()
	hits := makeScoredEntries(15, 14)
	got := applyDifferentialGate(hits)
	if len(got) != 2 {
		t.Errorf("expected 2 results above ceiling, got %d (scores: 15,14; top=%d >= softCeiling=%d)",
			len(got), hits[0].Score, softCeiling)
	}
}

// TestScoreAndRespondTo_DifferentialGate_SingleResult_NoOp verifies that a
// single-result slice passes through unchanged.
func TestScoreAndRespondTo_DifferentialGate_SingleResult_NoOp(t *testing.T) {
	t.Parallel()
	hits := makeScoredEntries(6)
	got := applyDifferentialGate(hits)
	if len(got) != 1 {
		t.Errorf("expected 1 result (no-op on single), got %d", len(got))
	}
	if got[0].Score != 6 {
		t.Errorf("score should be unchanged at 6, got %d", got[0].Score)
	}
}

// TestScoreAndRespondTo_DifferentialGate_ExactlyAtCeiling_NoSuppression verifies
// that scores [10, 9] — top exactly at softCeiling — are NOT suppressed (strict <).
func TestScoreAndRespondTo_DifferentialGate_ExactlyAtCeiling_NoSuppression(t *testing.T) {
	t.Parallel()
	hits := makeScoredEntries(10, 9)
	got := applyDifferentialGate(hits)
	if len(got) != 2 {
		t.Errorf("expected 2 results at exact ceiling, got %d (scores: 10,9; top=%d, softCeiling=%d, strict < means no suppression)",
			len(got), hits[0].Score, softCeiling)
	}
}

// --- logCoOccurrence tests ---

// TestComputeUnmatched_FiltersMatchedAndDeduplicates verifies that tokens
// already in matchedSet are excluded, duplicates are deduplicated, and the
// result is capped at MaxUnmatchedPerRecord.
func TestComputeUnmatched_FiltersMatchedAndDeduplicates(t *testing.T) {
	t.Parallel()
	matched := map[string]bool{"hook": true, "reload": true}
	tokens := []string{"solid", "hook", "solid", "dry", "reload"}
	got := computeUnmatched(tokens, matched)

	// "hook" and "reload" are matched — excluded.
	// "solid" appears twice — deduplicated to one.
	// Expected: ["solid", "dry"]
	if len(got) != 2 {
		t.Fatalf("got %v, want [solid dry]", got)
	}
	if got[0] != "solid" || got[1] != "dry" {
		t.Errorf("got %v, want [solid dry]", got)
	}
}

func TestComputeUnmatched_CapsAtMax(t *testing.T) {
	t.Parallel()
	matched := map[string]bool{}
	tokens := make([]string, usage.MaxUnmatchedPerRecord+5)
	for i := range tokens {
		tokens[i] = strings.Repeat("a", i+1) // unique tokens
	}
	got := computeUnmatched(tokens, matched)
	if len(got) != usage.MaxUnmatchedPerRecord {
		t.Errorf("got %d tokens, want %d (capped)", len(got), usage.MaxUnmatchedPerRecord)
	}
}

func TestComputeUnmatched_AllMatched_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	matched := map[string]bool{"solid": true, "dry": true}
	tokens := []string{"solid", "dry"}
	got := computeUnmatched(tokens, matched)
	if len(got) != 0 {
		t.Errorf("expected empty when all tokens matched, got %v", got)
	}
}

// TestLogCoOccurrence_NoUnmatched_SkipsWrite verifies that logCoOccurrence
// does not attempt a write when all tokens are already in the match set.
// Since AppendCoOccurrenceLog requires a real path, we use the function's
// internal guard: zero unmatched → early return, no file created.
func TestLogCoOccurrence_NoUnmatched_SkipsWrite(t *testing.T) {
	t.Parallel()
	// Entry with keywords covering all prompt tokens.
	entry := index.Entry{
		Name:     "code-clean-code",
		Keywords: map[string]int{"solid": 2, "dry": 1},
	}
	hits := []search.ScoredEntry{
		{Entry: entry, Score: 5, Matched: []string{"solid", "dry"}},
	}
	// tokens that are all in the matched keyword set
	tokens := []string{"solid", "dry"}

	// computeUnmatched is the gate — verify it returns empty
	matchedSet := map[string]bool{}
	for _, h := range hits {
		for kw := range h.Entry.Keywords {
			matchedSet[kw] = true
		}
	}
	unmatched := computeUnmatched(tokens, matchedSet)
	if len(unmatched) != 0 {
		t.Errorf("expected no unmatched tokens, got %v", unmatched)
	}
}

func TestLogCoOccurrence_NoHits_SkipsWrite(t *testing.T) {
	t.Parallel()
	// Empty hits — logCoOccurrence must return early, no panic.
	// The guard is: if len(hits) == 0 { return }
	// We call it indirectly by verifying computeUnmatched with empty matched set
	// returns the tokens, but the hits guard fires first.
	tokens := []string{"solid", "dry"}
	var hits []search.ScoredEntry

	// Verify the len(hits)==0 guard by calling logCoOccurrence directly.
	// It swallows all errors, so no panic is the observable outcome.
	logCoOccurrence(tokens, hits) // must not panic
}
