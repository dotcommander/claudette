package index

import (
	"strings"
	"testing"
)

const skillFixture = `---
name: test-skill
description: A test skill
---

# Test Skill

Some preamble text that isn't very useful for scoring.

## Quick Reference

| Intent | Tool |
|--------|------|
| "fix bug" | Read → Edit |

## When to Use

Use PROACTIVELY when user mentions "refactor", "code smell", or "tech debt".

## Details

More detailed content here...
`

func TestBodyContentSections_PriorityBeforePreamble(t *testing.T) {
	t.Parallel()
	result := bodyContentSections([]byte(skillFixture), 500)

	// "When to Use" and "Quick Reference" are priority sections —
	// their content must appear before the preamble prose.
	preambleIdx := strings.Index(result, "preamble text")
	triggerIdx := strings.Index(result, "PROACTIVELY")
	quickRefIdx := strings.Index(result, "fix bug")

	if triggerIdx < 0 {
		t.Fatal("expected trigger content (PROACTIVELY) in result, not found")
	}
	if quickRefIdx < 0 {
		t.Fatal("expected quick reference content (fix bug) in result, not found")
	}
	if preambleIdx >= 0 && triggerIdx > preambleIdx {
		t.Errorf("expected trigger content before preamble: triggerIdx=%d preambleIdx=%d", triggerIdx, preambleIdx)
	}
	if preambleIdx >= 0 && quickRefIdx > preambleIdx {
		t.Errorf("expected quick reference content before preamble: quickRefIdx=%d preambleIdx=%d", quickRefIdx, preambleIdx)
	}
}

func TestBodyContentSections_FrontmatterStripped(t *testing.T) {
	t.Parallel()
	result := bodyContentSections([]byte(skillFixture), 500)
	if strings.Contains(result, "name: test-skill") {
		t.Error("frontmatter should be stripped from body content")
	}
}

func TestBodyContentSections_RespectsBudget(t *testing.T) {
	t.Parallel()
	result := bodyContentSections([]byte(skillFixture), 50)
	runes := []rune(result)
	if len(runes) > 50 {
		t.Errorf("expected at most 50 runes, got %d", len(runes))
	}
}

func TestBodyContentSections_NoPrioritySections(t *testing.T) {
	t.Parallel()
	noPriority := `---
name: plain
description: no priority headers
---

# Plain Doc

Just some plain text with no special sections.

## More Plain Text

Still nothing special here.
`
	result := bodyContentSections([]byte(noPriority), 500)
	if !strings.Contains(result, "plain text") {
		t.Error("expected fallback content when no priority sections exist")
	}
}

func TestBodyContentSections_NilFrontmatter(t *testing.T) {
	t.Parallel()
	noFM := `# Just a title

Some content without any frontmatter at all.
`
	result := bodyContentSections([]byte(noFM), 500)
	if !strings.Contains(result, "content without any frontmatter") {
		t.Error("expected body content when no frontmatter present")
	}
}
