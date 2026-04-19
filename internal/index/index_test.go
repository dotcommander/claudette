package index

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Pure function tests (no HOME mutation — t.Parallel() safe) ---

func TestComputeIDF_EmptyEntries_ReturnsNil(t *testing.T) {
	t.Parallel()
	if got := ComputeIDF(nil); got != nil {
		t.Errorf("ComputeIDF(nil) must return nil, got %v", got)
	}
	if got := ComputeIDF([]Entry{}); got != nil {
		t.Errorf("ComputeIDF([]Entry{}) must return nil, got %v", got)
	}
}

func TestComputeIDF_SingleEntry_ReturnsNil(t *testing.T) {
	t.Parallel()
	entries := []Entry{{Keywords: kw("foo")}}
	if got := ComputeIDF(entries); got != nil {
		t.Errorf("ComputeIDF with single entry must return nil, got %v", got)
	}
}

func TestComputeIDF_UniqueKeywordGetsHighestBoost(t *testing.T) {
	t.Parallel()
	// 5 entries; "unique" appears in 1, "common" appears in all 5.
	unique := "unique"
	common := "common"
	entries := make([]Entry, 5)
	for i := range entries {
		entries[i] = Entry{Keywords: kw(common)}
	}
	entries[0].Keywords[unique] = 1

	idf := ComputeIDF(entries)
	if idf == nil {
		t.Fatal("expected non-nil IDF map")
	}
	if idf[unique] <= idf[common] {
		t.Errorf("unique keyword IDF (%v) must exceed common keyword IDF (%v)", idf[unique], idf[common])
	}
}

func TestComputeIDF_UbiquitousKeywordCollapsesToHalf(t *testing.T) {
	t.Parallel()
	// A keyword in all N entries: log(N/N)=0 → 0.5 + 1.5*0 = 0.5.
	const n = 5
	entries := make([]Entry, n)
	for i := range entries {
		entries[i] = Entry{Keywords: kw("common")}
	}
	idf := ComputeIDF(entries)
	if idf == nil {
		t.Fatal("expected non-nil IDF map")
	}
	if math.Abs(idf["common"]-0.5) > 1e-9 {
		t.Errorf("ubiquitous keyword IDF: want 0.5, got %v", idf["common"])
	}
}

func TestComputeIDF_RangeInvariant(t *testing.T) {
	t.Parallel()
	// All IDF values must fall in [0.5, 2.0].
	entries := []Entry{
		{Keywords: kw("a", "b")},
		{Keywords: kw("b", "c")},
		{Keywords: kw("c", "d")},
		{Keywords: kw("d", "e")},
		{Keywords: kw("e", "f")},
	}
	idf := ComputeIDF(entries)
	if idf == nil {
		t.Fatal("expected non-nil IDF map")
	}
	for word, val := range idf {
		if val < 0.5 || val > 2.0 {
			t.Errorf("IDF[%q] = %v is outside [0.5, 2.0]", word, val)
		}
	}
}

func TestComputeAvgFieldLen_EmptyReturnsZero(t *testing.T) {
	t.Parallel()
	if got := ComputeAvgFieldLen([]Entry{}); got != 0.0 {
		t.Errorf("empty entries: want 0.0, got %v", got)
	}
}

func TestComputeAvgFieldLen_Average(t *testing.T) {
	t.Parallel()
	// 2, 4, 6 keywords → average 4.0
	entries := []Entry{
		{Keywords: kw("a", "b")},
		{Keywords: kw("a", "b", "c", "d")},
		{Keywords: kw("a", "b", "c", "d", "e", "f")},
	}
	if got := ComputeAvgFieldLen(entries); math.Abs(got-4.0) > 1e-9 {
		t.Errorf("want 4.0, got %v", got)
	}
}

func TestComputeAvgFieldLen_SingleEntry(t *testing.T) {
	t.Parallel()
	entries := []Entry{{Keywords: kw("x", "y", "z")}}
	if got := ComputeAvgFieldLen(entries); math.Abs(got-3.0) > 1e-9 {
		t.Errorf("single entry with 3 keywords: want 3.0, got %v", got)
	}
}

// kw builds a Keywords map (map[string]int) from variadic string tokens with weight 1.
func kw(tokens ...string) map[string]int {
	m := make(map[string]int, len(tokens))
	for _, tok := range tokens {
		m[tok] = 1
	}
	return m
}

// --- NeedsRebuild pure-logic test (early-exit, t.Parallel() safe) ---

func TestNeedsRebuild_VersionChanged_ReturnsTrue(t *testing.T) {
	t.Parallel()
	// Version mismatch triggers early return — no filesystem access needed.
	cached := Index{Version: CurrentVersion - 1}
	if !NeedsRebuild(cached, nil) {
		t.Error("stale version must require rebuild")
	}
}

// --- Integration tests (mutate HOME — cannot run in parallel) ---

// seedSkillFile creates ~/.claude/skills/<name>.md with the given content.
func seedSkillFile(t *testing.T, tmp, name, content string) {
	t.Helper()
	dir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("seedSkillFile: mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("seedSkillFile: write: %v", err)
	}
}

const minimalSkill = `---
name: x
title: X Skill
description: test skill for index tests
---

# X Skill

Content for test skill.
`

