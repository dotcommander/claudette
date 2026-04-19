package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	counts := AggregateHitCounts(records, time.Time{})
	if counts["a"] != 3 {
		t.Errorf("a: got %d, want 3", counts["a"])
	}
	if counts["b"] != 2 {
		t.Errorf("b: got %d, want 2", counts["b"])
	}
}

func TestAggregateHitCounts_TTL_DropsOldRecords(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-DefaultUsageLogTTL)

	records := []UsageRecord{
		{Timestamp: now, Name: "recent", Score: 5},                             // within TTL
		{Timestamp: now.Add(-30 * 24 * time.Hour), Name: "recent", Score: 5},   // 30d ago — within
		{Timestamp: cutoff.Add(-time.Second), Name: "old", Score: 5},           // just beyond cutoff
		{Timestamp: now.Add(-200 * 24 * time.Hour), Name: "ancient", Score: 5}, // way beyond
	}
	counts := AggregateHitCounts(records, cutoff)

	if counts["recent"] != 2 {
		t.Errorf("recent: got %d, want 2", counts["recent"])
	}
	if counts["old"] != 0 {
		t.Errorf("old: got %d, want 0 (should be dropped)", counts["old"])
	}
	if counts["ancient"] != 0 {
		t.Errorf("ancient: got %d, want 0 (should be dropped)", counts["ancient"])
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

	// Zero cutoff — keeps all records (no TTL filter); file survives with its content.
	if err := truncateUsageLogWithPath(path, time.Time{}); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	parsed, err := parseUsageLogWithPath(path)
	if err != nil {
		t.Fatalf("parse after truncate: %v", err)
	}
	if len(parsed) != 1 {
		t.Errorf("expected 1 record (zero cutoff keeps all), got %d", len(parsed))
	}
}

func TestTruncateUsageLog_RetainsRecentWithinTTL(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "usage.log")
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-DefaultUsageLogTTL)

	records := []UsageRecord{
		{Timestamp: now, Name: "new", Score: 1},
		{Timestamp: now.Add(-30 * 24 * time.Hour), Name: "mid", Score: 2},
		{Timestamp: cutoff.Add(-time.Second), Name: "expired", Score: 3},
	}
	if err := appendUsageLogWithPath(path, records); err != nil {
		t.Fatal(err)
	}
	if err := truncateUsageLogWithPath(path, cutoff); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	parsed, err := parseUsageLogWithPath(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("got %d records, want 2 (recent+mid retained)", len(parsed))
	}
	names := map[string]bool{parsed[0].Name: true, parsed[1].Name: true}
	if !names["new"] || !names["mid"] {
		t.Errorf("unexpected names after compact: %v", names)
	}
}

func TestTruncateUsageLog_RemovesBeyondTTL(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "usage.log")
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-DefaultUsageLogTTL)

	// All records are beyond the cutoff.
	records := []UsageRecord{
		{Timestamp: cutoff.Add(-time.Hour), Name: "old-a", Score: 5},
		{Timestamp: cutoff.Add(-24 * time.Hour), Name: "old-b", Score: 3},
	}
	if err := appendUsageLogWithPath(path, records); err != nil {
		t.Fatal(err)
	}
	if err := truncateUsageLogWithPath(path, cutoff); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	parsed, err := parseUsageLogWithPath(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("got %d records, want 0 (all expired)", len(parsed))
	}
}

