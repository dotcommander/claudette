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

// bigramFloor is the minimum bonus awarded for a matched bigram.
// Even when both tokens are common (low IDF), positional co-occurrence is real signal.
// Set to half the old flat constant (3.0/2 = 1.5) so common pairs never score worse
// than half the old default; rare pairs score higher via the IDF-weighted average.
const bigramFloor = 1.5

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

// applyUsageBoost multiplies the raw score by a usage-based factor:
//
//	boost = 1 + log1p(decayedHitCount) / 10
//
// decayedHitCount is the time-decayed sum from AggregateDecayedHitCounts —
// recent hits count ~1.0, hits one half-life ago count ~0.5.
// Returns 1.0× when decayedHitCount is 0 or negative. Uses math.Log1p for
// numerical stability near zero. The curve is O(log n) — 0 hits → 1.00x,
// 10 → 1.24x, 100 → 1.46x, 1000 → 1.69x. No ceiling: the log function is
// the ceiling. Applied pre-rounding so sub-unit boosts can still re-rank ties.
func applyUsageBoost(score float64, decayedHitCount float64) float64 {
	if decayedHitCount <= 0 {
		return score
	}
	return score * (1 + math.Log1p(decayedHitCount)/10)
}

// tokenScore is the per-token result used by both the fast path and diagnostics.
type tokenScore struct {
	Delta    float64
	Kind     string // "direct"|"plural"|"stem"|"alias"|"miss" — may be combined e.g. "alias+direct"
	Weight   int    // field weight from entry.Keywords map (0 for alias-only/miss)
	AliasCat string // canonical category this token aliased to, if any
}

// EntryDiagnostics captures everything the scorer computed for a single entry.
// Populated unconditionally; Score discards the details, ScoreExplained keeps them.
//
// Suppressed is the human-readable reason this entry would NOT appear in Score's
// output. Empty string means the entry passed all gates. Values:
//
//	"below threshold"     — RawScore (post-boost, pre-round) < threshold
//	"idf gate: low-idf"   — only matched via common terms, no bigram
//
// Never set by the scorer itself — filled in by ScoreExplained based on the
// same conditions Score uses.
type EntryDiagnostics struct {
	Entry        index.Entry `json:"entry"`
	RawScore     float64     `json:"raw_score"`     // pre-round, post-usage-boost
	FinalScore   int         `json:"final_score"`   // math.Round(RawScore)
	TokenHits    []TokenHit  `json:"token_hits"`    // one per prompt token (includes misses)
	BigramHits   []string    `json:"bigram_hits"`   // prompt bigrams that matched this entry
	BigramDeltas []float64   `json:"bigram_deltas"` // IDF-weighted bonus per matched bigram (parallel to BigramHits)
	MaxIDF       float64     `json:"max_idf"`       // best per-term IDF contribution (for gate)
	UsageBoost   float64     `json:"usage_boost"`   // multiplier applied (1.0 if hit_count=0)
	PreBoostSum  float64     `json:"pre_boost_sum"` // score before applyUsageBoost
	Suppressed   string      `json:"suppressed,omitzero"`
}