func TestLoadOrRebuild_FreshInstall_BuildsAndSaves(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	seedSkillFile(t, tmp, "x", minimalSkill)
	sourceDirs := []string{filepath.Join(tmp, ".claude", "skills")}

	idx, err := LoadOrRebuild(sourceDirs)
	if err != nil {
		t.Fatalf("LoadOrRebuild: %v", err)
	}
	if len(idx.Entries) < 1 {
		t.Errorf("expected at least 1 entry, got %d", len(idx.Entries))
	}
	if idx.Version != CurrentVersion {
		t.Errorf("version: want %d, got %d", CurrentVersion, idx.Version)
	}

	// Index must have been saved to disk.
	indexPath, err := IndexPath()
	if err != nil {
		t.Fatalf("IndexPath: %v", err)
	}
	if _, err := os.Stat(indexPath); err != nil {
		t.Errorf("index.json should exist after LoadOrRebuild: %v", err)
	}
}

func TestLoadOrRebuild_CachedAndFresh_ReturnsCache(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	seedSkillFile(t, tmp, "x", minimalSkill)
	sourceDirs := []string{filepath.Join(tmp, ".claude", "skills")}

	// First call — builds and saves.
	idx1, err := LoadOrRebuild(sourceDirs)
	if err != nil {
		t.Fatalf("first LoadOrRebuild: %v", err)
	}

	// Second call — must hit cache, not rebuild.
	idx2, err := LoadOrRebuild(sourceDirs)
	if err != nil {
		t.Fatalf("second LoadOrRebuild: %v", err)
	}

	// BuildTime must be identical if cache was used (rebuild would set time.Now()).
	if !idx1.BuildTime.Equal(idx2.BuildTime) {
		t.Errorf("BuildTime changed: first=%v second=%v — cache was not used", idx1.BuildTime, idx2.BuildTime)
	}
}

func TestLoadOrRebuild_VersionMismatch_Rebuilds(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	seedSkillFile(t, tmp, "x", minimalSkill)

	// Write a stale index.json with an old version.
	indexPath, err := IndexPath()
	if err != nil {
		t.Fatalf("IndexPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale := `{"version":1,"entries":[]}`
	if err := os.WriteFile(indexPath, []byte(stale), 0o644); err != nil {
		t.Fatalf("write stale index: %v", err)
	}

	sourceDirs := []string{filepath.Join(tmp, ".claude", "skills")}
	idx, err := LoadOrRebuild(sourceDirs)
	if err != nil {
		t.Fatalf("LoadOrRebuild: %v", err)
	}
	if idx.Version != CurrentVersion {
		t.Errorf("after version mismatch rebuild: want version %d, got %d", CurrentVersion, idx.Version)
	}
	// Disk must also reflect the new version.
	disk, err := Load()
	if err != nil {
		t.Fatalf("Load after rebuild: %v", err)
	}
	if disk.Version != CurrentVersion {
		t.Errorf("on-disk version after rebuild: want %d, got %d", CurrentVersion, disk.Version)
	}
}

func TestLoadOrRebuild_CorruptIndex_Rebuilds(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	seedSkillFile(t, tmp, "x", minimalSkill)

	// Write garbage to the index path.
	indexPath, err := IndexPath()
	if err != nil {
		t.Fatalf("IndexPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("{{{garbage"), 0o644); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}

	sourceDirs := []string{filepath.Join(tmp, ".claude", "skills")}
	idx, err := LoadOrRebuild(sourceDirs)
	if err != nil {
		t.Fatalf("LoadOrRebuild must recover from corrupt index, got: %v", err)
	}
	if idx.Version != CurrentVersion {
		t.Errorf("rebuilt index version: want %d, got %d", CurrentVersion, idx.Version)
	}
}

func TestNeedsRebuild_FileCountChanged_ReturnsTrue(t *testing.T) {
	// Mutates HOME — cannot run in parallel (NeedsRebuild reads aliasOverridesPath).
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "x.md"), []byte(minimalSkill), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Cache claims 2 files but only 1 exists.
	cached := Index{
		Version:   CurrentVersion,
		FileCount: 2,
		// SourceMtime zero is fine — count mismatch fires first.
	}
	if !NeedsRebuild(cached, []string{skillsDir}) {
		t.Error("file count mismatch must trigger rebuild")
	}
}

func TestNeedsRebuild_NewerMtime_ReturnsTrue(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "x.md"), []byte(minimalSkill), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(filepath.Join(skillsDir, "x.md"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Cache SourceMtime is 1 hour before the file's actual mtime.
	cached := Index{
		Version:     CurrentVersion,
		FileCount:   1,
		SourceMtime: info.ModTime().Add(-time.Hour),
	}
	if !NeedsRebuild(cached, []string{skillsDir}) {
		t.Error("newer file mtime must trigger rebuild")
	}
}

func TestNeedsRebuild_SameState_ReturnsFalse(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "x.md"), []byte(minimalSkill), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Build the index once to get a properly-populated Index.
	idx, err := buildIndex([]string{skillsDir})
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	// Freshly built index must not need rebuilding against the same source.
	if NeedsRebuild(idx, []string{skillsDir}) {
		t.Error("freshly built index must not require rebuild (same state)")
	}
}
