package search

import (
	"testing"

	"github.com/dotcommander/claudette/internal/index"
)

func TestNewInMemoryCorpus_ComputesStats(t *testing.T) {
	t.Parallel()
	entries := baseEntries()
	c := NewInMemoryCorpus(entries...)

	wantIDF := index.ComputeIDF(entries)
	wantAvg := index.ComputeAvgFieldLen(entries)

	if len(c.IDFMap()) != len(wantIDF) {
		t.Errorf("IDFMap len: got %d, want %d", len(c.IDFMap()), len(wantIDF))
	}
	for tok, wv := range wantIDF {
		if gv, ok := c.IDFMap()[tok]; !ok || gv != wv {
			t.Errorf("IDF[%q]: got %v, want %v", tok, gv, wv)
		}
	}
	if c.AvgFieldLength() != wantAvg {
		t.Errorf("AvgFieldLength: got %v, want %v", c.AvgFieldLength(), wantAvg)
	}
	if len(c.Entries()) != len(entries) {
		t.Errorf("Entries len: got %d, want %d", len(c.Entries()), len(entries))
	}
}

func TestNewInMemoryCorpus_Empty(t *testing.T) {
	t.Parallel()
	c := NewInMemoryCorpus()
	if len(c.Entries()) != 0 {
		t.Errorf("expected 0 entries, got %d", len(c.Entries()))
	}
	if c.IDFMap() != nil {
		t.Errorf("expected nil IDFMap for empty corpus, got %v", c.IDFMap())
	}
	if c.AvgFieldLength() != 0 {
		t.Errorf("expected 0 AvgFieldLength for empty corpus, got %v", c.AvgFieldLength())
	}
}

func TestNewCorpus_PreservesSuppliedStats(t *testing.T) {
	t.Parallel()
	entries := baseEntries()
	customIDF := map[string]float64{"goroutine": 1.5, "channel": 0.8}
	customAvg := 42.0

	c := NewCorpus(entries, customIDF, customAvg)

	if c.AvgFieldLength() != customAvg {
		t.Errorf("AvgFieldLength: got %v, want %v", c.AvgFieldLength(), customAvg)
	}
	if len(c.IDFMap()) != len(customIDF) {
		t.Errorf("IDFMap len: got %d, want %d", len(c.IDFMap()), len(customIDF))
	}
	if c.IDFMap()["goroutine"] != 1.5 {
		t.Errorf("IDFMap[goroutine]: got %v, want 1.5", c.IDFMap()["goroutine"])
	}
	if len(c.Entries()) != len(entries) {
		t.Errorf("Entries len: got %d, want %d", len(c.Entries()), len(entries))
	}
}

func TestCorpusFromIndex_DelegatesToIndex(t *testing.T) {
	t.Parallel()
	entries := baseEntries()
	idf := index.ComputeIDF(entries)
	avg := index.ComputeAvgFieldLen(entries)

	idx := &index.Index{
		Entries:     entries,
		IDF:         idf,
		AvgFieldLen: avg,
	}
	c := CorpusFromIndex(idx)

	if len(c.Entries()) != len(idx.Entries) {
		t.Errorf("Entries: got %d, want %d", len(c.Entries()), len(idx.Entries))
	}
	if len(c.IDFMap()) != len(idx.IDF) {
		t.Errorf("IDFMap len: got %d, want %d", len(c.IDFMap()), len(idx.IDF))
	}
	if c.AvgFieldLength() != idx.AvgFieldLen {
		t.Errorf("AvgFieldLength: got %v, want %v", c.AvgFieldLength(), idx.AvgFieldLen)
	}
}

// TestRun_WithInMemoryCorpus_MatchesRawEntries is the drift canary: Run via
// InMemoryCorpus must produce the same Scored results as Score called directly
// with the same entries and stats.
func TestRun_WithInMemoryCorpus_MatchesRawEntries(t *testing.T) {
	t.Parallel()
	entries := baseEntries()
	tokens := []string{"goroutine", "channel"}

	idf := index.ComputeIDF(entries)
	avg := index.ComputeAvgFieldLen(entries)

	direct := Score(entries, tokens, DefaultThreshold, idf, avg)
	pr := Run(PipelineInput{
		Tokens:     tokens,
		Corpus:     NewInMemoryCorpus(entries...),
		Threshold:  DefaultThreshold,
		Limit:      0,
		ApplyGates: false,
	})

	if len(pr.Scored) != len(direct) {
		t.Fatalf("Scored len: got %d, want %d", len(pr.Scored), len(direct))
	}
	for i := range pr.Scored {
		if pr.Scored[i].Entry.Name != direct[i].Entry.Name {
			t.Errorf("[%d] name: got %q, want %q", i, pr.Scored[i].Entry.Name, direct[i].Entry.Name)
		}
		if pr.Scored[i].Score != direct[i].Score {
			t.Errorf("[%d] %s score: got %d, want %d", i, direct[i].Entry.Name, pr.Scored[i].Score, direct[i].Score)
		}
	}
}
