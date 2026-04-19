package index

import "strings"

// buildSuggestionSuppressSet builds a set of existing keywords per entry name
// so ComputeSuggestedAliases can skip tokens the entry already matches on.
// Keywords are lowercased to match the tokenizer's output.
// Frontmatter aliases already flow into Keywords via parse.go — single source of truth.
func buildSuggestionSuppressSet(entries []Entry) map[string]map[string]bool {
	set := make(map[string]map[string]bool, len(entries))
	for _, e := range entries {
		kw := make(map[string]bool, len(e.Keywords))
		for word := range e.Keywords {
			kw[strings.ToLower(word)] = true
		}
		set[e.Name] = kw
	}
	return set
}
