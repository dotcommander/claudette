package index

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

// ---------------------------------------------------------------------------
// deduplicateDirs
// ---------------------------------------------------------------------------

func TestDeduplicateDirs_RemovesMissingPaths(t *testing.T) {
	t.Parallel()
	real := t.TempDir()
	input := []string{real, "/nonexistent/xyz/abc123"}
	got := deduplicateDirs(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 dir, got %d: %v", len(got), got)
	}
	if got[0] != real {
		t.Errorf("got %q, want %q", got[0], real)
	}
}

func TestDeduplicateDirs_PreservesOrder(t *testing.T) {
	t.Parallel()
	a := t.TempDir()
	b := t.TempDir()
	c := t.TempDir()
	got := deduplicateDirs([]string{a, b, a, c})
	want := []string{a, b, c}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestDeduplicateDirs_EmptyInput_EmptyOutput(t *testing.T) {
	t.Parallel()
	got := deduplicateDirs(nil)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// resolveManifestPaths
// ---------------------------------------------------------------------------

func TestResolveManifestPaths_SingleString(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`"commands"`)
	got := resolveManifestPaths("/base", raw)
	want := []string{"/base/commands"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveManifestPaths_Array(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`["a","b"]`)
	got := resolveManifestPaths("/base", raw)
	want := []string{"/base/a", "/base/b"}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

func TestResolveManifestPaths_StripsLeadingDotSlash(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`"./skills"`)
	got := resolveManifestPaths("/base", raw)
	want := "/base/skills"
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %v, want %q", got, want)
	}
}

func TestResolveManifestPaths_StripsTrailingSlash(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`"skills/"`)
	got := resolveManifestPaths("/base", raw)
	want := "/base/skills"
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %v, want %q", got, want)
	}
}

func TestResolveManifestPaths_Empty_Nil(t *testing.T) {
	t.Parallel()
	got := resolveManifestPaths("/base", json.RawMessage{})
	if len(got) != 0 {
		t.Errorf("expected nil/empty, got %v", got)
	}
}

func TestResolveManifestPaths_Malformed_Nil(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"bad":"object"}`)
	got := resolveManifestPaths("/base", raw)
	if len(got) != 0 {
		t.Errorf("expected nil for malformed, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// pluginSubdirs
// ---------------------------------------------------------------------------

func TestPluginSubdirs_NoManifest_FallsBackToConvention(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got := pluginSubdirs(dir)
	want := []string{
		filepath.Join(dir, "skills"),
		filepath.Join(dir, "agents"),
		filepath.Join(dir, "commands"),
	}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

func TestPluginSubdirs_ManifestString(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifestDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := `{"skills":"skills","commands":"cmds"}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := pluginSubdirs(dir)
	// Expect: skills from manifest, agents by convention, cmds from manifest.
	wantContains := []string{
		filepath.Join(dir, "skills"),
		filepath.Join(dir, "agents"),
		filepath.Join(dir, "cmds"),
	}
	gotSet := make(map[string]bool, len(got))
	for _, p := range got {
		gotSet[p] = true
	}
	for _, w := range wantContains {
		if !gotSet[w] {
			t.Errorf("expected %q in result %v", w, got)
		}
	}
}

