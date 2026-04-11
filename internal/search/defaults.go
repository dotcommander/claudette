package search

// DefaultThreshold is the minimum score for an entry to be included in results.
const DefaultThreshold = 2

// DefaultLimit is the maximum number of results returned.
const DefaultLimit = 5

// DefaultConfidenceMultiplier gates hook output: top result must score >= threshold * multiplier.
const DefaultConfidenceMultiplier = 2
