package hook

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/claudette/internal/index"
)

// --- advisoryStatusSegment tests ---

func TestAdvisoryStatusSegment_AllZero_EmptyString(t *testing.T) {
	t.Parallel()
	// Critical invariant: zero HookStatus produces "" so stderr is byte-identical.
	got := advisoryStatusSegment(HookStatus{})
	if got != "" {
		t.Errorf("expected empty string for zero HookStatus, got %q", got)
	}
}

func TestAdvisoryStatusSegment_PartialFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status HookStatus
		checks []string // substrings that must appear
		absent []string // substrings that must NOT appear
	}{
		{
			name:   "RebuildMs only",
			status: HookStatus{RebuildMs: 45},
			checks: []string{" | rebuild_ms=45"},
			absent: []string{"miss_count", "zero_kw_count", "cold_count"},
		},
		{
			name:   "MissCount only",
			status: HookStatus{MissCount: 3},
			checks: []string{" | miss_count=3"},
			absent: []string{"rebuild_ms", "zero_kw_count", "cold_count"},
		},
		{
			name:   "ZeroKwCount only",
			status: HookStatus{ZeroKwCount: 7},
			checks: []string{" | zero_kw_count=7"},
			absent: []string{"rebuild_ms", "miss_count", "cold_count"},
		},
		{
			name:   "ColdCount only",
			status: HookStatus{ColdCount: 2},
			checks: []string{" | cold_count=2"},
			absent: []string{"rebuild_ms", "miss_count", "zero_kw_count"},
		},
		{
			name:   "AllFields",
			status: HookStatus{RebuildMs: 45, MissCount: 0, ZeroKwCount: 2, ColdCount: 0},
			checks: []string{" | rebuild_ms=45", "zero_kw_count=2"},
			absent: []string{"miss_count", "cold_count"},
		},
		{
			name:   "FixedOrder_RebuildBeforeMiss",
			status: HookStatus{RebuildMs: 10, MissCount: 5},
			checks: []string{" | rebuild_ms=10 miss_count=5"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := advisoryStatusSegment(tc.status)
			for _, sub := range tc.checks {
				if !strings.Contains(got, sub) {
					t.Errorf("expected %q in segment, got %q", sub, got)
				}
			}
			for _, sub := range tc.absent {
				if strings.Contains(got, sub) {
					t.Errorf("expected %q absent from segment, got %q", sub, got)
				}
			}
		})
	}
}

// --- Checker stub purity tests ---

// TestCheckerPurity_NoIO proves that all four production stub checkers return
// inactive with an empty message for a zero HookStatus.
// Tests the canonical stubs directly — not the package-level registry, which
// other parallel tests may temporarily replace.
func TestCheckerPurity_NoIO(t *testing.T) {
	t.Parallel()
	zero := HookStatus{}
	stubs := []AdvisoryChecker{
		{Name: "StaleBuild", Check: checkStaleBuild},
		{Name: "ZeroKeyword", Check: checkZeroKeyword},
		{Name: "ColdEntries", Check: checkColdEntries},
		{Name: "MissCluster", Check: checkMissCluster},
		{Name: "SuggestedAliases", Check: checkSuggestedAliases},
	}
	for _, c := range stubs {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			active, msg := c.Check(zero)
			if active {
				t.Errorf("checker %q must be inactive for zero HookStatus, got active=true msg=%q", c.Name, msg)
			}
			if msg != "" {
				t.Errorf("checker %q must return empty message when inactive, got %q", c.Name, msg)
			}
		})
	}
}

// --- Byte-identical stderr invariant ---

