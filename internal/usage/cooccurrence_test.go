package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendCoOccurrenceLog_RoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cooccurrence.log")
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	rec := CoOccurrenceRecord{
		Timestamp:       now,
		UnmatchedTokens: []string{"solid", "dry"},
		HitEntries:      []string{"code-clean-code"},
	}
	if err := AppendCoOccurrenceLog(path, rec); err != nil {
		t.Fatalf("append: %v", err)
	}

	records, err := ParseCoOccurrenceLog(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	got := records[0]
	if got.Timestamp.Unix() != now.Unix() {
		t.Errorf("timestamp: got %d, want %d", got.Timestamp.Unix(), now.Unix())
	}
	if len(got.UnmatchedTokens) != 2 || got.UnmatchedTokens[0] != "solid" {
		t.Errorf("unmatched tokens: got %v", got.UnmatchedTokens)
	}
	if len(got.HitEntries) != 1 || got.HitEntries[0] != "code-clean-code" {
		t.Errorf("hit entries: got %v", got.HitEntries)
	}
}

func TestAppendCoOccurrenceLog_Appends(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cooccurrence.log")
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	rec1 := CoOccurrenceRecord{Timestamp: now, UnmatchedTokens: []string{"first"}, HitEntries: []string{"entry-a"}}
	rec2 := CoOccurrenceRecord{Timestamp: now, UnmatchedTokens: []string{"second"}, HitEntries: []string{"entry-b"}}

	if err := AppendCoOccurrenceLog(path, rec1); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := AppendCoOccurrenceLog(path, rec2); err != nil {
		t.Fatalf("second append: %v", err)
	}

	records, err := ParseCoOccurrenceLog(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].UnmatchedTokens[0] != "first" {
		t.Errorf("record[0]: got %v, want [first]", records[0].UnmatchedTokens)
	}
	if records[1].UnmatchedTokens[0] != "second" {
		t.Errorf("record[1]: got %v, want [second]", records[1].UnmatchedTokens)
	}
}

func TestAppendCoOccurrenceLog_CapsAtMaxUnmatched(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cooccurrence.log")
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	// Build a token slice larger than MaxUnmatchedPerRecord.
	tokens := make([]string, MaxUnmatchedPerRecord+5)
	for i := range tokens {
		tokens[i] = "tok"
	}
	rec := CoOccurrenceRecord{
		Timestamp:       now,
		UnmatchedTokens: tokens,
		HitEntries:      []string{"entry-x"},
	}
	if err := AppendCoOccurrenceLog(path, rec); err != nil {
		t.Fatalf("append: %v", err)
	}

	records, err := ParseCoOccurrenceLog(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if len(records[0].UnmatchedTokens) != MaxUnmatchedPerRecord {
		t.Errorf("unmatched tokens: got %d, want %d (capped)", len(records[0].UnmatchedTokens), MaxUnmatchedPerRecord)
	}
}

func TestAppendCoOccurrenceLog_EmptyUnmatched_NoFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cooccurrence.log")
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	rec := CoOccurrenceRecord{
		Timestamp:       now,
		UnmatchedTokens: []string{}, // empty — must be skipped
		HitEntries:      []string{"entry-x"},
	}
	if err := AppendCoOccurrenceLog(path, rec); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected no file created for empty unmatched tokens")
	}
}

func TestParseCoOccurrenceLog_Missing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nonexistent.log")
	records, err := ParseCoOccurrenceLog(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if records != nil {
		t.Errorf("expected nil for missing file, got %v", records)
	}
}

func TestTruncateCoOccurrenceLog_TTLFilter(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cooccurrence.log")
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-DefaultUsageLogTTL)

	recent := CoOccurrenceRecord{
		Timestamp:       now,
		UnmatchedTokens: []string{"recent"},
		HitEntries:      []string{"entry-a"},
	}
	expired := CoOccurrenceRecord{
		Timestamp:       cutoff.Add(-time.Hour),
		UnmatchedTokens: []string{"expired"},
		HitEntries:      []string{"entry-b"},
	}
	if err := writeCoOccurrenceRecordsWithPath(path, []CoOccurrenceRecord{recent, expired}); err != nil {
		t.Fatal(err)
	}

	if err := TruncateCoOccurrenceLog(path, cutoff); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	records, err := ParseCoOccurrenceLog(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records after truncate, want 1 (recent only)", len(records))
	}
	if records[0].UnmatchedTokens[0] != "recent" {
		t.Errorf("expected 'recent' token, got %v", records[0].UnmatchedTokens)
	}
}

func TestTruncateCoOccurrenceLog_MissingFile_NoOp(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nonexistent.log")
	if err := TruncateCoOccurrenceLog(path, time.Now()); err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
}

// writeCoOccurrenceRecordsWithPath is a test helper that writes records with
// exact timestamps, bypassing AppendCoOccurrenceLog's empty-guard and cap.
// Mirrors writeMissRecordsWithPath in usage_test.go.
func writeCoOccurrenceRecordsWithPath(path string, records []CoOccurrenceRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}
