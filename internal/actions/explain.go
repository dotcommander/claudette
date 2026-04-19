package actions

import (
	"fmt"
	"io"

	"github.com/dotcommander/claudette/internal/output"
	"github.com/dotcommander/claudette/internal/search"
)

// ExplainOpts controls explain behavior from CLI flags.
type ExplainOpts struct {
	JSON  bool
	Limit int // max entries to show in the detailed breakdown; 0 means all
}

// NewExplainOpts returns defaults. Limit defaults to search.DefaultLimit (5).
// Threshold is intentionally fixed at search.DefaultThreshold inside Explain
// — the output marks entries as suppressed below it rather than dropping them,
// so there is no user-facing reason to override the threshold.
func NewExplainOpts() ExplainOpts {
	return ExplainOpts{Limit: search.DefaultLimit}
}

// Explain tokenizes the prompt, runs the diagnostic scorer, and writes a
// per-token / per-entry breakdown. Shows both kept and suppressed entries so
// users can see WHY expected matches didn't appear.
//
// Empty prompt → error (debugging flow; never silently skip).
// Slash-prefixed prompt → proceeds; explain is for debugging and the user
// asked for this exact prompt.
func Explain(w io.Writer, prompt string, opts ExplainOpts) error {
	if prompt == "" {
		return fmt.Errorf("explain: prompt must not be empty")
	}

	idx, err := LoadIndex()
	if err != nil {
		return err
	}

	stopWords := search.DefaultStopWords()
	rawTokens := search.Tokenize(prompt, nil)    // pre-stop-word for display
	tokens := search.Tokenize(prompt, stopWords) // what the scorer actually uses
	diags := search.ScoreExplained(idx.Entries, tokens, search.DefaultThreshold, idx.IDF, idx.AvgFieldLen)

	limit := opts.Limit
	if limit <= 0 || limit > len(diags) {
		limit = len(diags)
	}

	report := output.ExplainReport{
		Prompt:       prompt,
		RawTokens:    rawTokens,
		KeptTokens:   tokens,
		DroppedStops: diffTokens(rawTokens, tokens),
		AvgFieldLen:  idx.AvgFieldLen,
		HasIDF:       idx.IDF != nil,
		Diagnostics:  diags[:limit],
		TotalScored:  len(diags),
	}

	if opts.JSON {
		return output.WriteExplainJSON(w, report)
	}
	output.WriteExplainText(w, report)
	return nil
}

// diffTokens returns the elements in a that are not in b (set difference).
// Used to compute which stop words were filtered: diff(raw, kept) = dropped.
func diffTokens(a, b []string) []string {
	inB := make(map[string]int, len(b))
	for _, t := range b {
		inB[t]++
	}
	var out []string
	for _, t := range a {
		if inB[t] > 0 {
			inB[t]--
		} else {
			out = append(out, t)
		}
	}
	return out
}