// TestLogStatus_ByteIdenticalWhenAllZero verifies that when all HookStatus
// advisory fields are zero, the stderr line is byte-identical to the
// pre-advisory format: "<prefix>: <status> (NNms)\n".
func TestLogStatus_ByteIdenticalWhenAllZero(t *testing.T) {
	t.Parallel()

	prefix := "claudette"
	status := "hit: 3 results"
	hs := HookStatus{} // all zero

	// Capture by redirecting os.Stderr via a pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	// Fix the duration so we can predict the output exactly.
	fakeStart := time.Now().Add(-12 * time.Millisecond)
	fn := logStatus(prefix, &status, &hs, fakeStart)
	fn()

	os.Stderr = origStderr
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()

	// Extract the actual milliseconds from the captured output, then rebuild
	// the expected string with that same value — avoids flakiness from timing.
	var ms int
	if _, scanErr := fmt.Sscanf(got, "claudette: hit: 3 results (%dms)\n", &ms); scanErr != nil {
		t.Fatalf("stderr line does not match pre-advisory format: %q (parse error: %v)", got, scanErr)
	}

	want := fmt.Sprintf("claudette: hit: 3 results (%dms)\n", ms)
	if got != want {
		t.Errorf("byte-identical invariant failed:\n  got:  %q\n  want: %q", got, want)
	}

	// Confirm no advisory segment appears.
	if strings.Contains(got, " |") {
		t.Errorf("advisory segment must not appear when all HookStatus fields are zero, got %q", got)
	}
}

// TestLogStatus_WithRebuildMs_EmitsSegment verifies that a non-zero RebuildMs
// produces a stderr line containing "rebuild_ms=N" in the advisory segment.
func TestLogStatus_WithRebuildMs_EmitsSegment(t *testing.T) {
	t.Parallel()

	prefix := "claudette"
	status := "hit: 2 results"
	hs := HookStatus{RebuildMs: 45}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	fakeStart := time.Now().Add(-50 * time.Millisecond)
	fn := logStatus(prefix, &status, &hs, fakeStart)
	fn()

	os.Stderr = origStderr
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "rebuild_ms=45") {
		t.Errorf("expected rebuild_ms=45 in stderr line, got %q", got)
	}
	if !strings.Contains(got, " |") {
		t.Errorf("expected advisory segment separator ' |' in stderr line, got %q", got)
	}
}

// --- checkStaleBuild tests ---

func TestCheckStaleBuild_Activates_Above30Days(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ageDays int
		wantMsg string
	}{
		{ageDays: 31, wantMsg: "index is 31 days old"},
		{ageDays: 40, wantMsg: "index is 40 days old"},
		{ageDays: 365, wantMsg: "index is 365 days old"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("age_%d", tc.ageDays), func(t *testing.T) {
			t.Parallel()
			active, msg := checkStaleBuild(HookStatus{IndexAgeDays: tc.ageDays})
			if !active {
				t.Errorf("expected active=true for IndexAgeDays=%d", tc.ageDays)
			}
			if !strings.Contains(msg, tc.wantMsg) {
				t.Errorf("expected message to contain %q, got %q", tc.wantMsg, msg)
			}
			if !strings.Contains(msg, "claudette update") {
				t.Errorf("expected message to mention 'claudette update', got %q", msg)
			}
		})
	}
}

func TestCheckStaleBuild_Silent_AtOrBelow30Days(t *testing.T) {
	t.Parallel()
	tests := []int{0, 1, 15, 29, 30}
	for _, ageDays := range tests {
		ageDays := ageDays
		t.Run(fmt.Sprintf("age_%d", ageDays), func(t *testing.T) {
			t.Parallel()
			active, msg := checkStaleBuild(HookStatus{IndexAgeDays: ageDays})
			if active {
				t.Errorf("expected active=false for IndexAgeDays=%d, got active=true msg=%q", ageDays, msg)
			}
			if msg != "" {
				t.Errorf("expected empty message for IndexAgeDays=%d, got %q", ageDays, msg)
			}
		})
	}
}

// TestPopulateAdvisoryFields_IndexAge_Computed verifies that IndexAgeDays is
// derived from idx.BuildTime relative to the provided now time.
func TestPopulateAdvisoryFields_IndexAge_Computed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ageDays  int
		wantDays int
	}{
		{name: "fresh", ageDays: 0, wantDays: 0},
		{name: "30days", ageDays: 30, wantDays: 30},
		{name: "40days", ageDays: 40, wantDays: 40},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
			idx := index.Index{BuildTime: now.AddDate(0, 0, -tc.ageDays)}
			var hs HookStatus
			populateAdvisoryFields(&hs, &idx, now)
			if hs.IndexAgeDays != tc.wantDays {
				t.Errorf("IndexAgeDays: got %d, want %d", hs.IndexAgeDays, tc.wantDays)
			}
		})
	}
}

