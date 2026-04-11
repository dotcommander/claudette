package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"time"
)

// CurrentVersion is the index schema version; bump when Entry shape changes.
const CurrentVersion = 3

// Index is the on-disk cache of all scanned entries.
type Index struct {
	Version     int                `json:"version"`
	BuildTime   time.Time          `json:"build_time"`
	SourceMtime time.Time          `json:"source_mtime"`
	FileCount   int                `json:"file_count"`
	Entries     []Entry            `json:"entries"`
	IDF         map[string]float64 `json:"idf,omitzero"`  // inverse document frequency per keyword
	AvgFieldLen float64            `json:"avg_field_len"` // average number of keywords per entry (for BM25)
}

// IndexPath returns ~/.config/claudette/index.json.
func IndexPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "claudette", "index.json"), nil
}

// Load reads the index from disk. Returns os.ErrNotExist if missing.
func Load() (Index, error) {
	path, err := IndexPath()
	if err != nil {
		return Index{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Index{}, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, fmt.Errorf("corrupt index: %w", err)
	}
	return idx, nil
}

// Save writes the index atomically using temp-file-then-rename.
func Save(idx Index) error {
	path, err := IndexPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".claudette-index-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// NeedsRebuild checks whether the cached index is stale relative to source dirs.
func NeedsRebuild(cached Index, sourceDirs []string) bool {
	if cached.Version != CurrentVersion {
		return true
	}

	var maxMtime time.Time
	var count int

	if err := walkMdFiles(sourceDirs, func(_ string, info fs.FileInfo, _ string) {
		count++
		if info.ModTime().After(maxMtime) {
			maxMtime = info.ModTime()
		}
	}); err != nil {
		return true
	}

	if count != cached.FileCount {
		return true
	}
	return maxMtime.After(cached.SourceMtime)
}

// ComputeAvgFieldLen returns the average number of keywords per entry.
// Used as the avgdl denominator in BM25 length normalization.
func ComputeAvgFieldLen(entries []Entry) float64 {
	if len(entries) == 0 {
		return 0
	}
	totalKw := 0
	for _, e := range entries {
		totalKw += len(e.Keywords)
	}
	return float64(totalKw) / float64(len(entries))
}

// ComputeIDF calculates dampened inverse document frequency for all keywords.
// Returns a multiplier ranging from 0.5 (ubiquitous) to 2.0 (unique).
func ComputeIDF(entries []Entry) map[string]float64 {
	n := float64(len(entries))
	if n <= 1 {
		return nil
	}

	df := make(map[string]int)
	for _, e := range entries {
		for word := range e.Keywords {
			df[word]++
		}
	}

	logN := math.Log(n)
	idf := make(map[string]float64, len(df))
	for word, count := range df {
		ratio := math.Log(n/float64(count)) / logN
		idf[word] = 0.5 + 1.5*ratio
	}
	return idf
}

// LoadOrRebuild loads the index and rebuilds it if stale.
func LoadOrRebuild(sourceDirs []string) (Index, error) {
	cached, err := Load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Corrupt or unreadable — rebuild
		cached = Index{}
	}

	needsRebuild := errors.Is(err, os.ErrNotExist) || NeedsRebuild(cached, sourceDirs)
	if !needsRebuild {
		return cached, nil
	}

	entries, maxMtime, fileCount, scanErr := Scan(sourceDirs)
	if scanErr != nil {
		return Index{}, fmt.Errorf("scan failed: %w", scanErr)
	}

	idx := Index{
		Version:     CurrentVersion,
		BuildTime:   time.Now(),
		SourceMtime: maxMtime,
		FileCount:   fileCount,
		Entries:     entries,
		IDF:         ComputeIDF(entries),
		AvgFieldLen: ComputeAvgFieldLen(entries),
	}

	// Compact usage log into entry hit counts.
	records, logErr := ParseUsageLog()
	if logErr == nil && len(records) > 0 {
		counts := AggregateHitCounts(records)
		for i := range idx.Entries {
			if c, ok := counts[idx.Entries[i].Name]; ok {
				idx.Entries[i].HitCount = c
			}
		}
	}

	// Best-effort save; failing to persist doesn't block usage.
	if saveErr := Save(idx); saveErr == nil && logErr == nil && len(records) > 0 {
		_ = TruncateUsageLog()
	}
	return idx, nil
}
