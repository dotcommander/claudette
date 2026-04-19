package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/dotcommander/claudette/internal/search"
)

// WouldSurfaceEntry is one entry that would be injected by the hook.
// Path and Title mirror the full-mode formatContext output; Matched holds the
// contributing tokens exactly as formatContext's [matched: ...] bracket.
type WouldSurfaceEntry struct {
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	Matched []string `json:"matched"`
}

// ExplainReport is the structured result of an explain run.
// RawTokens, KeptTokens, DroppedStops, AvgFieldLen, HasIDF, Diagnostics, and
// TotalScored are unexported from JSON via "-" tags; MarshalJSON flattens them
// into the documented {prompt, tokens, corpus, diagnostics, would_surface} wire shape.
type ExplainReport struct {
	Prompt            string                    `json:"prompt"`
	RawTokens         []string                  `json:"-"`
	KeptTokens        []string                  `json:"-"`
	DroppedStops      []string                  `json:"-"`
	AvgFieldLen       float64                   `json:"-"`
	HasIDF            bool                      `json:"-"`
	Diagnostics       []search.EntryDiagnostics `json:"-"`
	TotalScored       int                       `json:"-"`
	WouldSurface      []WouldSurfaceEntry       `json:"-"`
	SuppressionReason string                    `json:"-"`
}

// MarshalJSON flattens ExplainReport into the documented wire shape:
//
//	{
//	  "prompt":             "...",
//	  "tokens":             {"raw": [...], "kept": [...], "dropped_stops": [...]},
//	  "corpus":             {"entries_scored": N, "entries_shown": M, "idf_enabled": bool, "avg_field_len": F},
//	  "diagnostics":        [...],
//	  "would_surface":      [{path, title, matched}, ...],
//	  "suppression_reason": "..." (omitted when empty)
//	}
func (r ExplainReport) MarshalJSON() ([]byte, error) {
	type tokensBlock struct {
		Raw          []string `json:"raw"`
		Kept         []string `json:"kept"`
		DroppedStops []string `json:"dropped_stops"`
	}
	type corpusBlock struct {
		EntriesScored int     `json:"entries_scored"`
		EntriesShown  int     `json:"entries_shown"`
		IDFEnabled    bool    `json:"idf_enabled"`
		AvgFieldLen   float64 `json:"avg_field_len"`
	}
	type wire struct {
		Prompt            string                    `json:"prompt"`
		Tokens            tokensBlock               `json:"tokens"`
		Corpus            corpusBlock               `json:"corpus"`
		Diagnostics       []search.EntryDiagnostics `json:"diagnostics"`
		WouldSurface      []WouldSurfaceEntry       `json:"would_surface"`
		SuppressionReason string                    `json:"suppression_reason,omitempty"`
	}

	raw := r.RawTokens
	if raw == nil {
		raw = []string{}
	}
	kept := r.KeptTokens
	if kept == nil {
		kept = []string{}
	}
	dropped := r.DroppedStops
	if dropped == nil {
		dropped = []string{}
	}
	diags := r.Diagnostics
	if diags == nil {
		diags = []search.EntryDiagnostics{}
	}
	wouldSurface := r.WouldSurface
	if wouldSurface == nil {
		wouldSurface = []WouldSurfaceEntry{}
	}

	w := wire{
		Prompt: r.Prompt,
		Tokens: tokensBlock{
			Raw:          raw,
			Kept:         kept,
			DroppedStops: dropped,
		},
		Corpus: corpusBlock{
			EntriesScored: r.TotalScored,
			EntriesShown:  len(diags),
			IDFEnabled:    r.HasIDF,
			AvgFieldLen:   r.AvgFieldLen,
		},
		Diagnostics:       diags,
		WouldSurface:      wouldSurface,
		SuppressionReason: r.SuppressionReason,
	}
	return json.Marshal(w)
}

