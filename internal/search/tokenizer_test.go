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
	// "without" is a negation marker — it negates "paths" and is itself consumed.
	// "or" is a stop word. Surviving tokens: "plain", "prompt", "tags".
	tokens := Tokenize("plain prompt without paths or tags", stops)

	expected := []string{"plain", "prompt", "tags"}
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
	// "without" is a negation marker — consumed, must not appear.
	if slices.Contains(tokens, "without") {
		t.Errorf("Tokenize: negation marker %q should not appear in output, got tokens=%v", "without", tokens)
	}
	// "paths" is negated by "without" — must not appear.
	if slices.Contains(tokens, "paths") {
		t.Errorf("Tokenize: negated token %q should be absent, got tokens=%v", "paths", tokens)
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

// --- Negation tests ---

func TestTokenize_NegationDropsFollowingToken(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// "not rust" — "rust" must be dropped; nothing else should be affected.
	tokens := Tokenize("not rust", stops)

	if slices.Contains(tokens, "rust") {
		t.Errorf("Tokenize: negated token %q should be absent, got tokens=%v", "rust", tokens)
	}
}

func TestTokenize_NegationJumpsStopWord(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// "not a rust goroutine" — "a" is a stop word between the marker and "rust".
	// Negation must jump the stop word and negate "rust". "goroutine" survives.
	// (Updated from "project" which is now a stop word.)
	tokens := Tokenize("not a rust goroutine", stops)

	if slices.Contains(tokens, "rust") {
		t.Errorf("Tokenize: negated token %q should be absent after stop-word jump, got tokens=%v", "rust", tokens)
	}
	if !slices.Contains(tokens, "goroutine") {
		t.Errorf("Tokenize: non-negated token %q should be present, got tokens=%v", "goroutine", tokens)
	}
}

func TestTokenize_NoNegation_Unchanged(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// No negation marker present — both tokens must survive (regression guard).
	// (Updated from "rust project" since "project" is now a stop word.)
	tokens := Tokenize("rust goroutine", stops)

	if !slices.Contains(tokens, "rust") {
		t.Errorf("Tokenize: expected %q without negation, got tokens=%v", "rust", tokens)
	}
	if !slices.Contains(tokens, "goroutine") {
		t.Errorf("Tokenize: expected %q without negation, got tokens=%v", "goroutine", tokens)
	}
}

func TestTokenize_CrossLanguageBleed(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// Full prompt: "I'm NOT using Rust, I'm using Go" — "rust" must be absent,
	// "go" must be present (prevents cross-language alias bleed).
	// "using" is a stop word, transparent to the negation — "not" skips it
	// and negates "rust" directly.
	tokens := Tokenize("I'm NOT using Rust, I'm using Go", stops)

	if slices.Contains(tokens, "rust") {
		t.Errorf("Tokenize: negated language %q should be absent, got tokens=%v", "rust", tokens)
	}
	if !slices.Contains(tokens, "go") {
		t.Errorf("Tokenize: non-negated language %q should be present, got tokens=%v", "go", tokens)
	}
}

func TestTokenize_NegationAtStart(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// Negation marker at the very start of input — must still work.
	tokens := Tokenize("not rust", stops)

	if slices.Contains(tokens, "rust") {
		t.Errorf("Tokenize: negation at start: %q should be absent, got tokens=%v", "rust", tokens)
	}
}

func TestTokenize_DoubleNegation_Ignored(t *testing.T) {
	t.Parallel()

	stops := DefaultStopWords()
	// "not not rust": both "not" tokens are markers; each sets pendingNegation=true.
	// The second marker resets the flag before any content token is consumed,
	// then "rust" becomes the target of the second negation.
	// Documented behavior: "rust" is absent regardless of double negation.
	tokens := Tokenize("not not rust", stops)

	if slices.Contains(tokens, "rust") {
		t.Errorf("Tokenize: double negation: %q should still be absent, got tokens=%v", "rust", tokens)
	}
}
