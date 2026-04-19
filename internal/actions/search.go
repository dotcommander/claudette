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
	Format      string
	JSON        bool // --json shorthand: when true, overrides Format to "json"
	Threshold   int
	Limit       int
	FilterTypes map[string]index.EntryType // CLI filter name -> entry type
}

// NewSearchOpts returns SearchOpts populated with package defaults.
// Callers (CLI flag binding, programmatic consumers) overlay values on top.
func NewSearchOpts() SearchOpts {
	return SearchOpts{
		Format:    "text",
		Threshold: search.DefaultThreshold,
		Limit:     search.DefaultLimit,
		FilterTypes: map[string]index.EntryType{
			"kb":      index.TypeKB,
			"skill":   index.TypeSkill,
			"agent":   index.TypeAgent,
			"command": index.TypeCommand,
		},
	}
}

// Search tokenizes the prompt, scores against the index, and writes results.
// When opts.JSON is true, output is always JSON regardless of opts.Format.
func Search(w io.Writer, prompt, filter string, opts SearchOpts) error {
	idx, err := LoadIndex()
	if err != nil {
		return err
	}

	var corpus search.Corpus = search.CorpusFromIndex(&idx)
	if filter != "" {
		t, ok := opts.FilterTypes[filter]
		if !ok {
			return fmt.Errorf("unknown filter type: %q", filter)
		}
		// Preserve parent-corpus IDF and AvgFieldLen: filtering entries must not
		// change term weights — scores must be comparable across filter runs.
		corpus = search.NewCorpus(search.FilterByType(idx.Entries, t), idx.IDF, idx.AvgFieldLen)
	}

	tokens := search.Tokenize(prompt, search.DefaultStopWords())
	pr := search.Run(search.PipelineInput{
		Tokens:     tokens,
		Corpus:     corpus,
		Threshold:  opts.Threshold,
		Limit:      opts.Limit,
		ApplyGates: false,
	})

	format := opts.Format
	if opts.JSON {
		format = "json"
	}
	switch format {
	case "json":
		return output.WriteJSON(w, pr.AboveThreshold)
	default:
		output.WriteText(w, pr.AboveThreshold)
		return nil
	}
}

// LoadIndex discovers source dirs and loads (or rebuilds) the cached index.
func LoadIndex() (index.Index, error) {
	sourceDirs, err := index.SourceDirs()
	if err != nil {
		return index.Index{}, fmt.Errorf("discovering sources: %w", err)
	}
	idx, _, err := index.LoadOrRebuild(sourceDirs)
	return idx, err
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

// FormatPrompt joins args into a single search prompt and rejects empty
// or whitespace-only input. Returns the trimmed prompt on success.
func FormatPrompt(args []string) (string, error) {
	return validatePrompt(strings.Join(args, " "), "prompt")
}

// ReadPromptFromReader reads the full contents of r and returns the trimmed
// result. Returns an error if the result is empty or whitespace-only.
func ReadPromptFromReader(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	return validatePrompt(string(data), "stdin")
}

// validatePrompt trims whitespace and rejects empty input. source is the
// label used in the error message ("prompt" for args, "stdin" for piped input)
// so callers see which input path failed without re-wrapping.
func validatePrompt(s, source string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", fmt.Errorf("%s is empty: provide a non-empty prompt", source)
	}
	return trimmed, nil
}
