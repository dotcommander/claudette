package search

import (
	"testing"

	"github.com/dotcommander/claudette/internal/index"
)

func TestRun_EmptyTokens(t *testing.T) {
	t.Parallel()
	pr := Run(PipelineInput{
		Tokens:    nil,
		Entries:   baseEntries(),
		Threshold: DefaultThreshold,
		Limit:     DefaultLimit,
	})
	if pr.Tokens != nil {
		t.Errorf("expected nil Tokens, got %v", pr.Tokens)
	}
	if len(pr.Diagnostics) != 0 {
		t.Errorf("expected 0 Diagnostics, got %d", len(pr.Diagnostics))
	}
	if len(pr.Scored) != 0 {
		t.Errorf("expected 0 Scored, got %d", len(pr.Scored))
	}
	if len(pr.AboveThreshold) != 0 {
		t.Errorf("expected 0 AboveThreshold, got %d", len(pr.AboveThreshold))
	}
	if len(pr.Surviving) != 0 {
		t.Errorf("expected 0 Surviving, got %d", len(pr.Surviving))
	}
	if pr.Suppression != GateReasonNone {
		t.Errorf("expected GateReasonNone, got %q", pr.Suppression)
	}
}

func TestRun_NoGates_HighScore(t *testing.T) {
	t.Parallel()
	// Multi-hit high-score: ApplyGates=false → Surviving == AboveThreshold, Suppression == None.
	entries := []index.Entry{
		makeEntry("e1", "Entry One", "misc", map[string]int{"goroutine": 3, "channel": 2}, nil),
		makeEntry("e2", "Entry Two", "misc", map[string]int{"goroutine": 2}, nil),
	}
	pr := Run(PipelineInput{
		Tokens:     []string{"goroutine", "channel"},
		Entries:    entries,
		Threshold:  1,
		Limit:      0,
		ApplyGates: false,
	})
	if len(pr.Surviving) != len(pr.AboveThreshold) {
		t.Errorf("without gates, Surviving (%d) must equal AboveThreshold (%d)",
			len(pr.Surviving), len(pr.AboveThreshold))
	}
	if pr.Suppression != GateReasonNone {
		t.Errorf("expected GateReasonNone, got %q", pr.Suppression)
	}
}

func TestRun_Gates_HighConfidence_PassThrough(t *testing.T) {
	t.Parallel()
	// High-confidence corpus (scores well above gate thresholds) → pass-through.
	entries := []index.Entry{
		makeEntry("e1", "Entry One", "misc", map[string]int{"goroutine": 3, "channel": 3}, nil),
		makeEntry("e2", "Entry Two", "misc", map[string]int{"goroutine": 3, "channel": 3}, nil),
	}
	pr := Run(PipelineInput{
		Tokens:     []string{"goroutine", "channel"},
		Entries:    entries,
		Threshold:  1,
		Limit:      0,
		ApplyGates: true,
	})
	// Both entries should survive (high scores, clear gap or above ceiling).
	if len(pr.Surviving) == 0 {
		t.Errorf("expected surviving results, got none")
	}
	if pr.Suppression != GateReasonNone && pr.Suppression != GateReasonDifferential {
		t.Errorf("unexpected suppression reason %q for high-confidence corpus", pr.Suppression)
	}
}

func TestRun_Gates_LowConfidence(t *testing.T) {
	t.Parallel()
	// Score 3 < DefaultThreshold(2)*DefaultConfidenceMultiplier(2)=4 → GateReasonLowConfidence.
	entries := []index.Entry{
		makeEntry("e1", "Entry One", "misc", map[string]int{"pipe": 1, "tok": 1}, nil),
	}
	pr := Run(PipelineInput{
		Tokens:     []string{"pipe", "tok"},
		Entries:    entries,
		Threshold:  1,
		Limit:      0,
		ApplyGates: true,
	})
	if pr.Suppression != GateReasonLowConfidence {
		t.Errorf("expected GateReasonLowConfidence, got %q (AboveThreshold scores: %v)",
			pr.Suppression, scoresOf(pr.AboveThreshold))
	}
	if len(pr.Surviving) != 0 {
		t.Errorf("expected 0 surviving after low-confidence gate, got %d", len(pr.Surviving))
	}
}

func TestRun_Gates_SingleTokenFloor(t *testing.T) {
	t.Parallel()
	// Single-token match with score 5 >= 4 (low-conf passes) but < 8 (floor) → SingleTokenFloor.
	// Weight=2 with nil IDF → BM25 ≈ 2. Use weight=3 tokens on a small corpus.
	// We can't easily manufacture a score of exactly 5 with these entries, so use
	// makeScoredEntries-style approach: call ApplyGates directly with a known ScoredEntry.
	// Pipeline test: verify the gate wires through correctly for the single-token path.
	hits := []ScoredEntry{
		{
			Entry:   index.Entry{Name: "e1", Title: "E1"},
			Score:   5,
			Matched: []string{"tok1"}, // single token
		},
	}
	surviving, reason := ApplyGates(hits)
	if reason != GateReasonSingleTokenFloor {
		t.Errorf("expected GateReasonSingleTokenFloor, got %q", reason)
	}
	if len(surviving) != 0 {
		t.Errorf("expected 0 surviving, got %d", len(surviving))
	}
}

func TestRun_Gates_Differential(t *testing.T) {
	t.Parallel()
	// Scores [6,5]: top=6 < softCeiling(10), gap=1 < minScoreGap(2) → Differential, len==1.
	hits := makeScoredEntries(6, 5)
	surviving, reason := ApplyGates(hits)
	if len(surviving) != 1 {
		t.Errorf("differential: expected 1 surviving, got %d", len(surviving))
	}
	if reason != GateReasonDifferential {
		t.Errorf("expected GateReasonDifferential, got %q", reason)
	}
}

