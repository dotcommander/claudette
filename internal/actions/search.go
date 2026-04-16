package actions

import (
	"fmt"
	"io"
	"strings"

	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/output"
	"github.com/dotcommander/claudette/internal/search"
)

// SearchOpts controls search behavior from CLI flags.
type SearchOpts struct {
	Format    string
	JSON      bool // --json shorthand: when true, overrides Format to "json"
	Threshold int
	Limit     int
}

// NewSearchOpts returns SearchOpts populated with package defaults.
// Callers (CLI flag binding, programmatic consumers) overlay values on top.
func NewSearchOpts() SearchOpts {
	return SearchOpts{
		Format:    "text",
		Threshold: search.DefaultThreshold,
		Limit:     search.DefaultLimit,
	}
}

// Search tokenizes the prompt, scores against the index, and writes results.
// When opts.JSON is true, output is always JSON regardless of opts.Format.
func Search(w io.Writer, prompt, filter string, opts SearchOpts) error {
	idx, err := LoadIndex()
	if err != nil {
		return err
	}

	entries := idx.Entries
	if filter != "" {
		t, ok := FilterTypes[filter]
		if !ok {
			return fmt.Errorf("unknown filter type: %q", filter)
		}
		entries = search.FilterByType(entries, t)
	}

	tokens := search.Tokenize(prompt, search.DefaultStopWords())
	results := search.ScoreTop(entries, tokens, opts.Threshold, opts.Limit, idx.IDF, idx.AvgFieldLen)

	format := opts.Format
	if opts.JSON {
		format = "json"
	}
	switch format {
	case "json":
		return output.WriteJSON(w, results)
	default:
		output.WriteText(w, results)
		return nil
	}
}

// FilterTypes maps CLI filter names to index entry types.
var FilterTypes = map[string]index.EntryType{
	"kb":      index.TypeKB,
	"skill":   index.TypeSkill,
	"agent":   index.TypeAgent,
	"command": index.TypeCommand,
}

// LoadIndex discovers source dirs and loads (or rebuilds) the cached index.
func LoadIndex() (index.Index, error) {
	sourceDirs, err := index.SourceDirs()
	if err != nil {
		return index.Index{}, fmt.Errorf("discovering sources: %w", err)
	}
	return index.LoadOrRebuild(sourceDirs)
}

// RebuildIndex forces a full rescan and saves the index.
func RebuildIndex() ([]index.Entry, error) {
	sourceDirs, err := index.SourceDirs()
	if err != nil {
		return nil, fmt.Errorf("discovering sources: %w", err)
	}
	idx, err := index.ForceRebuild(sourceDirs)
	if err != nil {
		return nil, err
	}
	return idx.Entries, nil
}

// WriteScanSummary formats and writes the scan summary.
func WriteScanSummary(w io.Writer, entries []index.Entry) {
	counts := make(map[string]int)
	for _, e := range entries {
		counts[string(e.Type)]++
	}
	output.WriteScanSummary(w, counts, len(entries))
}

// FormatPrompt joins args into a single search prompt.
func FormatPrompt(args []string) string {
	return strings.Join(args, " ")
}

// ReadPromptFromReader reads the full contents of r, trims trailing whitespace,
// and returns the result. Returns an error if the result is empty.
func ReadPromptFromReader(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	prompt := strings.TrimRight(string(data), "\r\n\t ")
	if prompt == "" {
		return "", fmt.Errorf("stdin is empty: provide a non-empty prompt")
	}
	return prompt, nil
}
