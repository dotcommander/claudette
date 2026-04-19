package index

import (
	"bytes"
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func parseEntry(path, sourceDir string, mtime time.Time) (Entry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, false
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	entryType := classifyType(path, sourceDir)
	name := filenameStem(path)
	title := extractTitle(data)
	category := extractCategory(path, sourceDir, entryType)
	var description string
	var tags []string

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

	body := bodyContentSections(data, 500)
	keywords := extractKeywords(name, title, category, description, tags, body)

	add := makeAdder(keywords)
	for _, tw := range extractTriggerWords(body) {
		add(tw, 2)
	}

	// Aliases: natural-language synonyms that feed into keyword matching.
	// Each alias phrase is tokenized and its tokens added at weight 1.
	for _, alias := range fm.Aliases {
		for _, tok := range splitWords(alias) {
			add(tok, 1)
		}
	}

	return Entry{
		Type:      entryType,
		Name:      name,
		Title:     cmp.Or(title, name),
		Desc:      truncateRunes(description, 200),
		Category:  category,
		FilePath:  path,
		FileMtime: mtime,
		Keywords:  keywords,
		Bigrams:   extractBigrams(title),
	}, true
}

// dirTypeMap maps exact directory basenames to their entry type.
// Immutable at package level — safe per Go rules.
var dirTypeMap = map[string]EntryType{
	"kb":       TypeKB,
	"skills":   TypeSkill,
	"agents":   TypeAgent,
	"commands": TypeCommand,
}

func classifyType(path, sourceDir string) EntryType {
	rel, _ := filepath.Rel(sourceDir, path)
	parts := strings.Split(rel, string(filepath.Separator))
	candidates := []string{filepath.Base(sourceDir)}
	if len(parts) > 0 {
		candidates = append(candidates, parts[0])
	}
	for _, name := range candidates {
		if t, ok := dirTypeMap[name]; ok {
			return t
		}
		if strings.HasSuffix(name, "-commands") {
			return TypeCommand
		}
	}
	return TypeKB
}

func extractTitle(data []byte) string {
	for _, line := range strings.Split(string(stripFrontmatter(data)), "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

func extractCategory(path, sourceDir string, entryType EntryType) string {
	if entryType == TypeKB {
		rel, _ := filepath.Rel(sourceDir, path)
		dir := filepath.Dir(rel)
		if dir != "." {
			return filepath.Base(dir)
		}
		return "kb"
	}
	return string(entryType)
}

// prioritySectionKeywords are header substrings (lowercased) that indicate
// high-signal sections worth surfacing before generic preamble.
var prioritySectionKeywords = []string{
	"trigger", "when to use", "use when", "use proactively",
	"quick reference", "usage", "overview",
	"description", "what", "how",
}

// bodyContentSections returns up to maxRunes runes of body content, preferring
// sections whose headers contain high-signal keywords over generic preamble.
func bodyContentSections(data []byte, maxRunes int) string {
	body := stripFrontmatter(data)
	lines := strings.Split(string(body), "\n")

	type section struct {
		header string
		body   string
	}

	var sections []section
	var cur strings.Builder
	curHeader := ""

	flush := func(nextHeader string) {
		sections = append(sections, section{header: curHeader, body: cur.String()})
		cur.Reset()
		curHeader = nextHeader
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			flush(strings.ToLower(strings.TrimLeft(line, "# ")))
		} else {
			cur.WriteString(line)
			cur.WriteByte('\n')
		}
	}
	flush("")

	var priority, fallback []string
	for _, s := range sections {
		if isPrioritySection(s.header) {
			priority = append(priority, s.body)
		} else {
			fallback = append(fallback, s.body)
		}
	}

	var sb strings.Builder
	for _, part := range priority {
		sb.WriteString(part)
	}
	for _, part := range fallback {
		sb.WriteString(part)
	}

	return truncateRunes(sb.String(), maxRunes)
}

func stripFrontmatter(data []byte) []byte {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return data
	}
	rest := data[4:]
	if end := bytes.Index(rest, []byte("---\n")); end >= 0 {
		return rest[end+4:]
	}
	return data
}

func isPrioritySection(header string) bool {
	return slices.ContainsFunc(prioritySectionKeywords, func(kw string) bool {
		return strings.Contains(header, kw)
	})
}

func filenameStem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
