package index

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// SourceDirs returns the directories to scan, combining user-configured
// source_dirs (from config.json) with defaults and plugin directories.
func SourceDirs() ([]string, error) {
	// User-configured dirs take priority.
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	candidates := append([]string(nil), cfg.SourceDirs...)

	// Always include defaults (existing dirs only).
	defaults, err := DefaultSourceDirs()
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, defaults...)

	// Plugin-installed components.
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	pluginsFile := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	if data, err := os.ReadFile(pluginsFile); err == nil {
		candidates = append(candidates, pluginDirs(data)...)
	}

	return deduplicateDirs(candidates), nil
}

// deduplicateDirs returns dirs in input order, keeping only existing paths
// and dropping duplicates (by path string).
func deduplicateDirs(dirs []string) []string {
	seen := make(map[string]struct{}, len(dirs))
	out := make([]string, 0, len(dirs))
	for _, p := range dirs {
		if _, ok := seen[p]; ok {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

type installedPlugins struct {
	Plugins map[string][]struct {
		InstallPath string `json:"installPath"`
	} `json:"plugins"`
}

// pluginDirs parses installed_plugins.json and returns all candidate plugin
// component directories (skills/agents/commands under each install path).
func pluginDirs(data []byte) []string {
	var installed installedPlugins
	if err := json.Unmarshal(data, &installed); err != nil {
		return nil
	}
	var dirs []string
	for _, installs := range installed.Plugins {
		for _, inst := range installs {
			if inst.InstallPath == "" {
				continue
			}
			for _, sub := range []string{"skills", "agents", "commands"} {
				dirs = append(dirs, filepath.Join(inst.InstallPath, sub))
			}
		}
	}
	return dirs
}

// walkMdFiles walks all source directories and calls fn for each .md file found.
func walkMdFiles(dirs []string, fn func(path string, info fs.FileInfo, sourceDir string)) error {
	for _, dir := range dirs {
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			fn(path, info, dir)
			return nil
		}); err != nil {
			return fmt.Errorf("walking %s: %w", dir, err)
		}
	}
	return nil
}

// ScanResult holds the output of a directory scan.
type ScanResult struct {
	Entries   []Entry
	MaxMtime  time.Time
	FileCount int
}

// Scan walks all source directories and builds entries.
func Scan(sourceDirs []string) (ScanResult, error) {
	var r ScanResult
	if err := walkMdFiles(sourceDirs, func(path string, info fs.FileInfo, sourceDir string) {
		r.FileCount++
		if info.ModTime().After(r.MaxMtime) {
			r.MaxMtime = info.ModTime()
		}
		entry, ok := parseEntry(path, sourceDir)
		if ok {
			r.Entries = append(r.Entries, entry)
		}
	}); err != nil {
		return ScanResult{}, err
	}
	return r, nil
}