// --- checkZeroKeyword tests ---

func TestCheckZeroKeyword_Activates_WhenCountPositive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		count   int
		wantSub string
	}{
		{count: 1, wantSub: "1 entries have zero keywords"},
		{count: 5, wantSub: "5 entries have zero keywords"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("count_%d", tc.count), func(t *testing.T) {
			t.Parallel()
			active, msg := checkZeroKeyword(HookStatus{ZeroKwCount: tc.count})
			if !active {
				t.Errorf("expected active=true for ZeroKwCount=%d", tc.count)
			}
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("expected message to contain %q, got %q", tc.wantSub, msg)
			}
			if !strings.Contains(msg, "frontmatter") {
				t.Errorf("expected message to mention 'frontmatter', got %q", msg)
			}
		})
	}
}

func TestCheckZeroKeyword_Silent_WhenZero(t *testing.T) {
	t.Parallel()
	active, msg := checkZeroKeyword(HookStatus{ZeroKwCount: 0})
	if active {
		t.Errorf("expected active=false for ZeroKwCount=0, got active=true msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("expected empty message for ZeroKwCount=0, got %q", msg)
	}
}

// TestPopulateAdvisoryFields_ZeroKwCount_FromIndex verifies that ZeroKwCount
// is read from idx.ZeroKeywordCount (precomputed at build time, not re-scanned).
func TestPopulateAdvisoryFields_ZeroKwCount_FromIndex(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	idx := index.Index{
		BuildTime:        now,
		ZeroKeywordCount: 3,
	}
	var hs HookStatus
	populateAdvisoryFields(&hs, &idx, now)
	if hs.ZeroKwCount != 3 {
		t.Errorf("ZeroKwCount: got %d, want 3", hs.ZeroKwCount)
	}
}

// --- checkColdEntries tests ---

func TestCheckColdEntries_Activates_WhenCountPositive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		count   int
		wantSub string
	}{
		{count: 1, wantSub: "1 entries have zero hits"},
		{count: 7, wantSub: "7 entries have zero hits"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("count_%d", tc.count), func(t *testing.T) {
			t.Parallel()
			active, msg := checkColdEntries(HookStatus{ColdCount: tc.count})
			if !active {
				t.Errorf("expected active=true for ColdCount=%d", tc.count)
			}
			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("expected message to contain %q, got %q", tc.wantSub, msg)
			}
			if !strings.Contains(msg, "90 days") {
				t.Errorf("expected message to mention '90 days', got %q", msg)
			}
		})
	}
}

