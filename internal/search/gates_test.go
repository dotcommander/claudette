package search

import (
	"testing"

	"github.com/dotcommander/claudette/internal/index"
)

// makeScoredEntries builds a slice of ScoredEntry with the given integer scores.
// Each entry gets two matched tokens so single-token suppression does not interfere.
func makeScoredEntries(scores ...int) []ScoredEntry {
	out := make([]ScoredEntry, len(scores))
	for i, s := range scores {
		out[i] = ScoredEntry{
			Entry:   index.Entry{Name: "entry", Title: "Entry"},
			Score:   s,
			Matched: []string{"tok1", "tok2"},
		}
	}
	return out
}

// makeSingleTokenEntry builds a ScoredEntry with exactly 1 matched token
// so the single-token gate can fire.
func makeSingleTokenEntry(score int) ScoredEntry {
	return ScoredEntry{
		Entry:   index.Entry{Name: "entry", Title: "Entry"},
		Score:   score,
		Matched: []string{"tok1"},
	}
}

func TestApplyGates_EmptyInput(t *testing.T) {
	t.Parallel()
	got, reason := ApplyGates(nil)
	if len(got) != 0 {
		t.Errorf("empty input: expected 0 results, got %d", len(got))
	}
	if reason != GateReasonNone {
		t.Errorf("empty input: expected GateReasonNone, got %q", reason)
	}
}

func TestApplyGates_PassThrough(t *testing.T) {
	t.Parallel()
	// Scores well above all gate thresholds — should pass all gates unchanged.
	hits := makeScoredEntries(15, 10)
	got, reason := ApplyGates(hits)
	if len(got) != 2 {
		t.Errorf("pass-through: expected 2, got %d", len(got))
	}
	if reason != GateReasonNone {
		t.Errorf("pass-through: expected GateReasonNone, got %q", reason)
	}
}

func TestApplyGates_LowConfidence(t *testing.T) {
	t.Parallel()
	// Top score = 1 < DefaultThreshold(2) * DefaultConfidenceMultiplier(2) = 4
	// Gate 1 fires → drop all.
	hits := makeScoredEntries(3)
	got, reason := ApplyGates(hits)
	if len(got) != 0 {
		t.Errorf("low-confidence: expected 0 results, got %d", len(got))
	}
	if reason != GateReasonLowConfidence {
		t.Errorf("low-confidence: expected %q, got %q", GateReasonLowConfidence, reason)
	}
}

func TestApplyGates_SingleTokenFloor(t *testing.T) {
	t.Parallel()
	// Single matched token, score = 5 < DefaultSingleTokenFloor(8).
	// Gate 1 passes (5 >= 4); gate 2 fires → drop all.
	entry := makeSingleTokenEntry(5)
	got, reason := ApplyGates([]ScoredEntry{entry})
	if len(got) != 0 {
		t.Errorf("single-token-floor: expected 0 results, got %d", len(got))
	}
	if reason != GateReasonSingleTokenFloor {
		t.Errorf("single-token-floor: expected %q, got %q", GateReasonSingleTokenFloor, reason)
	}
}

func TestApplyGates_SingleTokenAboveFloor_Passes(t *testing.T) {
	t.Parallel()
	// Single matched token, score = 9 >= DefaultSingleTokenFloor(8) — gate 2 does not fire.
	entry := makeSingleTokenEntry(9)
	got, reason := ApplyGates([]ScoredEntry{entry})
	if len(got) != 1 {
		t.Errorf("single-token above floor: expected 1 result, got %d", len(got))
	}
	if reason != GateReasonNone {
		t.Errorf("single-token above floor: expected GateReasonNone, got %q", reason)
	}
}

func TestApplyGates_Differential(t *testing.T) {
	t.Parallel()
	// top=6 < softCeiling(10), gap=1 < minScoreGap(2) → truncate to 1.
	hits := makeScoredEntries(6, 5)
	got, reason := ApplyGates(hits)
	if len(got) != 1 {
		t.Errorf("differential: expected 1, got %d", len(got))
	}
	if got[0].Score != 6 {
		t.Errorf("differential: surviving score should be 6, got %d", got[0].Score)
	}
	if reason != GateReasonDifferential {
		t.Errorf("differential: expected %q, got %q", GateReasonDifferential, reason)
	}
}

func TestApplyGates_DifferentialNotFiredAboveCeiling(t *testing.T) {
	t.Parallel()
	// top=10 == softCeiling — strict < means gate 3 does NOT fire.
	hits := makeScoredEntries(10, 9)
	got, reason := ApplyGates(hits)
	if len(got) != 2 {
		t.Errorf("at ceiling: expected 2, got %d", len(got))
	}
	if reason != GateReasonNone {
		t.Errorf("at ceiling: expected GateReasonNone, got %q", reason)
	}
}
