package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dotcommander/claudette/internal/config"
)

// UsageRecord is a single parsed line from the usage log.
type UsageRecord struct {
	Timestamp time.Time
	Name      string
	Score     int
}

// UsageLogPath returns ~/.config/claudette/usage.log.
func UsageLogPath() (string, error) {
	return config.ConfigFilePath("usage.log")
}

// AppendUsageLog opens the log in append mode and writes one TSV line per record.
// Format: <unix_timestamp>\t<entry_name>\t<score>\n
func AppendUsageLog(records []UsageRecord) error {
	path, err := UsageLogPath()
	if err != nil {
		return err
	}
	return appendUsageLogWithPath(path, records)
}

func appendUsageLogWithPath(path string, records []UsageRecord) (err error) {
	if len(records) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	bw := bufio.NewWriter(f)
	for _, r := range records {
		//nolint:errcheck // disk errors surfaced by Flush()
		fmt.Fprintf(bw, "%d\t%s\t%d\n", r.Timestamp.Unix(), r.Name, r.Score)
	}
	return bw.Flush()
}

// ParseUsageLog reads the log and returns all records.
// Returns nil, nil if the file does not exist.
func ParseUsageLog() ([]UsageRecord, error) {
	path, err := UsageLogPath()
	if err != nil {
		return nil, err
	}
	return parseUsageLogWithPath(path)
}

func parseUsageLogWithPath(path string) ([]UsageRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var records []UsageRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue // skip malformed lines
		}
		ts, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		score, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		records = append(records, UsageRecord{
			Timestamp: time.Unix(ts, 0),
			Name:      parts[1],
			Score:     score,
		})
	}
	return records, sc.Err()
}

// DefaultUsageLogTTL is the maximum age of usage records kept on disk and in
// aggregation. Records older than this are dropped during compaction.
const DefaultUsageLogTTL = 90 * 24 * time.Hour

// DefaultHalfLife is 70 days (≈ λ=0.01 per day), meaning a hit 70 days ago
// contributes half as much as a hit today.
const DefaultHalfLife = 70 * 24 * time.Hour

// AggregateHitCounts sums hit counts per entry name from parsed log records.
// Records with Timestamp before cutoff are ignored. Pass a zero time.Time to
// include all records.
// Kept for diagnostics; scorer uses AggregateDecayedHitCounts.
func AggregateHitCounts(records []UsageRecord, cutoff time.Time) map[string]int {
	counts := make(map[string]int, len(records))
	for _, r := range records {
		if !cutoff.IsZero() && r.Timestamp.Before(cutoff) {
			continue
		}
		counts[r.Name]++
	}
	return counts
}

// AggregateDecayedHitCounts computes a time-decayed hit count per entry name.
// Each record contributes weight = 0.5^(age/halfLife) to its entry's total.
// A recent hit contributes ≈1.0; a hit exactly one halfLife ago contributes 0.5.
// Records with Timestamp before cutoff are ignored. Pass a zero time.Time to
// include all records.
//
// FL-2: negative-signal source plugs in here; see Task #10 miss-path recording.
func AggregateDecayedHitCounts(records []UsageRecord, now time.Time, halfLife time.Duration, cutoff time.Time) map[string]float64 {
	counts := make(map[string]float64, len(records))
	halfLifeDays := halfLife.Hours() / 24
	for _, r := range records {
		if !cutoff.IsZero() && r.Timestamp.Before(cutoff) {
			continue
		}
		ageDays := now.Sub(r.Timestamp).Hours() / 24
		weight := math.Pow(0.5, ageDays/halfLifeDays)
		counts[r.Name] += weight
	}
	return counts
}

// TruncateUsageLog rewrites the usage log retaining only records at or after
// cutoff, atomically replacing the file. Pass a zero time.Time to keep all
// records (equivalent to the old zero-truncate behaviour with survivors=all).
func TruncateUsageLog(cutoff time.Time) error {
	path, err := UsageLogPath()
	if err != nil {
		return err
	}
	return truncateUsageLogWithPath(path, cutoff)
}

