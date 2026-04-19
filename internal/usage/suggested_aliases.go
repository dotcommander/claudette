package usage

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dotcommander/claudette/internal/fileutil"
	"gopkg.in/yaml.v3"
)

// suggestionsFile is the on-disk YAML structure for suggested-aliases.yaml.
type suggestionsFile struct {
	Entries []entrySuggestion `yaml:"entries"`
}

// entrySuggestion groups alias candidates for one entry.
type entrySuggestion struct {
	Entry      string           `yaml:"entry"`
	Candidates []candidateToken `yaml:"candidates"`
}

// candidateToken is a single alias candidate with its co-occurrence count.
type candidateToken struct {
	Token string `yaml:"token"`
	Count int    `yaml:"count"`
}

// ComputeSuggestedAliases builds the per-entry alias candidate list from
// co-occurrence records. For every (hitEntry, unmatchedToken) pair in each
// record, the count for that pair is incremented. Tokens that already appear
// in suppress[entry] (the entry's existing keywords, lowercased) are skipped.
// Candidates with count >= SuggestAliasThreshold are retained, sorted by count
// desc then token asc, and capped at TopNAliasCandidatesPerEntry per entry.
// Result is sorted by entry name ascending.
func ComputeSuggestedAliases(records []CoOccurrenceRecord, suppress map[string]map[string]bool) []entrySuggestion {
	// counts[entry][token] = number of records where the pair co-occurred.
	counts := make(map[string]map[string]int)

	for _, rec := range records {
		// Deduplicate pairs within one record so a token appearing twice in the
		// same record's unmatched list counts only once per entry.
		type pair struct{ entry, token string }
		seen := make(map[pair]bool)

		for _, entry := range rec.HitEntries {
			suppressed := suppress[entry]
			for _, tok := range rec.UnmatchedTokens {
				lower := strings.ToLower(tok)
				if suppressed[lower] {
					continue
				}
				p := pair{entry, lower}
				if seen[p] {
					continue
				}
				seen[p] = true
				if counts[entry] == nil {
					counts[entry] = make(map[string]int)
				}
				counts[entry][lower]++
			}
		}
	}

	var result []entrySuggestion
	for entry, tokenCounts := range counts {
		var candidates []candidateToken
		for tok, cnt := range tokenCounts {
			if cnt >= SuggestAliasThreshold {
				candidates = append(candidates, candidateToken{Token: tok, Count: cnt})
			}
		}
		if len(candidates) == 0 {
			continue
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Count != candidates[j].Count {
				return candidates[i].Count > candidates[j].Count
			}
			return candidates[i].Token < candidates[j].Token
		})
		if len(candidates) > TopNAliasCandidatesPerEntry {
			candidates = candidates[:TopNAliasCandidatesPerEntry]
		}
		result = append(result, entrySuggestion{Entry: entry, Candidates: candidates})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Entry < result[j].Entry
	})
	return result
}

// WriteSuggestedAliases marshals suggestions to YAML and atomically writes
// them to path via MkdirAll + AtomicWriteFile.
func WriteSuggestedAliases(path string, suggestions []entrySuggestion) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(suggestionsFile{Entries: suggestions})
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, data, 0o644)
}
