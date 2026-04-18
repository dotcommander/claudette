package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendUsageLog_CreatesFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "usage.log")
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	ts := now.Unix()

	records := []UsageRecord{
		{Timestamp: now, Name: "entry-a", Score: 10},
		{Timestamp: now, Name: "entry-b", Score: 5},
	}
	if err := appendUsageLogWithPath(path, records); err != nil {
		t.Fatalf("append: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := fmt.Sprintf("%d\tentry-a\t10\n%d\tentry-b\t5\n", ts, ts)
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestAppendUsageLog_Appends(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "usage.log")
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

	r1 := []UsageRecord{{Timestamp: now, Name: "first", Score: 3}}
	r2 := []UsageRecord{{Timestamp: now, Name: "second", Score: 7}}

	if err := appendUsageLogWithPath(path, r1); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := appendUsageLogWithPath(path, r2); err != nil {
		t.Fatalf("second append: %v", err)
	}

	records, err := parseUsageLogWithPath(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].Name != "first" || records[1].Name != "second" {
		t.Errorf("got names %q, %q; want first, second", records[0].Name, records[1].Name)
	}
}

func TestAppendUsageLog_EmptySlice(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "usage.log")
	if err := appendUsageLogWithPath(path, nil); err != nil {
		t.Fatalf("append nil: %v", err)
	}
	// File should not be created for empty input.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected no file for empty records")
	}
}

func TestParseUsageLog_Missing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nonexistent.log")
	records, err := parseUsageLogWithPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if records != nil {
		t.Errorf("expected nil for missing file, got %v", records)
	}
}

func TestParseUsageLog_MalformedLines(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "usage.log")
	content := "1744286400\tgood\t5\nbadline\n\t\t\n1744286400\talso-good\t8\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	records, err := parseUsageLogWithPath(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2 (malformed skipped)", len(records))
	}
	if records[0].Name != "good" || records[1].Name != "also-good" {
		t.Errorf("unexpected names: %q, %q", records[0].Name, records[1].Name)
	}
}

func TestAggregateHitCounts(t *testing.T) {
	t.Parallel()
	now := time.Now()
	records := []UsageRecord{
		{Timestamp: now, Name: "a", Score: 10},
		{Timestamp: now, Name: "b", Score: 5},
		{Timestamp: now, Name: "a", Score: 8},
		{Timestamp: now, Name: "a", Score: 3},
		{Timestamp: now, Name: "b", Score: 2},
	}
	counts := AggregateHitCounts(records)
	if counts["a"] != 3 {
		t.Errorf("a: got %d, want 3", counts["a"])
	}
	if counts["b"] != 2 {
		t.Errorf("b: got %d, want 2", counts["b"])
	}
}

func TestTruncateUsageLog(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "usage.log")
	now := time.Now()

	records := []UsageRecord{{Timestamp: now, Name: "entry", Score: 5}}
	if err := appendUsageLogWithPath(path, records); err != nil {
		t.Fatal(err)
	}

	if err := truncateUsageLogWithPath(path); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	parsed, err := parseUsageLogWithPath(path)
	if err != nil {
		t.Fatalf("parse after truncate: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected 0 records after truncate, got %d", len(parsed))
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "usage.log")
	now := time.Date(2026, 4, 10, 15, 30, 0, 0, time.UTC)

	original := []UsageRecord{
		{Timestamp: now, Name: "hook-reload", Score: 12},
		{Timestamp: now, Name: "go-formatter", Score: 8},
	}
	if err := appendUsageLogWithPath(path, original); err != nil {
		t.Fatal(err)
	}

	parsed, err := parseUsageLogWithPath(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != len(original) {
		t.Fatalf("got %d records, want %d", len(parsed), len(original))
	}
	for i, r := range parsed {
		if r.Name != original[i].Name {
			t.Errorf("[%d] name: got %q, want %q", i, r.Name, original[i].Name)
		}
		if r.Score != original[i].Score {
			t.Errorf("[%d] score: got %d, want %d", i, r.Score, original[i].Score)
		}
		if r.Timestamp.Unix() != original[i].Timestamp.Unix() {
			t.Errorf("[%d] timestamp: got %d, want %d", i, r.Timestamp.Unix(), original[i].Timestamp.Unix())
		}
	}
}
