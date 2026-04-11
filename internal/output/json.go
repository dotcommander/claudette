package output

import (
	"encoding/json"
	"io"

	"github.com/dotcommander/claudette/internal/search"
)

// SearchResult is the JSON output shape.
type SearchResult struct {
	Matches []MatchResult `json:"matches"`
	Total   int           `json:"total"`
}

// MatchResult is a single search result.
type MatchResult struct {
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Path     string   `json:"path"`
	Score    int      `json:"score"`
	Matched  []string `json:"matched"`
}

// WriteJSON writes scored entries as structured JSON.
func WriteJSON(w io.Writer, results []search.ScoredEntry) error {
	matches := make([]MatchResult, len(results))
	for i, r := range results {
		matches[i] = MatchResult{
			Type:     string(r.Entry.Type),
			Name:     r.Entry.Name,
			Title:    r.Entry.Title,
			Category: r.Entry.Category,
			Path:     r.Entry.FilePath,
			Score:    r.Score,
			Matched:  r.Matched,
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(SearchResult{
		Matches: matches,
		Total:   len(matches),
	})
}
