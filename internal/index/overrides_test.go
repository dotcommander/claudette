package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadAliasOverrides_Missing verifies that a missing file returns (nil, zero, nil).
func TestLoadAliasOverrides_Missing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases.yaml")
	overrides, mtime, err := loadAliasOverrides(path)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if overrides != nil {
		t.Errorf("expected nil overrides for missing file, got %v", overrides)
	}
	if !mtime.IsZero() {
		t.Errorf("expected zero mtime for missing file, got %v", mtime)
	}
}

// TestLoadAliasOverrides_Malformed verifies that invalid YAML returns an empty
// (nil) map without crashing. The mtime is still captured (file exists).
func TestLoadAliasOverrides_Malformed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases.yaml")
	if err := os.WriteFile(path, []byte("aliases: {: bad yaml\x00}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	overrides, mtime, err := loadAliasOverrides(path)
	// Malformed YAML is logged to stderr but does not propagate an error.
	if err != nil {
		t.Fatalf("expected nil error for malformed YAML, got %v", err)
	}
	if overrides != nil {
		t.Errorf("expected nil overrides for malformed YAML, got %v", overrides)
	}
	// mtime should be non-zero because the file exists.
	if mtime.IsZero() {
		t.Error("expected non-zero mtime for existing (malformed) file")
	}
}

// TestLoadAliasOverrides_Valid verifies a well-formed file parses correctly.
func TestLoadAliasOverrides_Valid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases.yaml")
	content := `aliases:
  skillify:
    - crystallize session into reusable skill
    - turn this pattern into a skill
  handoff:
    - durable state across resets
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	overrides, mtime, err := loadAliasOverrides(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mtime.IsZero() {
		t.Error("expected non-zero mtime")
	}
	if len(overrides) != 2 {
		t.Fatalf("expected 2 slug entries, got %d: %v", len(overrides), overrides)
	}
	if len(overrides["skillify"]) != 2 {
		t.Errorf("expected 2 skillify aliases, got %v", overrides["skillify"])
	}
	if len(overrides["handoff"]) != 1 {
		t.Errorf("expected 1 handoff alias, got %v", overrides["handoff"])
	}
}

// TestIndexBuild_OverrideAppliesToEntry builds an index from a temp dir with a
// fake entry, writes an aliases.yaml targeting that entry by slug, then verifies
// the entry's keyword set includes tokens from the override phrase.
func TestIndexBuild_OverrideAppliesToEntry(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create a minimal skill entry.
	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	entryContent := `---
name: my-tool
description: A generic tool
---

# My Tool

Does something useful.
`
	if err := os.WriteFile(filepath.Join(skillsDir, "my-tool.md"), []byte(entryContent), 0o644); err != nil {
		t.Fatalf("WriteFile entry: %v", err)
	}

	// Write aliases.yaml that targets "my-tool" with a distinctive phrase.
	cfgDir := filepath.Join(tmp, ".config", "claudette")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir cfg: %v", err)
	}
	aliasContent := `aliases:
  my-tool:
    - crystallize frobnicate widget
`
	if err := os.WriteFile(filepath.Join(cfgDir, "aliases.yaml"), []byte(aliasContent), 0o644); err != nil {
		t.Fatalf("WriteFile aliases: %v", err)
	}

	idx, err := buildIndex([]string{skillsDir})
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	var entry *Entry
	for i := range idx.Entries {
		if idx.Entries[i].Name == "my-tool" {
			entry = &idx.Entries[i]
			break
		}
	}
	if entry == nil {
		t.Fatal("entry 'my-tool' not found in index")
	}

	// "crystallize" and "frobnicate" and "widget" come exclusively from the override.
	for _, tok := range []string{"crystallize", "frobnicate", "widget"} {
		if _, ok := entry.Keywords[tok]; !ok {
			t.Errorf("expected keyword %q from alias override, not found; keywords=%v", tok, entry.Keywords)
		}
	}
}

// TestIndexStaleness_DetectsAliasesYamlChange verifies that changing aliases.yaml
// (different mtime) causes NeedsRebuild to return true.
func TestIndexStaleness_DetectsAliasesYamlChange(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfgDir := filepath.Join(tmp, ".config", "claudette")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	aliasPath := filepath.Join(cfgDir, "aliases.yaml")

	// Write initial aliases.yaml and record its mtime.
	if err := os.WriteFile(aliasPath, []byte("aliases:\n  foo:\n    - bar\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Stat(aliasPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	firstMtime := info.ModTime()

	// Build a cached index that records the first mtime.
	cached := Index{
		Version:      CurrentVersion,
		AliasesMtime: firstMtime,
		FileCount:    0,
		SourceMtime:  time.Time{},
	}

	// Initially not stale (same mtime).
	if NeedsRebuild(cached, nil) {
		t.Error("expected NeedsRebuild=false when aliases.yaml mtime matches cached")
	}

	// Advance mtime by backdating the file to a different time.
	newTime := firstMtime.Add(-2 * time.Second) // past, definitely different
	if err := os.Chtimes(aliasPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if !NeedsRebuild(cached, nil) {
		t.Error("expected NeedsRebuild=true after aliases.yaml mtime changed")
	}
}

// TestIndexStaleness_DetectsAliasesYamlDeletion verifies that deleting aliases.yaml
// (cached has non-zero mtime, current has zero) triggers a rebuild.
func TestIndexStaleness_DetectsAliasesYamlDeletion(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Cached index claims aliases.yaml had a specific mtime.
	cached := Index{
		Version:      CurrentVersion,
		AliasesMtime: time.Now().Add(-time.Hour),
		FileCount:    0,
		SourceMtime:  time.Time{},
	}

	// aliases.yaml does not exist (HOME points to empty tmp dir).
	if !NeedsRebuild(cached, nil) {
		t.Error("expected NeedsRebuild=true when cached has aliases mtime but file is absent")
	}
}
