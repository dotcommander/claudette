package usage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/dotcommander/claudette/internal/config"
)

// Co-occurrence logging constants.
const (
	// MaxUnmatchedPerRecord caps unmatched tokens stored per record.
	MaxUnmatchedPerRecord = 10
	// SuggestAliasThreshold is the minimum co-occurrence count before a token
	// is surfaced as an alias candidate for a given entry.
	SuggestAliasThreshold = 5
	// TopNAliasCandidatesPerEntry caps alias candidates returned per entry.
	TopNAliasCandidatesPerEntry = 3
)

// CoOccurrenceRecord is one line in cooccurrence.log: a timestamped snapshot
// of unmatched prompt tokens that co-occurred with a set of hit entries.
type CoOccurrenceRecord struct {
	Timestamp       time.Time `json:"timestamp"`
	UnmatchedTokens []string  `json:"unmatched_tokens"`
	HitEntries      []string  `json:"hit_entries"`
}

// CoOccurrenceLogPath returns ~/.config/claudette/cooccurrence.log.
func CoOccurrenceLogPath() (string, error) {
	return config.ConfigFilePath("cooccurrence.log")
}

// AppendCoOccurrenceLog appends rec to the co-occurrence log.
// Skips silently when rec.UnmatchedTokens is empty after capping.
// Mirrors AppendMissLog: one JSON object per line, append-only open.
func AppendCoOccurrenceLog(path string, rec CoOccurrenceRecord) (err error) {
	if len(rec.UnmatchedTokens) == 0 {
		return nil
	}
	// Cap unmatched tokens to avoid unbounded growth per record.
	if len(rec.UnmatchedTokens) > MaxUnmatchedPerRecord {
		rec.UnmatchedTokens = rec.UnmatchedTokens[:MaxUnmatchedPerRecord]
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
	return json.NewEncoder(f).Encode(rec)
}

// ParseCoOccurrenceLog reads cooccurrence.log and returns all records.
// Returns nil, nil when the file does not exist.
// Mirrors ParseMissLog: JSON decoder, skip malformed lines.
func ParseCoOccurrenceLog(path string) ([]CoOccurrenceRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var records []CoOccurrenceRecord
	dec := json.NewDecoder(f)
	for dec.More() {
		var r CoOccurrenceRecord
		if err := dec.Decode(&r); err != nil {
			continue // skip malformed lines
		}
		records = append(records, r)
	}
	return records, nil
}

// TruncateCoOccurrenceLog rewrites cooccurrence.log retaining only records
// at or after cutoff. Atomically replaces the file. No-op when file absent.
// Mirrors TruncateMissLog: parse → filter → temp-file-then-rename.
func TruncateCoOccurrenceLog(path string, cutoff time.Time) error {
	records, err := ParseCoOccurrenceLog(path)
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
	tmp, err := os.CreateTemp(dir, "cooccurrence-*.log.tmp")
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