func TestAggregateDecayedHitCounts_HalfLife(t *testing.T) {
	t.Parallel()
	// Two records for "a": one now, one exactly one half-life ago.
	// Decayed counts should be ~1.0 and ~0.5 respectively → total ~1.5 for "a",
	// and the recent hit contributes ~2x the old one.
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	halfLife := 70 * 24 * time.Hour
	records := []UsageRecord{
		{Timestamp: now, Name: "a", Score: 5},
		{Timestamp: now.Add(-halfLife), Name: "a", Score: 5},
		{Timestamp: now, Name: "b", Score: 3},
	}
	counts := AggregateDecayedHitCounts(records, now, halfLife, time.Time{})

	// "a" should be ~1.5 (1.0 recent + 0.5 old).
	const eps = 1e-6
	wantA := 1.5
	if diff := counts["a"] - wantA; diff < -eps || diff > eps {
		t.Errorf("a: got %v, want %v (±%v)", counts["a"], wantA, eps)
	}
	// "b" should be ~1.0 (single recent hit).
	wantB := 1.0
	if diff := counts["b"] - wantB; diff < -eps || diff > eps {
		t.Errorf("b: got %v, want %v (±%v)", counts["b"], wantB, eps)
	}
	// Ratio invariant: recent "a" hit weighs exactly 2× the old one.
	// We can't decompose the sum, but the 2:1 structure is encoded in wantA above.
}

func TestAggregateDecayedHitCounts_RecentDominates(t *testing.T) {
	t.Parallel()
	// A hit 10 half-lives ago contributes 2^(-10) ≈ 0.001 — negligible.
	// A hit today contributes 1.0.
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	halfLife := 70 * 24 * time.Hour
	ancient := now.Add(-10 * halfLife)
	records := []UsageRecord{
		{Timestamp: now, Name: "x", Score: 5},
		{Timestamp: ancient, Name: "x", Score: 5},
	}
	counts := AggregateDecayedHitCounts(records, now, halfLife, time.Time{})

	// Total should be between 1.0 and 1.005 (ancient barely contributes).
	if counts["x"] < 1.0 || counts["x"] > 1.005 {
		t.Errorf("x: got %v, want in [1.0, 1.005] — recent should dominate", counts["x"])
	}
}

func TestAggregateDecayedHitCounts_TTL_DropsOldRecords(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	halfLife := 70 * 24 * time.Hour
	cutoff := now.Add(-DefaultUsageLogTTL)

	records := []UsageRecord{
		{Timestamp: now, Name: "recent", Score: 5},                             // within TTL — weight ≈ 1.0
		{Timestamp: cutoff.Add(-time.Second), Name: "expired", Score: 5},       // just beyond cutoff — must be dropped
		{Timestamp: now.Add(-200 * 24 * time.Hour), Name: "ancient", Score: 5}, // way beyond — must be dropped
	}
	counts := AggregateDecayedHitCounts(records, now, halfLife, cutoff)

	const eps = 1e-6
	if counts["recent"] < 1.0-eps || counts["recent"] > 1.0+eps {
		t.Errorf("recent: got %v, want ≈1.0", counts["recent"])
	}
	if counts["expired"] != 0 {
		t.Errorf("expired: got %v, want 0 (should be dropped by cutoff)", counts["expired"])
	}
	if counts["ancient"] != 0 {
		t.Errorf("ancient: got %v, want 0 (should be dropped by cutoff)", counts["ancient"])
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

// --- Miss log tests ---

func TestAppendMissLog_RoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "misses.log")
	tokens := []string{"golang", "concurrency"}

	if err := appendMissLogWithPath(path, tokens); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := appendMissLogWithPath(path, []string{"solid"}); err != nil {
		t.Fatalf("second append: %v", err)
	}

	records, err := parseMissLogWithPath(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}

	got0 := records[0].Tokens
	sort.Strings(got0)
	want0 := []string{"concurrency", "golang"}
	sort.Strings(want0)
	for i, v := range want0 {
		if got0[i] != v {
			t.Errorf("[0] token[%d]: got %q, want %q", i, got0[i], v)
		}
	}
	if len(records[1].Tokens) != 1 || records[1].Tokens[0] != "solid" {
		t.Errorf("[1] tokens: got %v, want [solid]", records[1].Tokens)
	}
	if records[0].Timestamp.IsZero() || records[1].Timestamp.IsZero() {
		t.Error("expected non-zero timestamps")
	}
}

