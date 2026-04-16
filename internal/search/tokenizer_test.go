package search

import (
	"slices"
	"testing"
)

func TestTokenize_PathStripping(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// Input mixes a path token with standalone words. The entire path token
	// (/path/to/project) is stripped as path-like; standalone words survive.
	// "path", "to", "project" would appear as noise without path stripping.
	tokens := Tokenize("/path/to/project foo bar baz", stops)

	forbidden := []string{"path", "to", "project"}
	for _, f := range forbidden {
		if slices.Contains(tokens, f) {
			t.Errorf("Tokenize: path component %q should have been stripped, got tokens=%v", f, tokens)
		}
	}

	// "project" is part of the path token and must not appear.
	if slices.Contains(tokens, "project") {
		t.Errorf("Tokenize: path component %q should have been stripped (was part of path token), got tokens=%v", "project", tokens)
	}

	// Standalone words after the path must survive.
	required := []string{"foo", "bar", "baz"}
	for _, r := range required {
		if stops.Contains(r) {
			continue
		}
		if !slices.Contains(tokens, r) {
			t.Errorf("Tokenize: expected %q in tokens after path strip, got tokens=%v", r, tokens)
		}
	}
}

func TestTokenize_XMLTagStripping(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// "now" is a stopword; "read" is a stopword. Only "abc123" should survive.
	tokens := Tokenize("read <agentId>abc123</agentId> now", stops)

	if !slices.Contains(tokens, "abc123") {
		t.Errorf("Tokenize: expected content %q to survive XML tag stripping, got tokens=%v", "abc123", tokens)
	}
	if slices.Contains(tokens, "agentid") {
		t.Errorf("Tokenize: tag name %q should have been stripped, got tokens=%v", "agentid", tokens)
	}
}

func TestTokenize_RegressionPlainPrompt(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// Plain prompt with no paths or tags: verify non-stopwords survive and stopwords are absent.
	tokens := Tokenize("plain prompt without paths or tags", stops)

	expected := []string{"plain", "prompt", "without", "paths", "tags"}
	for _, e := range expected {
		if stops.Contains(e) {
			continue
		}
		if !slices.Contains(tokens, e) {
			t.Errorf("Tokenize: expected %q in tokens for plain prompt, got tokens=%v", e, tokens)
		}
	}
	// "or" is a stopword — must not appear.
	if slices.Contains(tokens, "or") {
		t.Errorf("Tokenize: stopword %q should be absent, got tokens=%v", "or", tokens)
	}
}

func TestTokenize_HyphenPreservation(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()

	t.Run("internal hyphens preserved as single token", func(t *testing.T) {
		t.Parallel()

		tokens := Tokenize("refactor tech-debt and multi-page docs", stops)

		// Internal hyphens must not be split: tech-debt and multi-page are single tokens.
		if !slices.Contains(tokens, "tech-debt") {
			t.Errorf("Tokenize: expected %q as a single token (hyphen preserved), got tokens=%v", "tech-debt", tokens)
		}
		if !slices.Contains(tokens, "multi-page") {
			t.Errorf("Tokenize: expected %q as a single token (hyphen preserved), got tokens=%v", "multi-page", tokens)
		}

		// Fragments must not appear separately.
		for _, frag := range []string{"tech", "debt", "multi", "page"} {
			if slices.Contains(tokens, frag) {
				t.Errorf("Tokenize: fragment %q should not appear separately when hyphen is preserved, got tokens=%v", frag, tokens)
			}
		}
	})

	t.Run("leading and trailing hyphens stripped", func(t *testing.T) {
		t.Parallel()

		tokens := Tokenize("-foo bar-", stops)

		if !slices.Contains(tokens, "foo") {
			t.Errorf("Tokenize: expected %q after leading-hyphen strip, got tokens=%v", "foo", tokens)
		}
		if !slices.Contains(tokens, "bar") {
			t.Errorf("Tokenize: expected %q after trailing-hyphen strip, got tokens=%v", "bar", tokens)
		}
		// Hyphen-prefixed / hyphen-suffixed forms must not appear.
		for _, bad := range []string{"-foo", "bar-"} {
			if slices.Contains(tokens, bad) {
				t.Errorf("Tokenize: token %q should not appear after hyphen strip, got tokens=%v", bad, tokens)
			}
		}
	})
}

func TestTokenize_Deduplication(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// "test" repeated three times (one mixed-case) — must appear exactly once.
	tokens := Tokenize("test test TEST", stops)

	count := 0
	for _, tok := range tokens {
		if tok == "test" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Tokenize: expected %q exactly once after dedup, got count=%d tokens=%v", "test", count, tokens)
	}
}