func TestCheckColdEntries_Silent_WhenZero(t *testing.T) {
	t.Parallel()
	active, msg := checkColdEntries(HookStatus{ColdCount: 0})
	if active {
		t.Errorf("expected active=false for ColdCount=0, got active=true msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("expected empty message for ColdCount=0, got %q", msg)
	}
}

// TestPopulateAdvisoryFields_ColdCount_FromIndex verifies that ColdCount is
// read from idx.ColdEntryCount (precomputed at build time, not re-scanned).
func TestPopulateAdvisoryFields_ColdCount_FromIndex(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	idx := index.Index{
		BuildTime:      now,
		ColdEntryCount: 4,
	}
	var hs HookStatus
	populateAdvisoryFields(&hs, &idx, now)
	if hs.ColdCount != 4 {
		t.Errorf("ColdCount: got %d, want 4", hs.ColdCount)
	}
}

// --- checkMissCluster tests ---

func TestCheckMissCluster_Activates_WhenTokensPresent(t *testing.T) {
	t.Parallel()
	tokens := []string{"solid", "dry"}
	active, msg := checkMissCluster(HookStatus{FrequentMissTokens: tokens, MissCount: len(tokens)})
	if !active {
		t.Error("expected active=true when FrequentMissTokens is non-empty")
	}
	if !strings.Contains(msg, "solid") || !strings.Contains(msg, "dry") {
		t.Errorf("expected token names in message, got %q", msg)
	}
	if !strings.Contains(msg, "KB entries") {
		t.Errorf("expected message to mention 'KB entries', got %q", msg)
	}
}

func TestCheckMissCluster_Silent_WhenEmpty(t *testing.T) {
	t.Parallel()
	active, msg := checkMissCluster(HookStatus{})
	if active {
		t.Errorf("expected active=false when FrequentMissTokens is nil, got msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("expected empty message, got %q", msg)
	}
}

// TestPopulateAdvisoryFields_MissTokens_FromIndex verifies that FrequentMissTokens
// and MissCount are read from idx.FrequentMissTokens.
func TestPopulateAdvisoryFields_MissTokens_FromIndex(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	idx := index.Index{
		BuildTime:          now,
		FrequentMissTokens: []string{"solid", "dry", "yagni"},
	}
	var hs HookStatus
	populateAdvisoryFields(&hs, &idx, now)
	if hs.MissCount != 3 {
		t.Errorf("MissCount: got %d, want 3", hs.MissCount)
	}
	if len(hs.FrequentMissTokens) != 3 {
		t.Errorf("FrequentMissTokens len: got %d, want 3", len(hs.FrequentMissTokens))
	}
	if hs.FrequentMissTokens[0] != "solid" {
		t.Errorf("FrequentMissTokens[0]: got %q, want solid", hs.FrequentMissTokens[0])
	}
}

// TestPopulateAdvisoryFields_MissTokens_EmptyWhenNone verifies that zero-value
// idx produces zero MissCount and nil FrequentMissTokens — preserving byte-identical stderr.
func TestPopulateAdvisoryFields_MissTokens_EmptyWhenNone(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	idx := index.Index{BuildTime: now}
	var hs HookStatus
	populateAdvisoryFields(&hs, &idx, now)
	if hs.MissCount != 0 {
		t.Errorf("MissCount: got %d, want 0", hs.MissCount)
	}
	if hs.FrequentMissTokens != nil {
		t.Errorf("FrequentMissTokens: got %v, want nil", hs.FrequentMissTokens)
	}
}

// TestAdvisoryStatusSegment_IndexAgeDays_ShownOnlyAboveThreshold verifies the
// byte-identical invariant: index_age_days only appears when > staleBuildThresholdDays.
func TestAdvisoryStatusSegment_IndexAgeDays_ShownOnlyAboveThreshold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		status       HookStatus
		wantContains string
		wantAbsent   string
	}{
		{
			name:       "exactly_at_threshold_silent",
			status:     HookStatus{IndexAgeDays: staleBuildThresholdDays},
			wantAbsent: "index_age_days",
		},
		{
			name:       "below_threshold_silent",
			status:     HookStatus{IndexAgeDays: 10},
			wantAbsent: "index_age_days",
		},
		{
			name:         "above_threshold_shown",
			status:       HookStatus{IndexAgeDays: 40},
			wantContains: "index_age_days=40",
		},
		{
			name:       "zero_all_fields_empty_string",
			status:     HookStatus{},
			wantAbsent: "index_age_days",
		},
		{
			name:         "above_threshold_with_rebuild_ms",
			status:       HookStatus{IndexAgeDays: 35, RebuildMs: 12},
			wantContains: "index_age_days=35",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := advisoryStatusSegment(tc.status)
			if tc.wantContains != "" && !strings.Contains(got, tc.wantContains) {
				t.Errorf("expected %q in segment, got %q", tc.wantContains, got)
			}
			if tc.wantAbsent != "" && strings.Contains(got, tc.wantAbsent) {
				t.Errorf("expected %q absent from segment, got %q", tc.wantAbsent, got)
			}
		})
	}
	// Byte-identical invariant: zero HookStatus → empty string.
	if got := advisoryStatusSegment(HookStatus{}); got != "" {
		t.Errorf("byte-identical invariant broken: got %q for zero HookStatus", got)
	}
}

// --- checkSuggestedAliases tests ---