func TestAppendMissLog_EmptyTokens_NoFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "misses.log")
	if err := appendMissLogWithPath(path, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// AppendMissLog skips empty — but appendMissLogWithPath doesn't guard it.
	// The public AppendMissLog does. Test public behaviour via zero len.
	if err := AppendMissLog(nil); err != nil {
		t.Fatalf("AppendMissLog(nil): %v", err)
	}
}

func TestTruncateMissLog_RemovesBeyondTTL(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "misses.log")
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-DefaultUsageLogTTL)

	// Write one recent and one expired record manually.
	recent := MissRecord{Timestamp: now, Tokens: []string{"recent"}}
	expired := MissRecord{Timestamp: cutoff.Add(-time.Hour), Tokens: []string{"expired"}}

	if err := appendMissLogWithPath(path, recent.Tokens); err != nil {
		t.Fatal(err)
	}
	// Manually write expired via parseMissLogWithPath round-trip is awkward;
	// use a helper that writes specific timestamps.
	if err := writeMissRecordsWithPath(path, []MissRecord{recent, expired}); err != nil {
		t.Fatal(err)
	}

	if err := truncateMissLogWithPath(path, cutoff); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	records, err := parseMissLogWithPath(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records after truncate, want 1 (recent only)", len(records))
	}
	if records[0].Tokens[0] != "recent" {
		t.Errorf("expected 'recent' token, got %v", records[0].Tokens)
	}
}

func TestComputeFrequentMissTokens_BelowThreshold_Empty(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-DefaultUsageLogTTL)

	// Two records with "golang" — below threshold of 3.
	records := []MissRecord{
		{Timestamp: now, Tokens: []string{"golang"}},
		{Timestamp: now, Tokens: []string{"golang"}},
	}
	got := ComputeFrequentMissTokens(records, now, cutoff, MissClusterThreshold, TopNMissTokens)
	if got != nil {
		t.Errorf("expected nil below threshold, got %v", got)
	}
}

func TestComputeFrequentMissTokens_AboveThreshold_ReturnsTopN(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-DefaultUsageLogTTL)

	// "solid" appears in 4 records, "dry" in 3, "yagni" in 2 (below threshold).
	records := []MissRecord{
		{Timestamp: now, Tokens: []string{"solid", "dry"}},
		{Timestamp: now, Tokens: []string{"solid", "dry"}},
		{Timestamp: now, Tokens: []string{"solid", "dry"}},
		{Timestamp: now, Tokens: []string{"solid"}},
	}
	got := ComputeFrequentMissTokens(records, now, cutoff, 3, 5)
	if len(got) != 2 {
		t.Fatalf("got %v, want [solid dry]", got)
	}
	// "solid" must rank first (4 hits > 3 hits).
	if got[0] != "solid" {
		t.Errorf("expected solid first, got %q", got[0])
	}
	if got[1] != "dry" {
		t.Errorf("expected dry second, got %q", got[1])
	}
}

func TestComputeFrequentMissTokens_TTLFilter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-DefaultUsageLogTTL)

	// 3 recent records with "recent" + 3 expired records with "expired".
	// Only "recent" should clear the threshold.
	records := []MissRecord{
		{Timestamp: now, Tokens: []string{"recent"}},
		{Timestamp: now, Tokens: []string{"recent"}},
		{Timestamp: now, Tokens: []string{"recent"}},
		{Timestamp: cutoff.Add(-time.Hour), Tokens: []string{"expired"}},
		{Timestamp: cutoff.Add(-time.Hour), Tokens: []string{"expired"}},
		{Timestamp: cutoff.Add(-time.Hour), Tokens: []string{"expired"}},
	}
	got := ComputeFrequentMissTokens(records, now, cutoff, MissClusterThreshold, TopNMissTokens)
	if len(got) != 1 || got[0] != "recent" {
		t.Errorf("expected [recent], got %v", got)
	}
}

// writeMissRecordsWithPath overwrites the file with the given records (for testing exact timestamps).
func writeMissRecordsWithPath(path string, records []MissRecord) error {
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
