package index

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// SourceDirs returns the default directories to scan.
func SourceDirs() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

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

	// KB directory
	add(filepath.Join(home, ".claude", "kb"))

	// User-defined components
	add(filepath.Join(home, ".claude", "skills"))
	add(filepath.Join(home, ".claude", "agents"))
	add(filepath.Join(home, ".claude", "commands"))

	// Plugin-installed components
	pluginsFile := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	data, err := os.ReadFile(pluginsFile)
	if err == nil {
		addPluginDirs(data, &dirs, seen)
	}

	return dirs, nil
}

type installedPlugins struct {
	Plugins map[string][]struct {
		InstallPath string `json:"installPath"`
	} `json:"plugins"`
}

func addPluginDirs(data []byte, dirs *[]string, seen map[string]struct{}) {
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
				p := filepath.Join(inst.InstallPath, sub)
				if _, ok := seen[p]; ok {
					continue
				}
				if _, err := os.Stat(p); err != nil {
					continue
				}
				seen[p] = struct{}{}
				*dirs = append(*dirs, p)
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

func parseEntry(path, sourceDir string) (Entry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, false
	}

	entryType := classifyType(path, sourceDir)
	name := filenameStem(path)
	title := extractTitle(data)
	category := extractCategory(path, sourceDir, entryType)
	var description string

	if entryType != TypeKB {
		fm, _ := ParseFrontmatter(data)
		if fm.Name != "" {
			name = fm.Name
		}
		if fm.Description != "" {
			description = fm.Description
			if title == "" {
				title = fm.Description
			}
		}
	}

	keywords := extractKeywords(name, title, category, description)
	return Entry{
		Type:     entryType,
		Name:     name,
		Title:    firstNonEmpty(title, name),
		Category: category,
		FilePath: path,
		Keywords: keywords,
	}, true
}

func classifyType(path, sourceDir string) EntryType {
	rel, _ := filepath.Rel(sourceDir, path)
	parts := strings.Split(rel, string(filepath.Separator))

	// If sourceDir ends with a type indicator, use that
	base := filepath.Base(sourceDir)
	switch base {
	case "kb":
		return TypeKB
	case "skills":
		return TypeSkill
	case "agents":
		return TypeAgent
	case "commands":
		return TypeCommand
	}

	// Fallback: check first path component
	if len(parts) > 0 {
		switch parts[0] {
		case "kb":
			return TypeKB
		case "skills":
			return TypeSkill
		case "agents":
			return TypeAgent
		case "commands":
			return TypeCommand
		}
	}
	return TypeKB
}

func extractTitle(data []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNum := 0
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		if lineNum == 1 && line == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if line == "---" {
				inFrontmatter = false
			}
			continue
		}
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

func extractCategory(path, sourceDir string, entryType EntryType) string {
	if entryType == TypeKB {
		// For KB, category is the immediate parent directory name
		rel, _ := filepath.Rel(sourceDir, path)
		dir := filepath.Dir(rel)
		if dir != "." {
			return filepath.Base(dir)
		}
		return "kb"
	}
	// For components, category is the type name
	return string(entryType)
}

func extractKeywords(name, title, category, description string) []string {
	seen := make(map[string]struct{})
	var keywords []string

	add := func(word string) {
		w := strings.ToLower(word)
		if len(w) <= 1 {
			return
		}
		if _, ok := seen[w]; ok {
			return
		}
		seen[w] = struct{}{}
		keywords = append(keywords, w)
	}

	// From filename (split on hyphens)
	for _, part := range strings.Split(name, "-") {
		add(part)
	}

	// From title (split on non-alnum)
	for _, part := range splitWords(title) {
		add(part)
	}

	// Category itself
	add(category)

	// From description (first 200 runes for performance)
	if description != "" {
		desc := []rune(description)
		if len(desc) > 200 {
			desc = desc[:200]
		}
		for _, part := range splitWords(string(desc)) {
			add(part)
		}
	}

	return keywords
}

func splitWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})
}

func filenameStem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
