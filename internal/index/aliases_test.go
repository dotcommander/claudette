package index

import (
	"os"
	"testing"
)

const aliasFixture = `---
name: maps-keys-returns-iterator
title: maps.Keys returns iterator, not slice (Go 1.22)
aliases:
  - pulling keys out of a go map
  - extracting map keys
  - iter.Seq instead of slice
---

# maps.Keys returns iterator, not slice (Go 1.22)

Use maps.Keys from the maps package to get an iterator over map keys.
`

func TestParseFrontmatter_AliasesField(t *testing.T) {
	t.Parallel()

	fm, err := ParseFrontmatter([]byte(aliasFixture))
	if err != nil {
		t.Fatalf("ParseFrontmatter error: %v", err)
	}

	want := []string{
		"pulling keys out of a go map",
		"extracting map keys",
		"iter.Seq instead of slice",
	}

	if len(fm.Aliases) != len(want) {
		t.Fatalf("expected %d aliases, got %d: %v", len(want), len(fm.Aliases), fm.Aliases)
	}
	for i, a := range want {
		if fm.Aliases[i] != a {
			t.Errorf("alias[%d]: want %q, got %q", i, a, fm.Aliases[i])
		}
	}
}

func TestParseEntry_AliasTokensInKeywords(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := dir + "/maps-keys-returns-iterator.md"
	if err := os.WriteFile(path, []byte(aliasFixture), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entry, ok := parseEntry(path, dir)
	if !ok {
		t.Fatal("parseEntry returned ok=false")
	}

	// Tokens derived exclusively from alias phrases (not in name or title).
	// "pulling" and "extracting" are alias-only; "seq" comes from "iter.Seq".
	wantTokens := []string{"pulling", "extracting", "seq"}
	for _, tok := range wantTokens {
		if _, found := entry.Keywords[tok]; !found {
			t.Errorf("expected keyword %q from aliases, not found; keywords=%v", tok, entry.Keywords)
		}
	}
}
