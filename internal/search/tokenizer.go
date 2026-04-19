package search

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
)

var (
	// rePathLike matches whitespace-separated tokens containing a slash, e.g. /var/log/app.log.
	// Replaced before tokenization to prevent directory components flooding query terms.
	rePathLike = regexp.MustCompile(`\S*/\S*`)

	// reXMLTag matches XML/HTML-ish tags, e.g. <agentId>. Content is preserved; tag names are dropped.
	reXMLTag = regexp.MustCompile(`<[^>]+>`)
)

// negationMarkers is the fixed set of words that negate the immediately-following
// content token. Adding a new marker is a one-line addition here — no new code paths.
// Markers are always suppressed from output regardless of stop-word membership.
//
//nolint:gochecknoglobals // immutable lookup table
var negationMarkers = map[string]struct{}{
	"not": {}, "no": {}, "without": {}, "avoid": {}, "never": {}, "instead": {},
}

// StopSet is a set of words to filter during tokenization.
type StopSet map[string]struct{}

// Contains reports whether the set includes the given word.
func (s StopSet) Contains(word string) bool {
	_, ok := s[word]
	return ok
}

//nolint:gochecknoglobals // immutable lookup table
var defaultStops StopSet

//nolint:gochecknoglobals // immutable lookup table
var defaultStopsOnce sync.Once

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
			"use", "using", "was", "way", "who", "why", "you",
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
			// Conversational — suppress chat noise from reaching scorer.
			"awesome", "cool", "done", "fine", "lgtm", "nice",
			"ok", "okay", "perfect", "please", "right", "sounds",
			"thanks", "yep",
			// Generic nouns — appear too broadly to discriminate between entries.
			// "product" and "public" scored unrelated skills on casual prompts.
			// "thing" and "work" already present above; not duplicated here.
			"public", "product", "platform", "feature", "system", "component", "module",
			"content", "context", "project", "application", "interface", "service",
			"item", "resource", "result", "value", "data", "type", "default",
			"base", "core", "common", "general", "basic", "simple", "standard", "option",
			"state", "mode", "config", "style", "form", "page", "view", "audience",
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
//
// Negation pass: tokens immediately following a negation marker (not, no,
// without, avoid, never, instead) are dropped. Stop words between the marker
// and the content token are transparent — "not a rust project" drops "rust".
// Negation markers themselves are always suppressed from output.
func Tokenize(prompt string, stops StopSet) []string {
	// Pre-strip XML-ish tags first (keeps content, removes tag names), then
	// path-like tokens (containing /). Tag strip must precede path strip because
	// closing tags (</tag>) contain "/" and would otherwise be treated as paths,
	// swallowing tag content along with them.
	prompt = reXMLTag.ReplaceAllString(prompt, " ")
	prompt = rePathLike.ReplaceAllString(prompt, " ")

	lower := strings.ToLower(prompt)

	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})

	// Negation pass: walk raw token stream before stop-word filtering.
	// negated holds the single content token immediately following each marker
	// (transparent to intervening stop words). Scope is always exactly one token.
	negated := buildNegatedSet(fields, stops)

	seen := make(map[string]struct{}, len(fields))
	result := make([]string, 0, len(fields))

	for _, raw := range fields {
		tok := strings.Trim(raw, "-")
		if len(tok) <= 1 {
			continue
		}
		if stops.Contains(tok) {
			continue
		}
		if _, isMarker := negationMarkers[tok]; isMarker {
			// Markers are always consumed — never emitted, even if not a stop word.
			continue
		}
		if _, ok := negated[tok]; ok {
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

// buildNegatedSet scans the raw token list (post-split, pre-filter) and returns
// the set of content tokens that immediately follow a negation marker.
// Stop words between a marker and its target token are transparent.
func buildNegatedSet(fields []string, stops StopSet) map[string]struct{} {
	negated := map[string]struct{}{}
	pendingNegation := false

	for _, raw := range fields {
		tok := strings.Trim(raw, "-")
		if len(tok) <= 1 {
			continue
		}
		if _, isMarker := negationMarkers[tok]; isMarker {
			pendingNegation = true
			continue
		}
		if !pendingNegation {
			continue
		}
		if stops.Contains(tok) {
			// Transparent stop word between marker and content — keep scanning.
			continue
		}
		// First content token after the marker: negate it, reset flag.
		negated[tok] = struct{}{}
		pendingNegation = false
	}

	return negated
}
