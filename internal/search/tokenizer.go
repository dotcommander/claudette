package search

import (
	"strings"
	"sync"
	"unicode"
)

// StopSet is a set of words to filter during tokenization.
type StopSet map[string]struct{}

// Contains reports whether the set includes the given word.
func (s StopSet) Contains(word string) bool {
	_, ok := s[word]
	return ok
}

var (
	defaultStops     StopSet
	defaultStopsOnce sync.Once
)

// DefaultStopWords returns the built-in stop word list.
func DefaultStopWords() StopSet {
	defaultStopsOnce.Do(func() {
		words := []string{
			"a", "an", "the", "and", "or", "but", "if", "in", "on", "at",
			"to", "for", "of", "by", "is", "it", "be", "as", "do", "we",
			"so", "no", "up", "he", "me", "my", "am",
			"all", "any", "are", "can", "did", "get", "got", "had", "has",
			"have", "her", "him", "his", "how", "its", "let", "may", "not",
			"now", "old", "our", "out", "own", "run", "say", "she", "too",
			"use", "was", "way", "who", "why", "you",
			"about", "after", "also", "back", "been", "call", "come",
			"could", "does", "down", "each", "else", "even", "every",
			"find", "from", "give", "good", "great", "help", "here",
			"into", "just", "keep", "know", "like", "long", "look",
			"made", "make", "many", "more", "most", "much", "must",
			"need", "next", "only", "over", "part", "same", "should",
			"show", "some", "such", "sure", "take", "tell", "than",
			"that", "them", "then", "there", "these", "they", "this",
			"very", "want", "well", "were", "what", "when", "where",
			"which", "will", "with", "work", "would", "yeah", "your",
			"before", "being", "between", "check", "doing",
			"don", "going", "gonna", "never", "often", "other",
			"really", "something", "still", "stuff", "thing", "things",
			"think", "those", "under", "while",
		}
		defaultStops = make(StopSet, len(words))
		for _, w := range words {
			defaultStops[w] = struct{}{}
		}
	})
	return defaultStops
}

// Tokenize lowercases the input, splits on non-alphanumeric boundaries
// (preserving internal hyphens), deduplicates, and removes stop words
// and single-char tokens.
func Tokenize(prompt string, stops StopSet) []string {
	lower := strings.ToLower(prompt)

	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})

	seen := make(map[string]struct{}, len(fields))
	result := make([]string, 0, len(fields))

	for _, tok := range fields {
		tok = strings.Trim(tok, "-")
		if len(tok) <= 1 {
			continue
		}
		if stops.Contains(tok) {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		result = append(result, tok)
	}
	return result
}
