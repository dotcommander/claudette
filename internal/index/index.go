package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const CurrentVersion = 1

// Index is the on-disk cache of all scanned entries.
type Index struct {
	Version     int       `json:"version"`
	BuildTime   time.Time `json:"build_time"`
	SourceMtime time.Time `json:"source_mtime"`
	FileCount   int       `json:"file_count"`
	Entries     []Entry   `json:"entries"`
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

	for _, dir := range sourceDirs {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".md" {
				return nil
			}
			count++
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.ModTime().After(maxMtime) {
				maxMtime = info.ModTime()
			}
			return nil
		})
	}

	if count != cached.FileCount {
		return true
	}
	if maxMtime.After(cached.SourceMtime) {
		return true
	}
	return false
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
	}
	// Best-effort save; failing to persist doesn't block usage.
	_ = Save(idx)
	return idx, nil
}