// WriteExplainJSON encodes the report as indented JSON.
func WriteExplainJSON(w io.Writer, r ExplainReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteExplainText writes the human-readable explain output.
// Layout: prompt echo → tokens block → corpus block → kept entries → suppressed entries.
func WriteExplainText(w io.Writer, r ExplainReport) {
	// Prompt echo (surfaces shell-quoting issues).
	fmt.Fprintf(w, "explain: %q\n\n", r.Prompt)

	// Tokens block.
	fmt.Fprintln(w, "tokens")
	tw := tabwriter.NewWriter(w, 2, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  raw:\t%v\n", formatTokenList(r.RawTokens))
	fmt.Fprintf(tw, "  kept:\t%v\n", formatTokenList(r.KeptTokens))
	dropped := "(none)"
	if len(r.DroppedStops) > 0 {
		dropped = formatTokenList(r.DroppedStops) + "  (stop words)"
	}
	fmt.Fprintf(tw, "  dropped:\t%s\n", dropped)
	tw.Flush()
	fmt.Fprintln(w)

	// Corpus block.
	keptCount := 0
	for _, d := range r.Diagnostics {
		if d.Suppressed == "" {
			keptCount++
		}
	}
	suppressedCount := len(r.Diagnostics) - keptCount
	fmt.Fprintln(w, "corpus")
	tw2 := tabwriter.NewWriter(w, 2, 0, 2, ' ', 0)
	fmt.Fprintf(tw2, "  entries scored:\t%d\n", r.TotalScored)
	fmt.Fprintf(tw2, "  entries shown:\t%d  (of %d above threshold, %d suppressed)\n",
		len(r.Diagnostics), keptCount, suppressedCount)
	fmt.Fprintf(tw2, "  idf enabled:\t%v\n", r.HasIDF)
	fmt.Fprintf(tw2, "  avg field length:\t%.1f\n", r.AvgFieldLen)
	tw2.Flush()
	fmt.Fprintln(w)

	if len(r.Diagnostics) == 0 {
		fmt.Fprintln(w, "(no matching entries)")
	} else {
		// Kept entries.
		keptIdx := 0
		for _, d := range r.Diagnostics {
			if d.Suppressed != "" {
				continue
			}
			keptIdx++
			writeEntryBlock(w, fmt.Sprintf("[%d]", keptIdx), d)
		}

		// Suppressed entries.
		suppIdx := 0
		for _, d := range r.Diagnostics {
			if d.Suppressed == "" {
				continue
			}
			if suppIdx == 0 {
				fmt.Fprintln(w, "--- suppressed (not in Score output) ---")
				fmt.Fprintln(w)
			}
			suppIdx++
			writeEntryBlock(w, fmt.Sprintf("[S%d]", suppIdx), d)
		}
	}

	// Hook simulation: what would actually be injected (always rendered).
	writeWouldSurface(w, r)
}

// writeWouldSurface appends the "would surface" block — a dry-run of the hook's
// gate logic showing exactly which files would be injected (and why not, if suppressed).
// Format mirrors formatContext full-mode so the output is directly comparable.
//
// When WouldSurface is non-empty, entries are always listed regardless of reason —
// the differential gate truncates to 1 but still surfaces it. Only when WouldSurface
// is empty is the "(nothing …)" line shown.
func writeWouldSurface(w io.Writer, r ExplainReport) {
	fmt.Fprintln(w, "would surface (full mode):")
	if len(r.WouldSurface) > 0 {
		for _, e := range r.WouldSurface {
			if len(e.Matched) > 0 {
				fmt.Fprintf(w, "  %s \u2014 %s [matched: %s]\n", e.Path, e.Title, strings.Join(e.Matched, ", "))
			} else {
				fmt.Fprintf(w, "  %s \u2014 %s\n", e.Path, e.Title)
			}
		}
		if r.SuppressionReason != "" {
			fmt.Fprintf(w, "  (truncated by gate: %s)\n", r.SuppressionReason)
		}
		return
	}
	if r.SuppressionReason != "" {
		fmt.Fprintf(w, "  (nothing would be injected: %s)\n", r.SuppressionReason)
	} else {
		fmt.Fprintln(w, "  (nothing above threshold)")
	}
}

// writeEntryBlock writes one entry's diagnostic block to w.
func writeEntryBlock(w io.Writer, label string, d search.EntryDiagnostics) {
	suppNote := ""
	if d.Suppressed != "" {
		suppNote = "  " + d.Suppressed
	}
	fmt.Fprintf(w, "%s score=%d  %s  (%s/%s)%s\n",
		label, d.FinalScore, d.Entry.Name, d.Entry.Type, d.Entry.Category, suppNote)
	fmt.Fprintf(w, "    final=%d  raw=%.2f  boost=%.2f×  preboost=%.2f\n",
		d.FinalScore, d.RawScore, d.UsageBoost, d.PreBoostSum)

	// Tokens subsection.
	fmt.Fprintln(w, "    tokens:")
	if len(d.TokenHits) == 0 {
		fmt.Fprintln(w, "      (no matches)")
	} else {
		tw := tabwriter.NewWriter(w, 6, 0, 2, ' ', 0)
		for _, h := range d.TokenHits {
			if h.Kind == "miss" {
				fmt.Fprintf(tw, "      %s\t%s\n", h.Token, h.Kind)
			} else {
				fmt.Fprintf(tw, "      %s\t%s\tweight=%d\tidf=%.2f\tΔ=%+.2f\n",
					h.Token, h.Kind, h.Weight, h.IDF, h.Delta)
			}
		}
		tw.Flush()
	}

	// Bigrams subsection.
	fmt.Fprintln(w, "    bigrams:")
	if len(d.BigramHits) == 0 {
		fmt.Fprintln(w, "      (no matches)")
	} else {
		for i, bg := range d.BigramHits {
			var delta float64
			if i < len(d.BigramDeltas) {
				delta = d.BigramDeltas[i]
			}
			fmt.Fprintf(w, "      %s  Δ=%+.2f\n", bg, delta)
		}
	}
	fmt.Fprintln(w)
}

// formatTokenList formats a token slice as [a b c] for display.
func formatTokenList(tokens []string) string {
	if len(tokens) == 0 {
		return "(none)"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, t := range tokens {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t)
	}
	b.WriteByte(']')
	return b.String()
}
