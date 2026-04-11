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
	seen := make(map[string]struct{})
	var dirs []string

	add := func(p string) {
		if _, ok := seen[p]; ok {
			return
		}
		if _, err := os.Stat(p); err != nil {
			return
		}
		seen[p] = struct{}{}
		dirs = append(dirs, p)
	}

	// User-configured dirs take priority.
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	for _, d := range cfg.SourceDirs {
		add(d)
	}

	// Always include defaults (existing dirs only).
	defaults, err := DefaultSourceDirs()
	if err != nil {
		return nil, err
	}
	for _, d := range defaults {
		add(d)
	}

	// Plugin-installed components.
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	pluginsFile := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	data, err := os.ReadFile(pluginsFile)
	if err == nil {
		addPluginDirs(data, add)
	}

	return dirs, nil
}

type installedPlugins struct {
	Plugins map[string][]struct {
		InstallPath string `json:"installPath"`
	} `json:"plugins"`
}

func addPluginDirs(data []byte, add func(string)) {
	var installed installedPlugins
	if err := json.Unmarshal(data, &installed); err != nil {
		return
	}
	for _, installs := range installed.Plugins {
		for _, inst := range installs {
			if inst.InstallPath == "" {
				continue
			}
			for _, sub := range []string{"skills", "agents", "commands"} {
				add(filepath.Join(inst.InstallPath, sub))
			}
		}
	}
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

// Scan walks all source directories and builds entries.
// Returns entries, max mtime across all files, and total file count.
func Scan(sourceDirs []string) ([]Entry, time.Time, int, error) {
	var entries []Entry
	var maxMtime time.Time
	var fileCount int

	if err := walkMdFiles(sourceDirs, func(path string, info fs.FileInfo, sourceDir string) {
		fileCount++
		if info.ModTime().After(maxMtime) {
			maxMtime = info.ModTime()
		}
		entry, ok := parseEntry(path, sourceDir)
		if ok {
			entries = append(entries, entry)
		}
	}); err != nil {
		return nil, time.Time{}, 0, err
	}

	return entries, maxMtime, fileCount, nil
}
