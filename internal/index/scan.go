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
	var tags []string

	// Parse frontmatter for all entry types (tags are universal)
	fm, _ := ParseFrontmatter(data)
	tags = fm.Tags

	if entryType != TypeKB {
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

	body := bodyContent(data, 500)
	keywords := extractKeywords(name, title, category, description, tags, body)

	// Add trigger words at weight 2
	add := makeAdder(keywords)
	for _, tw := range extractTriggerWords(body) {
		add(tw, 2)
	}

	bigrams := extractBigrams(title)

	return Entry{
		Type:     entryType,
		Name:     name,
		Title:    firstNonEmpty(title, name),
		Category: category,
		FilePath: path,
		Keywords: keywords,
		Bigrams:  bigrams,
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

// extractKeywords builds a weighted keyword map from all available fields.
// Weights: name=3, title=2, category=2, tags=2, description=1, body=1.
func extractKeywords(name, title, category, description string, tags []string, body string) map[string]int {
	kw := make(map[string]int)
	add := makeAdder(kw)

	// Name — weight 3 (strongest signal)
	add(name, 3)
	for _, part := range strings.FieldsFunc(name, isNameDelimiter) {
		add(part, 3)
	}

	// Title — weight 2
	for _, part := range splitWords(title) {
		add(part, 2)
	}

	// Category — weight 2
	add(category, 2)

	// Frontmatter tags — weight 2.
	// splitWords handles space/punctuation-delimited tags; strings.Split("-") indexes hyphen parts.
	for _, tag := range tags {
		for _, part := range splitWords(tag) {
			add(part, 2)
		}
		for _, part := range strings.Split(tag, "-") {
			add(part, 2)
		}
	}

	// Description and body — weight 1
	for _, part := range splitWords(truncateRunes(description, 500)) {
		add(part, 1)
	}
	for _, part := range splitWords(body) {
		add(part, 1)
	}

	return kw
}

func isNameDelimiter(r rune) bool { return r == '-' || r == ':' }

// makeAdder returns a closure that upserts into kw, keeping the highest weight per word.
func makeAdder(kw map[string]int) func(string, int) {
	return func(word string, weight int) {
		w := strings.ToLower(word)
		if len(w) <= 1 {
			return
		}
		if cur, ok := kw[w]; !ok || weight > cur {
			kw[w] = weight
		}
	}
}

// truncateRunes returns s truncated to at most n runes.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// bodyContent returns the first maxRunes runes of content after frontmatter.
func bodyContent(data []byte, maxRunes int) string {
	if bytes.IndexByte(data, '\r') >= 0 {
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	}
	body := data
	if bytes.HasPrefix(data, []byte("---\n")) {
		rest := data[4:]
		if end := bytes.Index(rest, []byte("---\n")); end >= 0 {
			body = rest[end+4:]
		}
	}
	runes := []rune(string(body))
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return string(runes)
}

// extractTriggerWords pulls quoted words from TRIGGER/PROACTIVELY lines.
func extractTriggerWords(body string) []string {
	var words []string
	for _, line := range strings.Split(body, "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "trigger when") &&
			!strings.Contains(lower, "use proactively when") &&
			!strings.Contains(lower, "use when") {
			continue
		}
		remaining := line
		for {
			start := strings.IndexByte(remaining, '"')
			if start < 0 {
				break
			}
			remaining = remaining[start+1:]
			end := strings.IndexByte(remaining, '"')
			if end < 0 {
				break
			}
			quoted := remaining[:end]
			remaining = remaining[end+1:]
			for _, w := range splitWords(quoted) {
				if len(w) > 1 {
					words = append(words, w)
				}
			}
		}
	}
	return words
}

// extractBigrams generates consecutive word pairs from a title.
func extractBigrams(title string) []string {
	words := splitWords(title)
	if len(words) < 2 {
		return nil
	}
	bigrams := make([]string, 0, len(words)-1)
	for i := range len(words) - 1 {
		bigrams = append(bigrams, words[i]+" "+words[i+1])
	}
	return bigrams
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
