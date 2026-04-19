package actions

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	rawTokens := search.Tokenize(prompt, nil)
	tokens := search.Tokenize(prompt, stopWords)

	pr := search.Run(search.PipelineInput{
		Tokens:     tokens,
		Corpus:     search.CorpusFromIndex(&idx),
		Threshold:  search.DefaultThreshold,
		Limit:      0,
		ApplyGates: true,
	})

	limit := opts.Limit
	if limit <= 0 || limit > len(pr.Diagnostics) {
		limit = len(pr.Diagnostics)
	}

	wouldSurface := make([]output.WouldSurfaceEntry, len(pr.Surviving))
	homePrefix := explainHomePrefix()
	for i, s := range pr.Surviving {
		wouldSurface[i] = output.WouldSurfaceEntry{
			Path:    trimHomePrefix(s.Entry.FilePath, homePrefix),
			Title:   s.Entry.Title,
			Matched: s.Matched,
		}
	}

	report := output.ExplainReport{
		Prompt:            prompt,
		RawTokens:         rawTokens,
		KeptTokens:        tokens,
		DroppedStops:      diffTokens(rawTokens, tokens),
		AvgFieldLen:       idx.AvgFieldLen,
		HasIDF:            idx.IDF != nil,
		Diagnostics:       pr.Diagnostics[:limit],
		TotalScored:       len(pr.Diagnostics),
		WouldSurface:      wouldSurface,
		SuppressionReason: string(pr.Suppression),
	}

	if opts.JSON {
		return output.WriteExplainJSON(w, report)
	}
	output.WriteExplainText(w, report)
	return nil
}

// explainHomePrefix returns the absolute path prefix for ~/.claude/ used to
// produce display paths that match what formatContext emits in full mode.
func explainHomePrefix() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".claude") + string(os.PathSeparator)
	}
	return ""
}

// trimHomePrefix replaces a leading absolute ~/.claude/ prefix with the
// tilde-relative form (~/.claude/…) so display paths match hook full-mode output.
func trimHomePrefix(path, prefix string) string {
	if prefix != "" && strings.HasPrefix(path, prefix) {
		return "~/.claude/" + strings.TrimPrefix(path, prefix)
	}
	return path
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
