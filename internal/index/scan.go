package index

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/claudette/internal/config"
)

// SourceDirs returns the directories to scan, combining user-configured
// source_dirs (from config.json) with defaults and plugin directories.
func SourceDirs() ([]string, error) {
	// User-configured dirs take priority.
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	candidates := append([]string(nil), cfg.SourceDirs...)

	// Always include defaults (existing dirs only).
	defaults, err := config.DefaultSourceDirs()
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

// pluginManifest represents the relevant fields of .claude-plugin/plugin.json.
// Commands may be a single path string or a list of path strings.
type pluginManifest struct {
	Commands json.RawMessage `json:"commands"`
	Skills   json.RawMessage `json:"skills"`
}

// pluginSubdirs returns all component directories for a single plugin install
// path. It reads .claude-plugin/plugin.json to discover declared directories,
// falling back to the conventional "skills", "agents", "commands" if the
// manifest is absent or unparseable.
func pluginSubdirs(installPath string) []string {
	manifestPath := filepath.Join(installPath, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		// No manifest — use conventional dirs.
		return conventionalSubdirs(installPath)
	}

	var m pluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return conventionalSubdirs(installPath)
	}

	var dirs []string

	// skills: string or array of strings.
	dirs = append(dirs, resolveManifestPaths(installPath, m.Skills)...)

	// agents: convention only (not declared in manifest spec).
	dirs = append(dirs, filepath.Join(installPath, "agents"))

	// commands: string or array of strings.
	dirs = append(dirs, resolveManifestPaths(installPath, m.Commands)...)

	return dirs
}

// resolveManifestPaths decodes a JSON field that may be a string or []string,
// resolves each value as a path relative to installPath, and returns the results.
func resolveManifestPaths(installPath string, raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var paths []string
	// Try []string first.
	if err := json.Unmarshal(raw, &paths); err != nil {
		// Try single string.
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return nil
		}
		paths = []string{s}
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		// Strip leading "./" if present.
		p = strings.TrimPrefix(p, "./")
		// Strip trailing "/" if present.
		p = strings.TrimSuffix(p, "/")
		out = append(out, filepath.Join(installPath, p))
	}
	return out
}

// conventionalSubdirs returns the standard skills/agents/commands paths for a
// plugin that has no plugin.json manifest.
func conventionalSubdirs(installPath string) []string {
	subs := []string{"skills", "agents", "commands"}
	out := make([]string, 0, len(subs))
	for _, s := range subs {
		out = append(out, filepath.Join(installPath, s))
	}
	return out
}

// pluginDirs parses installed_plugins.json and returns all candidate plugin
// component directories. It reads each plugin's manifest to discover declared
// command/skill directories, so plugins that split commands across multiple
// subdirectories (e.g. dev-commands, session-commands) are fully indexed.
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
			dirs = append(dirs, pluginSubdirs(inst.InstallPath)...)
		}
	}
	return dirs
}

// walkMdFiles walks all source directories and calls fn for each .md file found.
func walkMdFiles(dirs []string, fn func(path string, info fs.FileInfo, sourceDir string)) error {
	for _, dir := range dirs {
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
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
		entry, ok := parseEntry(path, sourceDir, info.ModTime())
		if ok {
			r.Entries = append(r.Entries, entry)
		}
	}); err != nil {
		return ScanResult{}, err
	}
	return r, nil
}