func TestCheckSuggestedAliases_Silent_WhenZero(t *testing.T) {
	t.Parallel()
	active, msg := checkSuggestedAliases(HookStatus{SuggestedAliasCount: 0})
	if active {
		t.Errorf("expected active=false for SuggestedAliasCount=0, got msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("expected empty message, got %q", msg)
	}
}

func TestCheckSuggestedAliases_Silent_BelowMinCount(t *testing.T) {
	t.Parallel()
	active, msg := checkSuggestedAliases(HookStatus{SuggestedAliasCount: SuggestedAliasMinCount - 1})
	if active {
		t.Errorf("expected active=false below min count, got msg=%q", msg)
	}
}

func TestCheckSuggestedAliases_Activates_AtMinCount(t *testing.T) {
	t.Parallel()
	active, msg := checkSuggestedAliases(HookStatus{SuggestedAliasCount: SuggestedAliasMinCount})
	if !active {
		t.Errorf("expected active=true at min count %d", SuggestedAliasMinCount)
	}
	if !strings.Contains(msg, "suggested aliases") {
		t.Errorf("expected message to mention 'suggested aliases', got %q", msg)
	}
	if !strings.Contains(msg, fmt.Sprintf("%d", SuggestedAliasMinCount)) {
		t.Errorf("expected message to contain count %d, got %q", SuggestedAliasMinCount, msg)
	}
}

func TestCheckSuggestedAliases_Activates_AboveMinCount(t *testing.T) {
	t.Parallel()
	count := SuggestedAliasMinCount + 5
	active, msg := checkSuggestedAliases(HookStatus{SuggestedAliasCount: count})
	if !active {
		t.Errorf("expected active=true for SuggestedAliasCount=%d", count)
	}
	if !strings.Contains(msg, fmt.Sprintf("%d", count)) {
		t.Errorf("expected message to contain count %d, got %q", count, msg)
	}
}

// TestAdvisoryStatusSegment_SuggestedAliasCount_ZeroSuppressed verifies that
// SuggestedAliasCount below SuggestedAliasMinCount does NOT appear in the segment
// and does NOT cause a non-empty segment on its own.
func TestAdvisoryStatusSegment_SuggestedAliasCount_ZeroSuppressed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		count  int
		wantIn bool
	}{
		{name: "zero", count: 0, wantIn: false},
		{name: "below_min", count: SuggestedAliasMinCount - 1, wantIn: false},
		{name: "at_min", count: SuggestedAliasMinCount, wantIn: true},
		{name: "above_min", count: SuggestedAliasMinCount + 2, wantIn: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hs := HookStatus{SuggestedAliasCount: tc.count}
			got := advisoryStatusSegment(hs)
			if tc.wantIn && !strings.Contains(got, "suggested_alias_count") {
				t.Errorf("expected suggested_alias_count in segment for count=%d, got %q", tc.count, got)
			}
			if !tc.wantIn && strings.Contains(got, "suggested_alias_count") {
				t.Errorf("expected suggested_alias_count absent for count=%d, got %q", tc.count, got)
			}
		})
	}
	// Byte-identical invariant: zero SuggestedAliasCount alone must not produce a segment.
	if got := advisoryStatusSegment(HookStatus{SuggestedAliasCount: 0}); got != "" {
		t.Errorf("byte-identical invariant: SuggestedAliasCount=0 must produce empty segment, got %q", got)
	}
}

// TestPopulateAdvisoryFields_SuggestedAliasCount_FromIndex verifies wiring.
func TestPopulateAdvisoryFields_SuggestedAliasCount_FromIndex(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	idx := index.Index{
		BuildTime:           now,
		SuggestedAliasCount: 5,
	}
	var hs HookStatus
	populateAdvisoryFields(&hs, &idx, now)
	if hs.SuggestedAliasCount != 5 {
		t.Errorf("SuggestedAliasCount: got %d, want 5", hs.SuggestedAliasCount)
	}
}

// TestPopulateAdvisoryFields_SuggestedAliasCount_ZeroPreservesInvariant verifies
// that a zero SuggestedAliasCount produces no segment (byte-identical invariant).
func TestPopulateAdvisoryFields_SuggestedAliasCount_ZeroPreservesInvariant(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	idx := index.Index{BuildTime: now, SuggestedAliasCount: 0}
	var hs HookStatus
	populateAdvisoryFields(&hs, &idx, now)
	if hs.SuggestedAliasCount != 0 {
		t.Errorf("SuggestedAliasCount: got %d, want 0", hs.SuggestedAliasCount)
	}
	if got := advisoryStatusSegment(hs); got != "" {
		t.Errorf("byte-identical invariant: zero SuggestedAliasCount must produce empty segment, got %q", got)
	}
}