// TokenHit is one prompt token's interaction with one entry.
// Kind values: "direct" | "plural" | "stem" | "alias" | "alias+direct" |
// "alias+plural" | "alias+stem" | "miss". Multiple mechanisms may fire for
// one token (alias + keyword additive); Kind joins them with "+" in that case.
type TokenHit struct {
	Token    string  `json:"token"`
	Kind     string  `json:"kind"`
	Weight   int     `json:"weight,omitzero"`    // field weight from entry.Keywords (0 for alias-only, miss)
	IDF      float64 `json:"idf"`                // IDF multiplier used (1.0 if no IDF map)
	Delta    float64 `json:"delta"`              // this token's contribution to score
	AliasCat string  `json:"alias_cat,omitzero"` // canonical category alias resolved to, if any
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

// ScoreExplained returns diagnostics for every entry, including suppressed ones.
// Unlike Score, it does not filter by threshold or the IDF gate — callers get
// the complete picture, with Suppressed populated for entries Score would drop.
// Sort order: kept entries (score desc, name asc) then suppressed entries (score desc, name asc).
//
// When tokens is empty, returns nil (same contract as Score).
func ScoreExplained(entries []index.Entry, tokens []string, threshold int, idf map[string]float64, avgdl float64) []EntryDiagnostics {
	if len(tokens) == 0 {
		return nil
	}

	promptBigrams := buildBigrams(tokens)
	threshF := float64(threshold)

	diags := make([]EntryDiagnostics, 0, len(entries))
	for _, entry := range entries {
		d := scoreEntryCore(entry, tokens, promptBigrams, idf, avgdl)

		bigramHit := len(d.BigramHits) > 0
		switch {
		case d.RawScore < threshF:
			d.Suppressed = "below threshold"
		case idf != nil && !bigramHit && d.MaxIDF < minIDFForMatch:
			d.Suppressed = "idf gate: low-idf"
		}
		diags = append(diags, d)
	}

	// Sort: kept entries first (score desc, name asc), then suppressed (score desc, name asc).
	slices.SortFunc(diags, func(a, b EntryDiagnostics) int {
		aSupp := a.Suppressed != ""
		bSupp := b.Suppressed != ""
		if aSupp != bSupp {
			if bSupp {
				return -1 // a kept, b suppressed → a first
			}
			return 1
		}
		if a.FinalScore != b.FinalScore {
			return cmp.Compare(b.FinalScore, a.FinalScore) // descending
		}
		return cmp.Compare(a.Entry.Name, b.Entry.Name)
	})

	return diags
}

// scoreEntryCore is the single scoring implementation. Always fully populates
// EntryDiagnostics. Callers drop unused fields.
func scoreEntryCore(entry index.Entry, tokens []string, promptBigrams []string, idf map[string]float64, avgdl float64) EntryDiagnostics {
	var preBoostSum float64
	var maxIDF float64
	hits := make([]TokenHit, 0, len(tokens))

	dl := float64(len(entry.Keywords))
	idfMulFn := func(tok string) float64 { return idfMul(idf, tok) }

	for _, tok := range tokens {
		ts := scoreToken(entry, tok, idf, dl, avgdl)
		preBoostSum += ts.Delta

		mul := idfMulFn(tok)
		if ts.Kind != "miss" {
			// Track max per-term IDF multiplier for the IDF gate.
			// Alias hits are curated signals — treat them as IDF=minIDFForMatch so
			// they always pass the gate regardless of the token's corpus frequency.
			if strings.Contains(ts.Kind, "alias") {
				maxIDF = max(maxIDF, minIDFForMatch)
			} else {
				maxIDF = max(maxIDF, mul)
			}
		}

		hits = append(hits, TokenHit{
			Token:    tok,
			Kind:     ts.Kind,
			Weight:   ts.Weight,
			IDF:      mul,
			Delta:    ts.Delta,
			AliasCat: ts.AliasCat,
		})
	}

	// Bigram matching: IDF-weighted average bonus, floored at bigramFloor.
	// rare-rare pairs score higher than common-common pairs; positional evidence
	// still earns at least bigramFloor regardless of term frequency.
	var bigramHits []string
	var bigramDeltas []float64
	if len(entry.Bigrams) > 0 && len(promptBigrams) > 0 {
		for _, pb := range promptBigrams {
			for _, eb := range entry.Bigrams {
				if pb == eb {
					tok1, tok2, _ := strings.Cut(pb, " ")
					bonus := math.Max(bigramFloor, (idfMul(idf, tok1)+idfMul(idf, tok2))/2)
					preBoostSum += bonus
					bigramHits = append(bigramHits, pb)
					bigramDeltas = append(bigramDeltas, bonus)
					break // each prompt bigram counts once
				}
			}
		}
	}

	rawScore := applyUsageBoost(preBoostSum, entry.HitCountDecayed)
	var usageBoost float64
	if preBoostSum == 0 {
		usageBoost = 1.0
	} else {
		usageBoost = rawScore / preBoostSum
	}

	return EntryDiagnostics{
		Entry:        entry,
		RawScore:     rawScore,
		FinalScore:   int(math.Round(rawScore)),
		TokenHits:    hits,
		BigramHits:   bigramHits,
		BigramDeltas: bigramDeltas,
		MaxIDF:       maxIDF,
		UsageBoost:   usageBoost,
		PreBoostSum:  preBoostSum,
	}
}

// scoreEntry is the Score-path adapter: same return tuple as before.
// Kept so Score's call site is untouched; this is a 5-line forwarder, not
// a parallel implementation.
func scoreEntry(entry index.Entry, tokens []string, promptBigrams []string, idf map[string]float64, avgdl float64) (float64, []matchTerm, bool, float64) {
	d := scoreEntryCore(entry, tokens, promptBigrams, idf, avgdl)
	terms := make([]matchTerm, 0, len(d.TokenHits)+len(d.BigramHits))
	for _, h := range d.TokenHits {
		if h.Kind != "miss" {
			terms = append(terms, matchTerm{term: h.Token, delta: h.Delta})
		}
	}
	for i, bg := range d.BigramHits {
		terms = append(terms, matchTerm{term: bg, delta: d.BigramDeltas[i]})
	}
	return d.RawScore, terms, len(d.BigramHits) > 0, d.MaxIDF
}

// scoreToken returns the score contribution and what kind of match fired.
// dl is the document length (len(entry.Keywords)); avgdl is the corpus average.
//
// Matching order: category alias → Keywords (direct, plural, stem) → SuggestedAliases.
// SuggestedAliases use weight=1 and no double-count: a token matched by Keywords
// is never re-scored via SuggestedAliases.
func scoreToken(entry index.Entry, tok string, idf map[string]float64, dl, avgdl float64) tokenScore {
	mul := idfMul(idf, tok)
	var delta float64
	var aliasCat string
	var kindParts []string

	// Category alias boost: flat +2 (additive — does not short-circuit keyword matching).
	// Fixed multiplier avoids IDF suppressing curated alias signals when the alias
	// token is also a common keyword (e.g. "go" has high df, low IDF, but is a
	// deliberate user intent signal that must not be frequency-dampened).
	if canonical, ok := CategoryAlias(tok); ok && canonical == entry.Category {
		delta += 2.0
		aliasCat = canonical
		kindParts = append(kindParts, "alias")
	}

	// Direct keyword match: BM25(weight, IDF, dl, avgdl).
	if weight, ok := entry.Keywords[tok]; ok {
		kindStr := strings.Join(append(kindParts, "direct"), "+")
		if len(kindParts) == 0 {
			kindStr = "direct"
		}
		return tokenScore{Delta: delta + bm25(weight, mul, dl, avgdl), Kind: kindStr, Weight: weight, AliasCat: aliasCat}
	}

	// Plural normalization: BM25 × 0.9.
	if weight, ok := entry.Keywords[tok+"s"]; ok {
		kindStr := strings.Join(append(kindParts, "plural"), "+")
		if len(kindParts) == 0 {
			kindStr = "plural"
		}
		return tokenScore{Delta: delta + bm25(weight, mul, dl, avgdl)*0.9, Kind: kindStr, Weight: weight, AliasCat: aliasCat}
	}
	if stem, ok := strings.CutSuffix(tok, "s"); ok {
		if weight, ok := entry.Keywords[stem]; ok {
			kindStr := strings.Join(append(kindParts, "plural"), "+")
			if len(kindParts) == 0 {
				kindStr = "plural"
			}
			return tokenScore{Delta: delta + bm25(weight, mul, dl, avgdl)*0.9, Kind: kindStr, Weight: weight, AliasCat: aliasCat}
		}
	}

	// Stem/prefix match: BM25 × 0.6.
	if len(tok) >= 4 {
		for kw, weight := range entry.Keywords {
			if HasStemMatch(tok, kw) {
				kindStr := strings.Join(append(kindParts, "stem"), "+")
				if len(kindParts) == 0 {
					kindStr = "stem"
				}
				return tokenScore{Delta: delta + bm25(weight, mul, dl, avgdl)*0.6, Kind: kindStr, Weight: weight, AliasCat: aliasCat}
			}
		}
	}

	// SuggestedAlias match: weight=1, BM25, no double-count (Keywords already returned above).
	// Plural normalization also applies to suggested aliases.
	for _, sa := range entry.SuggestedAliases {
		matched := sa == tok
		if !matched {
			// plural: "foo" matches "foos" and vice versa
			matched = sa == tok+"s" || sa+"s" == tok
		}
		if matched {
			kindStr := strings.Join(append(kindParts, "suggested"), "+")
			if len(kindParts) == 0 {
				kindStr = "suggested"
			}
			return tokenScore{Delta: delta + bm25(1, mul, dl, avgdl), Kind: kindStr, Weight: 1, AliasCat: aliasCat}
		}
	}

	// Return alias-only contribution, if any.
	if delta > 0 {
		return tokenScore{Delta: delta, Kind: strings.Join(kindParts, "+"), AliasCat: aliasCat}
	}
	return tokenScore{Kind: "miss"}
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