func truncateUsageLogWithPath(path string, cutoff time.Time) error {
	records, err := parseUsageLogWithPath(path)
	if err != nil {
		// If the file doesn't exist there's nothing to compact.
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	// Collect survivors — records within the TTL window.
	survivors := records[:0]
	for _, r := range records {
		if cutoff.IsZero() || !r.Timestamp.Before(cutoff) {
			survivors = append(survivors, r)
		}
	}

	// Write survivors to a temp file then rename for atomicity.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "usage-*.log.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	bw := bufio.NewWriter(tmp)
	for _, r := range survivors {
		//nolint:errcheck // disk errors surfaced by Flush()
		fmt.Fprintf(bw, "%d\t%s\t%d\n", r.Timestamp.Unix(), r.Name, r.Score)
	}
	if err := bw.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// --- Miss log ---

// MissRecord is a single line in the miss log: a timestamped set of tokens
// that found zero results in the index.
type MissRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Tokens    []string  `json:"tokens"`
}

// MissLogPath returns ~/.config/claudette/misses.log.
func MissLogPath() (string, error) {
	return config.ConfigFilePath("misses.log")
}

// AppendMissLog appends a MissRecord (one JSON object per line) to misses.log.
// Returns silently on any I/O error so miss recording never blocks the hook.
// Skips empty token slices.
func AppendMissLog(tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	path, err := MissLogPath()
	if err != nil {
		return err
	}
	return appendMissLogWithPath(path, tokens)
}

func appendMissLogWithPath(path string, tokens []string) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	rec := MissRecord{Timestamp: time.Now().UTC(), Tokens: tokens}
	enc := json.NewEncoder(f)
	return enc.Encode(rec)
}

// ParseMissLog reads misses.log and returns all records.
// Returns nil, nil if the file does not exist.
func ParseMissLog() ([]MissRecord, error) {
	path, err := MissLogPath()
	if err != nil {
		return nil, err
	}
	return parseMissLogWithPath(path)
}

func parseMissLogWithPath(path string) ([]MissRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var records []MissRecord
	dec := json.NewDecoder(f)
	for dec.More() {
		var r MissRecord
		if err := dec.Decode(&r); err != nil {
			continue // skip malformed lines
		}
		records = append(records, r)
	}
	return records, nil
}

// TruncateMissLog rewrites misses.log retaining only records at or after cutoff.
// Atomically replaces the file. No-op when the file does not exist.
func TruncateMissLog(cutoff time.Time) error {
	path, err := MissLogPath()
	if err != nil {
		return err
	}
	return truncateMissLogWithPath(path, cutoff)
}

func truncateMissLogWithPath(path string, cutoff time.Time) error {
	records, err := parseMissLogWithPath(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	survivors := records[:0]
	for _, r := range records {
		if cutoff.IsZero() || !r.Timestamp.Before(cutoff) {
			survivors = append(survivors, r)
		}
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "misses-*.log.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	enc := json.NewEncoder(tmp)
	for _, r := range survivors {
		if encErr := enc.Encode(r); encErr != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return encErr
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// MissClusterThreshold is the minimum number of miss records a token must
// appear in (within the TTL window) to be surfaced as a frequent miss.
// TopNMissTokens is the maximum number of tokens to return.
const (
	MissClusterThreshold = 3
	TopNMissTokens       = 5
)

// ComputeFrequentMissTokens counts how many miss records each token appears in
// (within the TTL window), then returns the top-N tokens that appear in at
// least threshold records. Result is sorted descending by frequency, then
// alphabetically. Returns nil when no token meets the threshold.
func ComputeFrequentMissTokens(records []MissRecord, now time.Time, cutoff time.Time, threshold int, topN int) []string {
	freq := make(map[string]int)
	for _, r := range records {
		if !cutoff.IsZero() && r.Timestamp.Before(cutoff) {
			continue
		}
		_ = now // kept for future time-decay extension; cutoff is the active filter
		seen := make(map[string]bool, len(r.Tokens))
		for _, tok := range r.Tokens {
			if seen[tok] {
				continue // count once per record, not per occurrence within record
			}
			seen[tok] = true
			freq[tok]++
		}
	}

	type kv struct {
		token string
		count int
	}
	var candidates []kv
	for tok, count := range freq {
		if count >= threshold {
			candidates = append(candidates, kv{tok, count})
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].count != candidates[j].count {
			return candidates[i].count > candidates[j].count
		}
		return candidates[i].token < candidates[j].token
	})

	if len(candidates) > topN {
		candidates = candidates[:topN]
	}
	result := make([]string, len(candidates))
	for i, c := range candidates {
		result[i] = c.token
	}
	return result
}
