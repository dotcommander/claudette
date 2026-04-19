package search

// GateReason identifies why ApplyGates suppressed or truncated results.
type GateReason string

const (
	GateReasonNone             GateReason = ""
	GateReasonLowConfidence    GateReason = "low-confidence"
	GateReasonSingleTokenFloor GateReason = "single-token-floor"
	GateReasonDifferential     GateReason = "differential"
)

// minScoreGap and softCeiling are tuning constants for the differential gate.
// When the top score is below softCeiling and the gap to 2nd place is smaller
// than minScoreGap, confidence is too low to return multiple entries — suppress to 1.
const (
	minScoreGap = 2  // top must lead 2nd by this margin when score is below ceiling
	softCeiling = 10 // at or above this score, confidence is high enough — return all
)

// ApplyGates applies the hook's three post-scoring gates to a ranked result list.
// Input must already be sorted by score descending (as ScoreTop guarantees).
// Returns the surviving entries and the GateReason if any results were suppressed
// or truncated; GateReasonNone means all inputs passed through intact.
//
// Gate order (matches scoreTokens inline logic exactly):
//  1. Low-confidence: top score < DefaultThreshold * DefaultConfidenceMultiplier → drop all.
//  2. Single-token weak: top has <2 matched tokens AND score < DefaultSingleTokenFloor → drop all.
//  3. Differential: top < softCeiling AND gap to #2 < minScoreGap → truncate to 1.
func ApplyGates(hits []ScoredEntry) (surviving []ScoredEntry, reason GateReason) {
	if len(hits) == 0 {
		return hits, GateReasonNone
	}

	top := hits[0]

	if top.Score < DefaultThreshold*DefaultConfidenceMultiplier {
		return nil, GateReasonLowConfidence
	}

	if len(top.Matched) < 2 && top.Score < DefaultSingleTokenFloor {
		return nil, GateReasonSingleTokenFloor
	}

	if len(hits) >= 2 && top.Score < softCeiling && (top.Score-hits[1].Score) < minScoreGap {
		return hits[:1], GateReasonDifferential
	}

	return hits, GateReasonNone
}
