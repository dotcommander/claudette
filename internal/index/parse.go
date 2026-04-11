package index

import (
	"bufio"
	"bytes"
	"cmp"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func parseEntry(path, sourceDir string) (Entry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, false
	}
	// Normalize line endings once — callees no longer need to handle \r\n.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

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

	body := bodyContentSections(data, 500)
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
		Title:    cmp.Or(title, name),
		Desc:     truncateRunes(description, 200),
		Category: category,
		FilePath: path,
		Keywords: keywords,
		Bigrams:  bigrams,
	}, true
}

func classifyType(path, sourceDir string) EntryType {
	// Check source dir base name first; fall back to the first relative path component.
	rel, _ := filepath.Rel(sourceDir, path)
	parts := strings.Split(rel, string(filepath.Separator))
	candidates := []string{filepath.Base(sourceDir)}
	if len(parts) > 0 {
		candidates = append(candidates, parts[0])
	}
	for _, name := range candidates {
		switch name {
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

// prioritySectionKeywords are header substrings (lowercased) that indicate
// high-signal sections worth surfacing before generic preamble.
var prioritySectionKeywords = []string{
	"trigger", "when to use", "use when", "use proactively",
	"quick reference", "usage", "overview",
	"description", "what", "how",
}

// bodyContentSections returns up to maxRunes runes of body content, preferring
// sections whose headers contain high-signal keywords (trigger phrases, usage,
// overview, etc.) over generic preamble prose.
//
// Algorithm:
//  1. Strip YAML frontmatter.
//  2. Split by markdown section headers (## / ###).
//  3. Rank sections: priority headers first, then the rest in document order.
//  4. Concatenate and truncate to maxRunes.
func bodyContentSections(data []byte, maxRunes int) string {
	// Strip frontmatter.
	body := data
	if bytes.HasPrefix(data, []byte("---\n")) {
		rest := data[4:]
		if end := bytes.Index(rest, []byte("---\n")); end >= 0 {
			body = rest[end+4:]
		}
	}

	text := string(body)
	lines := strings.Split(text, "\n")

	// Split into sections. A section is [header line, ...body lines].
	// Index 0 holds everything before the first ## / ### header.
	type section struct {
		header string // lowercased header text (empty for pre-header content)
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
	flush("") // flush last section

	// Partition into priority and fallback.
	isPriority := func(header string) bool {
		for _, kw := range prioritySectionKeywords {
			if strings.Contains(header, kw) {
				return true
			}
		}
		return false
	}

	var priority, fallback []string
	for _, s := range sections {
		if isPriority(s.header) {
			priority = append(priority, s.body)
		} else {
			fallback = append(fallback, s.body)
		}
	}

	// Concatenate: priority sections first, then fallback.
	var sb strings.Builder
	for _, part := range priority {
		sb.WriteString(part)
	}
	for _, part := range fallback {
		sb.WriteString(part)
	}

	runes := []rune(sb.String())
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
