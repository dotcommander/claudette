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
const CurrentVersion = 9

// Index is the on-disk cache of all scanned entries.
type Index struct {
	Version             int                `json:"version"`
	BuildTime           time.Time          `json:"build_time"`
	SourceMtime         time.Time          `json:"source_mtime"`
	FileCount           int                `json:"file_count"`
	AliasesMtime        time.Time          `json:"aliases_mtime,omitzero"`         // mtime of aliases.yaml at build time (zero if absent)
	ZeroKeywordCount    int                `json:"zero_keyword_count,omitzero"`    // entries with no keywords after parse (authoring signal)
	ColdEntryCount      int                `json:"cold_entry_count,omitzero"`      // entries never hit, >90d old, not brand-new (>7d old)
	FrequentMissTokens  []string           `json:"frequent_miss_tokens,omitempty"` // top missed tokens above threshold; empty when below
	SuggestedAliasCount int                `json:"suggested_alias_count,omitzero"` // entries with alias candidates pending review
	Entries             []Entry            `json:"entries"`
	IDF                 map[string]float64 `json:"idf,omitzero"`  // inverse document frequency per keyword
	AvgFieldLen         float64            `json:"avg_field_len"` // average number of keywords per entry (for BM25)
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

	now := time.Now()
	return Index{
		Version:          CurrentVersion,
		BuildTime:        now,
		SourceMtime:      scan.MaxMtime,
		FileCount:        scan.FileCount,
		AliasesMtime:     aliasesMtime,
		ZeroKeywordCount: countZeroKeyword(scan.Entries),
		ColdEntryCount:   countColdEntries(scan.Entries, now),
		Entries:          scan.Entries,
		IDF:              ComputeIDF(scan.Entries),
		AvgFieldLen:      ComputeAvgFieldLen(scan.Entries),
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

// countZeroKeyword returns the number of entries that have no keywords after
// parsing. A non-zero count is an authoring signal (frontmatter parse failure
// or empty-body file). Precomputed once at build time; advisory path reads it.
func countZeroKeyword(entries []Entry) int {
	n := 0
	for _, e := range entries {
		if len(e.Keywords) == 0 {
			n++
		}
	}
	return n
}

// coldEntryMinAge is the minimum file age for a cold-entry advisory.
// coldEntryGrace is the grace period for newly added entries.
const (
	coldEntryMinAge = 90 * 24 * time.Hour
	coldEntryGrace  = 7 * 24 * time.Hour
)

// countColdEntries returns the number of entries that have never been hit
// (HitCount == 0), are older than coldEntryMinAge (not freshly added),
// and are newer than coldEntryGrace (not brand-new). Dynamic: depends on now.
func countColdEntries(entries []Entry, now time.Time) int {
	n := 0
	for _, e := range entries {
		age := now.Sub(e.FileMtime)
		if e.HitCount == 0 && age > coldEntryMinAge && age > coldEntryGrace {
			n++
		}
	}
	return n
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
// Returns the elapsed rebuild duration when a rebuild happened; 0 on cache hit.
func LoadOrRebuild(sourceDirs []string) (Index, time.Duration, error) {
	cached, err := Load()
	if err == nil && !NeedsRebuild(cached, sourceDirs) {
		return cached, 0, nil
	}

	// Missing, corrupt, or stale — rebuild. Timing starts here, not at Load,
	// so we measure only the actual rebuild work.
	rebuildStart := time.Now()
	idx, err := buildIndex(sourceDirs)
	if err != nil {
		return Index{}, 0, err
	}

	now := time.Now()
	cutoff := now.Add(-usage.DefaultUsageLogTTL)

	// Compact usage log into entry hit counts (raw for diagnostics, decayed for scoring).
	// Drop records older than DefaultUsageLogTTL to bound on-disk log growth.
	records, logErr := usage.ParseUsageLog()
	if logErr == nil && len(records) > 0 {
		rawCounts := usage.AggregateHitCounts(records, cutoff)
		decayedCounts := usage.AggregateDecayedHitCounts(records, now, usage.DefaultHalfLife, cutoff)
		for i := range idx.Entries {
			name := idx.Entries[i].Name
			if c, ok := rawCounts[name]; ok {
				idx.Entries[i].HitCount = c
			}
			if c, ok := decayedCounts[name]; ok {
				idx.Entries[i].HitCountDecayed = c
			}
		}
		// Recompute ColdEntryCount now that HitCount values are accurate.
		idx.ColdEntryCount = countColdEntries(idx.Entries, now)
	}

	// Aggregate miss log: compact old records, then compute frequent missed tokens.
	// Best-effort — miss recording failure must never block the hook.
	_ = usage.TruncateMissLog(cutoff)
	missRecords, _ := usage.ParseMissLog()
	idx.FrequentMissTokens = usage.ComputeFrequentMissTokens(
		missRecords, now, cutoff,
		usage.MissClusterThreshold, usage.TopNMissTokens,
	)

	// Co-occurrence pipeline: compact old records, compute alias candidates, write advisory file.
	// Best-effort — failures must never block the hook.
	coPath, coPathErr := usage.CoOccurrenceLogPath()
	suggestionsPath, suggestionsPathErr := config.ConfigFilePath("suggested-aliases.yaml")
	if coPathErr == nil && suggestionsPathErr == nil {
		_ = usage.TruncateCoOccurrenceLog(coPath, cutoff)
		coRecords, _ := usage.ParseCoOccurrenceLog(coPath)
		suppressSet := buildSuggestionSuppressSet(idx.Entries)
		suggestions := usage.ComputeSuggestedAliases(coRecords, suppressSet)
		_ = usage.WriteSuggestedAliases(suggestionsPath, suggestions)
		idx.SuggestedAliasCount = len(suggestions)

		// Populate SuggestedAliases on each entry so the scorer can consume them.
		// Build entry-name → []token map from the computed suggestions.
		sugMap := make(map[string][]string, len(suggestions))
		for _, s := range suggestions {
			tokens := make([]string, len(s.Candidates))
			for i, c := range s.Candidates {
				tokens[i] = c.Token
			}
			sugMap[s.Entry] = tokens
		}
		for i := range idx.Entries {
			if toks, ok := sugMap[idx.Entries[i].Name]; ok {
				idx.Entries[i].SuggestedAliases = toks
			}
		}
	}

	// Best-effort save; compact the usage log to only survivors within the TTL window.
	if saveErr := Save(idx); saveErr == nil && logErr == nil && len(records) > 0 {
		_ = usage.TruncateUsageLog(cutoff)
	}
	return idx, time.Since(rebuildStart), nil
}
