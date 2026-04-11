package search

// DefaultThreshold is the minimum score for an entry to be included in results.
const DefaultThreshold = 2

// DefaultLimit is the maximum number of results returned.
const DefaultLimit = 5

// DefaultConfidenceMultiplier gates hook output: top result must score >= threshold * multiplier.
const DefaultConfidenceMultiplier = 2

// DefaultSingleTokenFloor is the minimum score for results with only one matched token.
// Single-token matches need stronger signal to avoid conversational leakage
// (e.g., "great work on the refactor" -> only "refactor" survives stop-word filtering).
const DefaultSingleTokenFloor = 8
