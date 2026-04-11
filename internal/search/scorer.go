package search

import (
	"cmp"
	"math"
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

// bm25 computes the BM25 term score for a single keyword occurrence.
// weight is the field weight (name=3, title=2, tag=2, desc=1).
// idf is the dampened inverse document frequency multiplier.
// dl is the document length (number of keywords in this entry).
// avgdl is the average document length across all entries.
// When avgdl is 0 (single-entry corpus or unset), falls back to weight×idf.
func bm25(weight int, idf, dl, avgdl float64) float64 {
	const k1, b = 1.2, 0.75
	if avgdl == 0 {
		return float64(weight) * idf
	}
	tf := float64(weight)
	return tf * (k1 + 1) / (tf + k1*(1-b+b*dl/avgdl)) * idf
}

// Score computes relevance scores for all entries against tokenized prompt.
// Uses field-weighted keywords, IDF multipliers, BM25 saturation, and bigrams.
// Returns entries with score >= threshold, sorted by score descending.
func Score(entries []index.Entry, tokens []string, threshold int, idf map[string]float64, avgdl float64) []ScoredEntry {
	if len(tokens) == 0 {
		return nil
	}

	// Pre-build prompt bigrams once rather than per-entry.
	promptBigrams := buildBigrams(tokens)

	threshF := float64(threshold)
	var results []ScoredEntry

	for _, entry := range entries {
		score, matched := scoreEntry(entry, tokens, promptBigrams, idf, avgdl)
		if score >= threshF {
			results = append(results, ScoredEntry{
				Entry:   entry,
				Score:   int(math.Round(score)),
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

// scoreEntry computes the score and matched tokens for a single entry.
func scoreEntry(entry index.Entry, tokens []string, promptBigrams []string, idf map[string]float64, avgdl float64) (float64, []string) {
	var score float64
	var matched []string

	dl := float64(len(entry.Keywords))
	for _, tok := range tokens {
		delta, hit := scoreToken(entry, tok, idf, dl, avgdl)
		score += delta
		if hit {
			matched = append(matched, tok)
		}
	}

	// Bigram matching: flat +3 per matched bigram.
	// Build a set from entry bigrams for O(1) lookup.
	if len(entry.Bigrams) > 0 && len(promptBigrams) > 0 {
		entryBigramSet := make(map[string]struct{}, len(entry.Bigrams))
		for _, bg := range entry.Bigrams {
			entryBigramSet[bg] = struct{}{}
		}
		for _, bg := range promptBigrams {
			if _, ok := entryBigramSet[bg]; ok {
				score += 3.0
				matched = append(matched, bg)
			}
		}
	}

	return score, matched
}

// scoreToken returns the score contribution and whether the token matched.
// dl is the document length (len(entry.Keywords)); avgdl is the corpus average.
func scoreToken(entry index.Entry, tok string, idf map[string]float64, dl, avgdl float64) (float64, bool) {
	mul := idfMul(idf, tok)
	var delta float64

	// Category alias boost: +2 × IDF (additive — does not short-circuit keyword matching).
	if canonical, ok := CategoryAlias(tok); ok && canonical == entry.Category {
		delta += 2.0 * mul
	}

	// Direct keyword match: BM25(weight, IDF, dl, avgdl).
	if weight, ok := entry.Keywords[tok]; ok {
		return delta + bm25(weight, mul, dl, avgdl), true
	}

	// Plural normalization: BM25 × 0.9.
	if weight, ok := entry.Keywords[tok+"s"]; ok {
		return delta + bm25(weight, mul, dl, avgdl)*0.9, true
	}
	if stem, ok := strings.CutSuffix(tok, "s"); ok {
		if weight, ok := entry.Keywords[stem]; ok {
			return delta + bm25(weight, mul, dl, avgdl)*0.9, true
		}
	}

	// Stem/prefix match: BM25 × 0.6.
	if len(tok) >= 4 {
		for kw, weight := range entry.Keywords {
			if HasStemMatch(tok, kw) {
				return delta + bm25(weight, mul, dl, avgdl)*0.6, true
			}
		}
	}

	// Return alias-only contribution, if any.
	return delta, delta > 0
}

// buildBigrams returns consecutive token pairs.
func buildBigrams(tokens []string) []string {
	if len(tokens) < 2 {
		return nil
	}
	out := make([]string, len(tokens)-1)
	for i := range len(tokens) - 1 {
		out[i] = tokens[i] + " " + tokens[i+1]
	}
	return out
}

// ScoreTop returns at most limit results.
func ScoreTop(entries []index.Entry, tokens []string, threshold, limit int, idf map[string]float64, avgdl float64) []ScoredEntry {
	results := Score(entries, tokens, threshold, idf, avgdl)
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

// idfMul returns the IDF multiplier for a token, defaulting to 1.0 for unknown tokens.
func idfMul(idf map[string]float64, token string) float64 {
	if idf == nil {
		return 1.0
	}
	if w, ok := idf[token]; ok {
		return w
	}
	return 1.0
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
