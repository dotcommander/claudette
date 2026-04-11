package index

import (
	"strings"
	"unicode"
)

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

func isNameDelimiter(r rune) bool { return r == '-' || r == ':' }

func splitWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})
}

// truncateRunes returns s truncated to at most n runes.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
