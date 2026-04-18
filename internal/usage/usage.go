package usage

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// AggregateHitCounts sums hit counts per entry name from parsed log records.
func AggregateHitCounts(records []UsageRecord) map[string]int {
	counts := make(map[string]int, len(records))
	for _, r := range records {
		counts[r.Name]++
	}
	return counts
}

// TruncateUsageLog truncates the usage log to zero bytes.
func TruncateUsageLog() error {
	path, err := UsageLogPath()
	if err != nil {
		return err
	}
	return truncateUsageLogWithPath(path)
}

func truncateUsageLogWithPath(path string) error {
	return os.Truncate(path, 0)
}
