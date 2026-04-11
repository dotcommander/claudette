package output

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/dotcommander/claudette/internal/search"
)

// WriteText formats scored entries as human-readable text.
func WriteText(w io.Writer, results []search.ScoredEntry) {
	if len(results) == 0 {
		fmt.Fprintln(w, "No matching entries found.")
		return
	}

	for _, r := range results {
		fmt.Fprintf(w, "  [%d] %s\n", r.Score, r.Entry.Title)
		fmt.Fprintf(w, "      %s  (%s/%s)\n", r.Entry.FilePath, r.Entry.Type, r.Entry.Category)
		if len(r.Matched) > 0 {
			fmt.Fprintf(w, "      matched: %s\n", strings.Join(r.Matched, ", "))
		}
		fmt.Fprintln(w)
	}
}

// WriteScanSummary formats scan results as a summary.
func WriteScanSummary(w io.Writer, counts map[string]int, total int) {
	fmt.Fprintf(w, "Indexed %d entries:\n", total)
	for _, label := range slices.Sorted(maps.Keys(counts)) {
		fmt.Fprintf(w, "  %-10s %d\n", label, counts[label])
	}
}