func TestRun_Limit_Zero_NoCap(t *testing.T) {
	t.Parallel()
	// Limit=0 means no cap — all above-threshold results are returned.
	entries := []index.Entry{
		makeEntry("e1", "E1", "misc", map[string]int{"pipe": 3}, nil),
		makeEntry("e2", "E2", "misc", map[string]int{"pipe": 3}, nil),
		makeEntry("e3", "E3", "misc", map[string]int{"pipe": 3}, nil),
		makeEntry("e4", "E4", "misc", map[string]int{"pipe": 3}, nil),
		makeEntry("e5", "E5", "misc", map[string]int{"pipe": 3}, nil),
	}
	pr := Run(PipelineInput{
		Tokens:     []string{"pipe"},
		Entries:    entries,
		Threshold:  1,
		Limit:      0,
		ApplyGates: false,
	})
	if len(pr.AboveThreshold) != 5 {
		t.Errorf("Limit=0: expected 5 results, got %d", len(pr.AboveThreshold))
	}
}

func TestRun_Limit_Capping(t *testing.T) {
	t.Parallel()
	// Limit=3 with 5 matching entries → len(AboveThreshold)==3.
	entries := []index.Entry{
		makeEntry("e1", "E1", "misc", map[string]int{"pipe": 3}, nil),
		makeEntry("e2", "E2", "misc", map[string]int{"pipe": 3}, nil),
		makeEntry("e3", "E3", "misc", map[string]int{"pipe": 3}, nil),
		makeEntry("e4", "E4", "misc", map[string]int{"pipe": 3}, nil),
		makeEntry("e5", "E5", "misc", map[string]int{"pipe": 3}, nil),
	}
	pr := Run(PipelineInput{
		Tokens:     []string{"pipe"},
		Entries:    entries,
		Threshold:  1,
		Limit:      3,
		ApplyGates: false,
	})
	if len(pr.AboveThreshold) != 3 {
		t.Errorf("Limit=3: expected 3 AboveThreshold results, got %d", len(pr.AboveThreshold))
	}
}

// TestRun_ScoredMatchesScore is the drift canary: Run().Scored must be byte-identical
// to Score() for the same corpus inputs. Any divergence means the two code paths drifted.
func TestRun_ScoredMatchesScore(t *testing.T) {
	t.Parallel()

	corpora := []struct {
		name    string
		entries []index.Entry
		tokens  []string
		idf     map[string]float64
		avgdl   float64
	}{
		{
			name:    "base entries channel no idf",
			entries: baseEntries(),
			tokens:  []string{"channel"},
		},
		{
			name:    "base entries concurrency idf",
			entries: baseEntries(),
			tokens:  []string{"concurrency"},
			idf:     index.ComputeIDF(baseEntries()),
			avgdl:   index.ComputeAvgFieldLen(baseEntries()),
		},
		{
			name: "bigram match",
			entries: []index.Entry{
				makeEntry("race-entry", "Race Condition Detection", "misc",
					map[string]int{"race": 1, "condition": 1},
					[]string{"race condition"}),
				makeEntry("other", "Other Entry", "misc",
					map[string]int{"pipe": 1}, nil),
			},
			tokens: []string{"race", "condition"},
		},
		{
			name:    "alias match golang to go",
			entries: baseEntries(),
			tokens:  []string{"golang"},
		},
	}

	for _, c := range corpora {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			scoreResults := Score(c.entries, c.tokens, DefaultThreshold, c.idf, c.avgdl)
			pr := Run(PipelineInput{
				Tokens:      c.tokens,
				Entries:     c.entries,
				IDF:         c.idf,
				AvgFieldLen: c.avgdl,
				Threshold:   DefaultThreshold,
				Limit:       0,
				ApplyGates:  false,
			})

			if len(pr.Scored) != len(scoreResults) {
				t.Errorf("Scored=%d, Score=%d; must be equal", len(pr.Scored), len(scoreResults))
				for i, r := range scoreResults {
					t.Logf("Score[%d] name=%s score=%d matched=%v", i, r.Entry.Name, r.Score, r.Matched)
				}
				for i, r := range pr.Scored {
					t.Logf("Run.Scored[%d] name=%s score=%d matched=%v", i, r.Entry.Name, r.Score, r.Matched)
				}
				return
			}

			for i := range pr.Scored {
				s := scoreResults[i]
				r := pr.Scored[i]
				if r.Entry.Name != s.Entry.Name {
					t.Errorf("[%d] name: Run=%q Score=%q", i, r.Entry.Name, s.Entry.Name)
				}
				if r.Score != s.Score {
					t.Errorf("[%d] %s: score Run=%d Score=%d", i, r.Entry.Name, r.Score, s.Score)
				}
				if len(r.Matched) != len(s.Matched) {
					t.Errorf("[%d] %s: matched len Run=%d Score=%d (Run=%v Score=%v)",
						i, r.Entry.Name, len(r.Matched), len(s.Matched), r.Matched, s.Matched)
					continue
				}
				for j := range r.Matched {
					if r.Matched[j] != s.Matched[j] {
						t.Errorf("[%d] %s: Matched[%d] Run=%q Score=%q",
							i, r.Entry.Name, j, r.Matched[j], s.Matched[j])
					}
				}
			}
		})
	}
}

// scoresOf extracts score integers for error messages.
func scoresOf(entries []ScoredEntry) []int {
	out := make([]int, len(entries))
	for i, e := range entries {
		out[i] = e.Score
	}
	return out
}
