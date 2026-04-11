package search

import (
	"cmp"
	"slices"
	"strings"

	"github.com/dotcommander/claudette/internal/index"
)

// ScoredEntry pairs an entry with its relevance score.
type ScoredEntry struct {
	Entry   index.Entry `json:"entry"`
	Score   int         `json:"score"`
	Matched []string    `json:"matched"`
}

// Score computes relevance scores for all entries against tokenized prompt.
// Returns entries with score >= threshold, sorted by score descending.
func Score(entries []index.Entry, tokens []string, threshold int) []ScoredEntry {
	if len(tokens) == 0 {
		return nil
	}

	var results []ScoredEntry

	for _, entry := range entries {
		score := 0
		var matched []string

		for _, tok := range tokens {
			// Category alias boost: +2
			if canonical, ok := CategoryAlias(tok); ok && canonical == entry.Category {
				score += 2
				matched = append(matched, tok)
			}

			// Direct keyword match: +1
			if slices.Contains(entry.Keywords, tok) {
				score++
				matched = append(matched, tok)
				continue
			}
			// Plural normalization
			if slices.Contains(entry.Keywords, tok+"s") {
				score++
				matched = append(matched, tok)
				continue
			}
			if s, ok := strings.CutSuffix(tok, "s"); ok {
				if slices.Contains(entry.Keywords, s) {
					score++
					matched = append(matched, tok)
				}
			}
		}

		if score >= threshold {
			results = append(results, ScoredEntry{
				Entry:   entry,
				Score:   score,
				Matched: dedup(matched),
			})
		}
	}

	slices.SortFunc(results, func(a, b ScoredEntry) int {
		if a.Score != b.Score {
			return cmp.Compare(b.Score, a.Score) // descending
		}
		return cmp.Compare(a.Entry.Name, b.Entry.Name)
	})

	return results
}

// ScoreTop returns at most limit results.
func ScoreTop(entries []index.Entry, tokens []string, threshold, limit int) []ScoredEntry {
	results := Score(entries, tokens, threshold)
	if len(results) > limit {
		return results[:limit]
	}
	return results
}

// FilterByType returns only entries matching the given type.
func FilterByType(entries []index.Entry, t index.EntryType) []index.Entry {
	var out []index.Entry
	for _, e := range entries {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

func dedup(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
