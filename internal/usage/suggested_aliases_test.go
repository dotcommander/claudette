package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// makeCoRecords builds a slice of CoOccurrenceRecords for testing.
func makeCoRecords(n int, unmatched []string, entries []string) []CoOccurrenceRecord {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	records := make([]CoOccurrenceRecord, n)
	for i := range records {
		records[i] = CoOccurrenceRecord{
			Timestamp:       now,
			UnmatchedTokens: unmatched,
			HitEntries:      entries,
		}
	}
	return records
}

func TestComputeSuggestedAliases_BelowThreshold_Empty(t *testing.T) {
	t.Parallel()
	// Token "golang" appears in only SuggestAliasThreshold-1 records — should not surface.
	records := makeCoRecords(SuggestAliasThreshold-1, []string{"golang"}, []string{"entry-a"})
	got := ComputeSuggestedAliases(records, nil)
	if len(got) != 0 {
		t.Errorf("expected empty result below threshold, got %v", got)
	}
}

func TestComputeSuggestedAliases_AtThreshold_Surfaced(t *testing.T) {
	t.Parallel()
	records := makeCoRecords(SuggestAliasThreshold, []string{"golang"}, []string{"entry-a"})
	got := ComputeSuggestedAliases(records, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	if got[0].Entry != "entry-a" {
		t.Errorf("entry: got %q, want entry-a", got[0].Entry)
	}
	if len(got[0].Candidates) != 1 || got[0].Candidates[0].Token != "golang" {
		t.Errorf("candidates: got %v", got[0].Candidates)
	}
	if got[0].Candidates[0].Count != SuggestAliasThreshold {
		t.Errorf("count: got %d, want %d", got[0].Candidates[0].Count, SuggestAliasThreshold)
	}
}

func TestComputeSuggestedAliases_TopNCap(t *testing.T) {
	t.Parallel()
	// More than TopNAliasCandidatesPerEntry distinct tokens all above threshold.
	n := SuggestAliasThreshold
	entries := []string{"entry-x"}
	// Each token appears exactly n times (at threshold).
	var records []CoOccurrenceRecord
	tokens := []string{"alpha", "beta", "gamma", "delta", "epsilon"} // 5 > TopN=3
	for _, tok := range tokens {
		records = append(records, makeCoRecords(n, []string{tok}, entries)...)
	}
	got := ComputeSuggestedAliases(records, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if len(got[0].Candidates) > TopNAliasCandidatesPerEntry {
		t.Errorf("candidates capped at %d, got %d", TopNAliasCandidatesPerEntry, len(got[0].Candidates))
	}
}

func TestComputeSuggestedAliases_SuppressSet_FiltersExistingKeywords(t *testing.T) {
	t.Parallel()
	// "golang" is already a keyword for entry-a — must be suppressed.
	suppress := map[string]map[string]bool{
		"entry-a": {"golang": true},
	}
	records := makeCoRecords(SuggestAliasThreshold, []string{"golang"}, []string{"entry-a"})
	got := ComputeSuggestedAliases(records, suppress)
	if len(got) != 0 {
		t.Errorf("expected empty result when token suppressed, got %v", got)
	}
}

func TestComputeSuggestedAliases_SuppressSet_CaseInsensitive(t *testing.T) {
	t.Parallel()
	// Suppress set is lowercase "golang"; token in record is "Golang" (mixed case).
	suppress := map[string]map[string]bool{
		"entry-a": {"golang": true},
	}
	records := makeCoRecords(SuggestAliasThreshold, []string{"Golang"}, []string{"entry-a"})
	got := ComputeSuggestedAliases(records, suppress)
	if len(got) != 0 {
		t.Errorf("expected suppression to be case-insensitive, got %v", got)
	}
}

func TestComputeSuggestedAliases_DeterministicSort(t *testing.T) {
	t.Parallel()
	// Two entries, both with candidates. Result must be sorted by entry name ascending.
	n := SuggestAliasThreshold
	var records []CoOccurrenceRecord
	records = append(records, makeCoRecords(n, []string{"tok"}, []string{"zebra"})...)
	records = append(records, makeCoRecords(n, []string{"tok"}, []string{"apple"})...)

	got := ComputeSuggestedAliases(records, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Entry != "apple" || got[1].Entry != "zebra" {
		t.Errorf("expected [apple, zebra], got [%s, %s]", got[0].Entry, got[1].Entry)
	}
}

func TestComputeSuggestedAliases_CandidateSortByCountDescThenTokenAsc(t *testing.T) {
	t.Parallel()
	// "beta" appears SuggestAliasThreshold+2 times; "alpha" appears SuggestAliasThreshold times.
	// "beta" must rank first (higher count). Both above threshold.
	entries := []string{"entry-a"}
	var records []CoOccurrenceRecord
	records = append(records, makeCoRecords(SuggestAliasThreshold+2, []string{"beta"}, entries)...)
	records = append(records, makeCoRecords(SuggestAliasThreshold, []string{"alpha"}, entries)...)

	got := ComputeSuggestedAliases(records, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	cands := got[0].Candidates
	if len(cands) < 2 {
		t.Fatalf("expected 2 candidates, got %d", len(cands))
	}
	if cands[0].Token != "beta" {
		t.Errorf("expected beta first (higher count), got %q", cands[0].Token)
	}
	if cands[1].Token != "alpha" {
		t.Errorf("expected alpha second, got %q", cands[1].Token)
	}
}

func TestWriteSuggestedAliases_YAMLRoundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "suggested-aliases.yaml")

	suggestions := []entrySuggestion{
		{
			Entry: "code-clean-code",
			Candidates: []candidateToken{
				{Token: "solid", Count: 7},
				{Token: "dry", Count: 5},
			},
		},
	}
	if err := WriteSuggestedAliases(path, suggestions); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var got suggestionsFile
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(got.Entries))
	}
	if got.Entries[0].Entry != "code-clean-code" {
		t.Errorf("entry name: got %q, want code-clean-code", got.Entries[0].Entry)
	}
	if len(got.Entries[0].Candidates) != 2 {
		t.Fatalf("candidates: got %d, want 2", len(got.Entries[0].Candidates))
	}
	if got.Entries[0].Candidates[0].Token != "solid" || got.Entries[0].Candidates[0].Count != 7 {
		t.Errorf("candidate[0]: got %+v", got.Entries[0].Candidates[0])
	}
}

func TestWriteSuggestedAliases_CreatesParentDir(t *testing.T) {
	t.Parallel()
	// Path with a non-existent parent — MkdirAll must create it.
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	path := filepath.Join(dir, "suggested-aliases.yaml")

	if err := WriteSuggestedAliases(path, []entrySuggestion{}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist, got %v", err)
	}
}
