package search

import (
	"github.com/dotcommander/claudette/internal/index"
)

// PipelineInput is the unified input for Run. Every caller (hook,
// actions/search, actions/explain) composes this and reads the result.
type PipelineInput struct {
	Tokens      []string
	Entries     []index.Entry
	IDF         map[string]float64
	AvgFieldLen float64
	Threshold   int
	Limit       int
	ApplyGates  bool
}

// PipelineResult carries every stage so callers render whichever view they need.
type PipelineResult struct {
	Tokens         []string
	Diagnostics    []EntryDiagnostics
	Scored         []ScoredEntry
	AboveThreshold []ScoredEntry
	Surviving      []ScoredEntry
	Suppression    GateReason
}

// Run executes the full scoring pipeline. Pure function: no I/O, no globals.
func Run(in PipelineInput) PipelineResult {
	if len(in.Tokens) == 0 {
		return PipelineResult{Tokens: in.Tokens}
	}

	diags := ScoreExplained(in.Entries, in.Tokens, in.Threshold, in.IDF, in.AvgFieldLen)

	scored := make([]ScoredEntry, 0, len(diags))
	for _, d := range diags {
		if d.Suppressed != "" {
			continue
		}
		scored = append(scored, ScoredEntry{
			Entry:   d.Entry,
			Score:   d.FinalScore,
			Matched: matchedTermsFromDiagnostic(d),
		})
	}

	aboveThreshold := scored
	if in.Limit > 0 && len(aboveThreshold) > in.Limit {
		aboveThreshold = aboveThreshold[:in.Limit]
	}

	surviving := aboveThreshold
	reason := GateReasonNone
	if in.ApplyGates {
		surviving, reason = ApplyGates(aboveThreshold)
	}

	return PipelineResult{
		Tokens:         in.Tokens,
		Diagnostics:    diags,
		Scored:         scored,
		AboveThreshold: aboveThreshold,
		Surviving:      surviving,
		Suppression:    reason,
	}
}
