package index

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"time"

	"github.com/dotcommander/claudette/internal/config"
	"github.com/dotcommander/claudette/internal/usage"
)

// CurrentVersion is the index schema version; bump when Entry shape changes.
const CurrentVersion = 4

// Index is the on-disk cache of all scanned entries.
type Index struct {
	Version      int                `json:"version"`
	BuildTime    time.Time          `json:"build_time"`
	SourceMtime  time.Time          `json:"source_mtime"`
	FileCount    int                `json:"file_count"`
	AliasesMtime time.Time          `json:"aliases_mtime,omitzero"` // mtime of aliases.yaml at build time (zero if absent)
	Entries      []Entry            `json:"entries"`
	IDF          map[string]float64 `json:"idf,omitzero"`  // inverse document frequency per keyword
	AvgFieldLen  float64            `json:"avg_field_len"` // average number of keywords per entry (for BM25)
}

// IndexPath returns ~/.config/claudette/index.json.
func IndexPath() (string, error) {
	return config.ConfigFilePath("index.json")
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
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONFile(path, data)
}

// buildIndex scans source directories and constructs a fresh Index.
func buildIndex(sourceDirs []string) (Index, error) {
	scan, err := Scan(sourceDirs)
	if err != nil {
		return Index{}, fmt.Errorf("scan failed: %w", err)
	}

	// Load user alias overrides and merge them before computing IDF/avgdl,
	// so the final keyword set reflects all aliases (frontmatter + overrides).
	overridesPath, err := aliasOverridesPath()
	if err != nil {
		return Index{}, err
	}
	overrides, aliasesMtime, err := loadAliasOverrides(overridesPath)
	if err != nil {
		return Index{}, err
	}
	applyAliasOverrides(scan.Entries, overrides)

	return Index{
		Version:      CurrentVersion,
		BuildTime:    time.Now(),
		SourceMtime:  scan.MaxMtime,
		FileCount:    scan.FileCount,
		AliasesMtime: aliasesMtime,
		Entries:      scan.Entries,
		IDF:          ComputeIDF(scan.Entries),
		AvgFieldLen:  ComputeAvgFieldLen(scan.Entries),
	}, nil
}

// ForceRebuild scans, builds, and persists a fresh index unconditionally.
func ForceRebuild(sourceDirs []string) (Index, error) {
	idx, err := buildIndex(sourceDirs)
	if err != nil {
		return Index{}, err
	}
	if err := Save(idx); err != nil {
		return Index{}, fmt.Errorf("saving index: %w", err)
	}
	return idx, nil
}

// NeedsRebuild checks whether the cached index is stale relative to source dirs.
// It also accounts for changes to ~/.config/claudette/aliases.yaml — both
// modification (mtime change) and deletion/creation (zero-vs-nonzero mtime).
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
	if maxMtime.After(cached.SourceMtime) {
		return true
	}

	// Check aliases.yaml staleness: compare current mtime against cached.
	// A zero mtime means "file absent". If existence or mtime changed, rebuild.
	var currentAliasesMtime time.Time
	if p, err := aliasOverridesPath(); err == nil {
		if info, err := os.Stat(p); err == nil {
			currentAliasesMtime = info.ModTime()
		}
	}
	return !currentAliasesMtime.Equal(cached.AliasesMtime)
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
	if err == nil && !NeedsRebuild(cached, sourceDirs) {
		return cached, nil
	}

	// Missing, corrupt, or stale — rebuild.
	idx, err := buildIndex(sourceDirs)
	if err != nil {
		return Index{}, err
	}

	// Compact usage log into entry hit counts.
	records, logErr := usage.ParseUsageLog()
	if logErr == nil && len(records) > 0 {
		counts := usage.AggregateHitCounts(records)
		for i := range idx.Entries {
			if c, ok := counts[idx.Entries[i].Name]; ok {
				idx.Entries[i].HitCount = c
			}
		}
	}

	// Best-effort save; failing to persist doesn't block usage.
	if saveErr := Save(idx); saveErr == nil && logErr == nil && len(records) > 0 {
		_ = usage.TruncateUsageLog()
	}
	return idx, nil
}
