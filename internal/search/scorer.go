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

// matchTerm records a matched token and its score contribution.
type matchTerm struct {
	term  string
	delta float64
}

// minIDFForMatch is the minimum per-term IDF contribution required for a result
// to be included. Entries that match only via very common (low-IDF) terms are suppressed.
// Bigram matches bypass this gate — positional evidence is already discriminating.
const minIDFForMatch = 1.0

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
		score, matchTerms, bigramHit, maxIDF := scoreEntry(entry, tokens, promptBigrams, idf, avgdl)
		if score < threshF {
			continue
		}
		// IDF gate: suppress entries matched only via common terms, unless a bigram
		// hit is present (positional evidence) or no IDF map is configured.
		if idf != nil && !bigramHit && maxIDF < minIDFForMatch {
			continue
		}
		results = append(results, ScoredEntry{
			Entry:   entry,
			Score:   int(math.Round(score)),
			Matched: sortedDedupMatchTerms(matchTerms),
		})
	}

	slices.SortFunc(results, func(a, b ScoredEntry) int {
		if a.Score != b.Score {
			return cmp.Compare(b.Score, a.Score) // descending
		}
		return cmp.Compare(a.Entry.Name, b.Entry.Name)
	})

	return results
}

// scoreEntry computes the score and matched terms for a single entry.
// Returns: total score, matched terms with deltas, whether any bigram matched, max per-term IDF contribution.
func scoreEntry(entry index.Entry, tokens []string, promptBigrams []string, idf map[string]float64, avgdl float64) (float64, []matchTerm, bool, float64) {
	var score float64
	var terms []matchTerm
	var maxIDF float64
	var bigramHit bool

	dl := float64(len(entry.Keywords))
	for _, tok := range tokens {
		delta, hit, aliasHit := scoreToken(entry, tok, idf, dl, avgdl)
		score += delta
		if hit {
			terms = append(terms, matchTerm{term: tok, delta: delta})
			// Track max per-term IDF multiplier for the IDF gate.
			// Alias hits are curated signals — treat them as IDF=minIDFForMatch so
			// they always pass the gate regardless of the token's corpus frequency.
			if aliasHit {
				maxIDF = max(maxIDF, minIDFForMatch)
			} else if v := idfMul(idf, tok); v > maxIDF {
				maxIDF = v
			}
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
				terms = append(terms, matchTerm{term: bg, delta: 3.0})
				bigramHit = true
			}
		}
	}

	return score, terms, bigramHit, maxIDF
}

// scoreToken returns the score contribution, whether the token matched, and
// whether the match was driven by a category alias.
// dl is the document length (len(entry.Keywords)); avgdl is the corpus average.
func scoreToken(entry index.Entry, tok string, idf map[string]float64, dl, avgdl float64) (float64, bool, bool) {
	mul := idfMul(idf, tok)
	var delta float64
	var aliasHit bool

	// Category alias boost: flat +2 (additive — does not short-circuit keyword matching).
	// Fixed multiplier avoids IDF suppressing curated alias signals when the alias
	// token is also a common keyword (e.g. "go" has high df, low IDF, but is a
	// deliberate user intent signal that must not be frequency-dampened).
	if canonical, ok := CategoryAlias(tok); ok && canonical == entry.Category {
		delta += 2.0
		aliasHit = true
	}

	// Direct keyword match: BM25(weight, IDF, dl, avgdl).
	if weight, ok := entry.Keywords[tok]; ok {
		return delta + bm25(weight, mul, dl, avgdl), true, aliasHit
	}

	// Plural normalization: BM25 × 0.9.
	if weight, ok := entry.Keywords[tok+"s"]; ok {
		return delta + bm25(weight, mul, dl, avgdl)*0.9, true, aliasHit
	}
	if stem, ok := strings.CutSuffix(tok, "s"); ok {
		if weight, ok := entry.Keywords[stem]; ok {
			return delta + bm25(weight, mul, dl, avgdl)*0.9, true, aliasHit
		}
	}

	// Stem/prefix match: BM25 × 0.6.
	if len(tok) >= 4 {
		for kw, weight := range entry.Keywords {
			if HasStemMatch(tok, kw) {
				return delta + bm25(weight, mul, dl, avgdl)*0.6, true, aliasHit
			}
		}
	}

	// Return alias-only contribution, if any.
	return delta, delta > 0, aliasHit
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
	if w, ok := idf[token]; ok {
		return w
	}
	return 1.0
}

// sortedDedupMatchTerms deduplicates matched terms, keeping the highest delta
// per term, then sorts by delta descending with alphabetical tiebreak.
func sortedDedupMatchTerms(terms []matchTerm) []string {
	best := make(map[string]float64, len(terms))
	for _, mt := range terms {
		if d, ok := best[mt.term]; !ok || mt.delta > d {
			best[mt.term] = mt.delta
		}
	}

	deduped := make([]matchTerm, 0, len(best))
	for term, delta := range best {
		deduped = append(deduped, matchTerm{term: term, delta: delta})
	}

	slices.SortFunc(deduped, func(a, b matchTerm) int {
		if a.delta != b.delta {
			return cmp.Compare(b.delta, a.delta) // descending
		}
		return cmp.Compare(a.term, b.term) // alphabetical tiebreak
	})

	out := make([]string, len(deduped))
	for i, mt := range deduped {
		out[i] = mt.term
	}
	return out
}