func TestPluginSubdirs_ManifestArray(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifestDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := `{"skills":["a","b"],"commands":["c"]}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := pluginSubdirs(dir)
	wantContains := []string{
		filepath.Join(dir, "a"),
		filepath.Join(dir, "b"),
		filepath.Join(dir, "agents"),
		filepath.Join(dir, "c"),
	}
	gotSet := make(map[string]bool, len(got))
	for _, p := range got {
		gotSet[p] = true
	}
	for _, w := range wantContains {
		if !gotSet[w] {
			t.Errorf("expected %q in result %v", w, got)
		}
	}
}

func TestPluginSubdirs_MalformedManifest_FallsBack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	manifestDir := filepath.Join(dir, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte("{{{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := pluginSubdirs(dir)
	want := conventionalSubdirs(dir)
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// pluginDirs
// ---------------------------------------------------------------------------

func TestPluginDirs_InvalidJSON_ReturnsNil(t *testing.T) {
	t.Parallel()
	got := pluginDirs([]byte("not json"))
	if len(got) != 0 {
		t.Errorf("expected nil/empty for invalid JSON, got %v", got)
	}
}

func TestPluginDirs_ParsesRealStructure(t *testing.T) {
	t.Parallel()
	// Read the testdata fixture and substitute a real tmp dir for __TMPDIR__.
	fixtureData, err := os.ReadFile("testdata/installed_plugins.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	pluginBase := t.TempDir()
	// The fixture points to __TMPDIR__/plugin-a; create that directory.
	installPath := filepath.Join(pluginBase, "plugin-a")
	if err := os.MkdirAll(installPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data := bytes.ReplaceAll(fixtureData, []byte("__TMPDIR__"), []byte(pluginBase))
	got := pluginDirs(data)
	// pluginSubdirs falls back to conventional dirs; expect skills/agents/commands under installPath.
	wantContains := filepath.Join(installPath, "skills")
	found := false
	for _, p := range got {
		if p == wantContains {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q in pluginDirs result %v", wantContains, got)
	}
}

// ---------------------------------------------------------------------------
// SourceDirs integration (sequential — mutates HOME)
// ---------------------------------------------------------------------------

const minimalSkillFixture = `---
name: source-dirs-skill
description: Used by SourceDirs tests.
---

Content.
`

func TestSourceDirs_DefaultsOnly(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create ~/.claude/skills but NOT ~/.claude/kb.
	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := SourceDirs()
	if err != nil {
		t.Fatalf("SourceDirs: %v", err)
	}

	// skills must be present.
	containsSkills := false
	for _, p := range got {
		if p == skillsDir {
			containsSkills = true
		}
		// kb must NOT be present (directory doesn't exist).
		kbDir := filepath.Join(tmp, ".claude", "kb")
		if p == kbDir {
			t.Errorf("expected absent kb dir to be excluded, but got %v", got)
		}
	}
	if !containsSkills {
		t.Errorf("expected skills dir %q in output %v", skillsDir, got)
	}
}

func TestSourceDirs_UserConfigPrefersFirst(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create the custom dir that config will reference.
	customDir := filepath.Join(tmp, "custom-src")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write config.json referencing customDir.
	cfgDir := filepath.Join(tmp, ".config", "claudette")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir cfg: %v", err)
	}
	cfgContent := `{"source_dirs":["` + customDir + `"]}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Also create a default dir so deduplication has something to compare.
	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := SourceDirs()
	if err != nil {
		t.Fatalf("SourceDirs: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected non-empty result")
	}
	if got[0] != customDir {
		t.Errorf("expected custom dir first, got %v", got)
	}
}

func TestSourceDirs_PluginsIncluded(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create a plugin install dir with a skills subdir.
	pluginInstall := filepath.Join(tmp, "my-plugin")
	pluginSkills := filepath.Join(pluginInstall, "skills")
	if err := os.MkdirAll(pluginSkills, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write installed_plugins.json.
	pluginsDir := filepath.Join(tmp, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pluginsJSON := `{"plugins":{"myplugin":[{"installPath":"` + pluginInstall + `"}]}}`
	if err := os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), []byte(pluginsJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := SourceDirs()
	if err != nil {
		t.Fatalf("SourceDirs: %v", err)
	}

	// pluginSkills exists and should be in the output.
	found := false
	for _, p := range got {
		if p == pluginSkills {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected plugin skills dir %q in SourceDirs output %v", pluginSkills, got)
	}
}

func TestSourceDirs_MissingPluginsFile_Silent(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// No installed_plugins.json; just a default skills dir.
	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := SourceDirs()
	if err != nil {
		t.Fatalf("SourceDirs returned error without plugins file: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected non-empty result even without plugins file")
	}
}
