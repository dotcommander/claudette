package search

import "github.com/dotcommander/claudette/internal/index"

// Corpus is the minimal surface search.Run needs from an indexed document set.
// Exposes IDFMap (whole map) rather than per-token IDF(token) because
// ScoreExplained consumes the map internally; per-token lookup would force a
// scorer rewrite with no benefit.
type Corpus interface {
	Entries() []index.Entry
	IDFMap() map[string]float64
	AvgFieldLength() float64
}

// InMemoryCorpus is a fixture Corpus. NewInMemoryCorpus computes IDF and
// AvgFieldLen from the supplied entries.
type InMemoryCorpus struct {
	entries     []index.Entry
	idf         map[string]float64
	avgFieldLen float64
}

func (c *InMemoryCorpus) Entries() []index.Entry     { return c.entries }
func (c *InMemoryCorpus) IDFMap() map[string]float64 { return c.idf }
func (c *InMemoryCorpus) AvgFieldLength() float64    { return c.avgFieldLen }

// NewInMemoryCorpus builds a Corpus from raw entries, computing IDF and
// AvgFieldLen fresh. Use for standalone test fixtures.
func NewInMemoryCorpus(entries ...index.Entry) *InMemoryCorpus {
	return &InMemoryCorpus{
		entries:     entries,
		idf:         index.ComputeIDF(entries),
		avgFieldLen: index.ComputeAvgFieldLen(entries),
	}
}

// NewCorpus builds a Corpus with caller-supplied IDF and AvgFieldLen.
// Used when entries are a filtered subset of a larger corpus and callers
// must preserve the parent corpus's statistics (e.g. search --filter=skill).
func NewCorpus(entries []index.Entry, idf map[string]float64, avgFieldLen float64) Corpus {
	return &InMemoryCorpus{entries: entries, idf: idf, avgFieldLen: avgFieldLen}
}

// indexCorpus wraps *index.Index because Index.Entries is a field — cannot
// add an Entries() method of the same name directly to *Index.
type indexCorpus struct{ idx *index.Index }

func (c *indexCorpus) Entries() []index.Entry     { return c.idx.Entries }
func (c *indexCorpus) IDFMap() map[string]float64 { return c.idx.IDF }
func (c *indexCorpus) AvgFieldLength() float64    { return c.idx.AvgFieldLen }

// CorpusFromIndex wraps a loaded *index.Index as a Corpus.
func CorpusFromIndex(idx *index.Index) Corpus { return &indexCorpus{idx: idx} }
