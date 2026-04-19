package actions

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalSkillMD is a seed skill file used by tests that need the index build
// to find at least one entry (Install calls RebuildIndex).
const minimalSkillMD = `---
name: test-skill
description: A test skill for install tests.
---

# Test Skill

Some content.
`

// setupInstallEnv creates a sandboxed HOME for install/uninstall tests.
// It also creates ~/.claude/skills with a minimal skill so index builds succeed.
func setupInstallEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("setupInstallEnv: mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(minimalSkillMD), 0o644); err != nil {
		t.Fatalf("setupInstallEnv: WriteFile: %v", err)
	}
	return tmp
}

// readSettingsJSON reads and parses HOME/.claude/settings.json, returning nil map
// if the file doesn't exist.
func readSettingsJSON(t *testing.T, home string) map[string]any {
	t.Helper()
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("readSettingsJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("readSettingsJSON unmarshal: %v", err)
	}
	return m
}

// settingsContainsClaudette returns true if any hook entry in any event in s
// has a command containing "claudette".
func settingsContainsClaudette(s map[string]any) bool {
	if s == nil {
		return false
	}
	hooks, _ := s["hooks"].(map[string]any)
	for _, v := range hooks {
		groups, _ := v.([]any)
		for _, g := range groups {
			group, _ := g.(map[string]any)
			hookList, _ := group["hooks"].([]any)
			for _, h := range hookList {
				entry, _ := h.(map[string]any)
				cmd, _ := entry["command"].(string)
				if strings.Contains(cmd, "claudette") {
					return true
				}
			}
		}
	}
	return false
}

// seedClaudetteHooks pre-wires hooks using a known path that contains
// "claudette" so tests that exercise Uninstall or idempotency can reliably
// detect the entries via the hookIdentifier substring check.
func seedClaudetteHooks(t *testing.T) {
	t.Helper()
	var buf bytes.Buffer
	if err := wireHooks(&buf, "/fake/bin/claudette"); err != nil {
		t.Fatalf("seedClaudetteHooks: %v", err)
	}
}

// hookCommandsForEvent returns all hook commands registered under the given event name.
func hookCommandsForEvent(s map[string]any, event string) []string {
	hooks, _ := s["hooks"].(map[string]any)
	groups, _ := hooks[event].([]any)
	var cmds []string
	for _, g := range groups {
		group, _ := g.(map[string]any)
		hookList, _ := group["hooks"].([]any)
		for _, h := range hookList {
			entry, _ := h.(map[string]any)
			if cmd, ok := entry["command"].(string); ok {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

// ---------------------------------------------------------------------------
// wireHooks
// ---------------------------------------------------------------------------

func TestWireHooks_FreshSettings_WritesBothEvents(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	binPath := "/fake/bin/claudette"
	var buf bytes.Buffer
	if err := wireHooks(&buf, binPath); err != nil {
		t.Fatalf("wireHooks: %v", err)
	}

	s := readSettingsJSON(t, home)
	if s == nil {
		t.Fatal("settings.json was not created")
	}

	upsCmds := hookCommandsForEvent(s, "UserPromptSubmit")
	ptfCmds := hookCommandsForEvent(s, "PostToolUseFailure")

	wantUPS := binPath + " hook"
	wantPTF := binPath + " post-tool-use-failure"

	foundUPS := false
	for _, c := range upsCmds {
		if c == wantUPS {
			foundUPS = true
		}
	}
	if !foundUPS {
		t.Errorf("UserPromptSubmit: want %q; got %v", wantUPS, upsCmds)
	}

	foundPTF := false
	for _, c := range ptfCmds {
		if c == wantPTF {
			foundPTF = true
		}
	}
	if !foundPTF {
		t.Errorf("PostToolUseFailure: want %q; got %v", wantPTF, ptfCmds)
	}

	out := buf.String()
	if !strings.Contains(out, "+ UserPromptSubmit") {
		t.Errorf("expected '+ UserPromptSubmit' in output: %q", out)
	}
	if !strings.Contains(out, "+ PostToolUseFailure") {
		t.Errorf("expected '+ PostToolUseFailure' in output: %q", out)
	}
}

func TestWireHooks_Idempotent(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupInstallEnv(t)

	binPath := "/fake/bin/claudette"
	var buf bytes.Buffer
	if err := wireHooks(&buf, binPath); err != nil {
		t.Fatalf("wireHooks first: %v", err)
	}
	buf.Reset()
	if err := wireHooks(&buf, binPath); err != nil {
		t.Fatalf("wireHooks second: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "already wired (idempotent no-op)") {
		t.Errorf("expected idempotent message on second call; got %q", out)
	}
}

func TestWireHooks_MigratesOldPostToolUse(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	// Pre-seed settings.json with a PostToolUse claudette entry.
	settingsDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldSettings := map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/old/claudette old-hook",
							"timeout": 3000,
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(oldSettings, "", "  ")
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	if err := wireHooks(&buf, "/new/claudette"); err != nil {
		t.Fatalf("wireHooks: %v", err)
	}

	s := readSettingsJSON(t, home)
	hooks, _ := s["hooks"].(map[string]any)

	// PostToolUse entry for claudette must be gone.
	oldCmds := hookCommandsForEvent(s, "PostToolUse")
	for _, c := range oldCmds {
		if strings.Contains(c, "claudette") {
			t.Errorf("PostToolUse still contains claudette entry: %v", oldCmds)
		}
	}

	// PostToolUseFailure must be wired.
	ptfCmds := hookCommandsForEvent(s, "PostToolUseFailure")
	if len(ptfCmds) == 0 {
		t.Errorf("PostToolUseFailure not wired after migration; hooks=%v", hooks)
	}

	out := buf.String()
	if !strings.Contains(out, "migrated") {
		t.Errorf("expected 'migrated' in output; got %q", out)
	}
}

func TestWireHooks_UpdatesStaleCommand(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	// Pre-seed with an old path.
	settingsDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldSettings := map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/old/path/claudette hook",
							"timeout": 3000,
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(oldSettings, "", "  ")
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	newBin := "/new/path/claudette"
	var buf bytes.Buffer
	if err := wireHooks(&buf, newBin); err != nil {
		t.Fatalf("wireHooks: %v", err)
	}

	s := readSettingsJSON(t, home)
	upsCmds := hookCommandsForEvent(s, "UserPromptSubmit")
	wantCmd := newBin + " hook"
	found := false
	for _, c := range upsCmds {
		if c == wantCmd {
			found = true
		}
		if strings.Contains(c, "/old/path") {
			t.Errorf("stale command still present: %q", c)
		}
	}
	if !found {
		t.Errorf("updated command %q not found; got %v", wantCmd, upsCmds)
	}
}

func TestWireHooks_DoesNotTouchOtherTools(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	// Pre-seed with another tool's hook under UserPromptSubmit.
	settingsDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	otherCmd := "/usr/local/bin/othertool hook"
	oldSettings := map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": otherCmd,
							"timeout": 3000,
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(oldSettings, "", "  ")
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	if err := wireHooks(&buf, "/fake/claudette"); err != nil {
		t.Fatalf("wireHooks: %v", err)
	}

	s := readSettingsJSON(t, home)
	upsCmds := hookCommandsForEvent(s, "UserPromptSubmit")
	found := false
	for _, c := range upsCmds {
		if c == otherCmd {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("other tool's hook %q was removed; all cmds: %v", otherCmd, upsCmds)
	}
}

// ---------------------------------------------------------------------------
// ensureConfig
// ---------------------------------------------------------------------------

func TestEnsureConfig_FreshInstall_WritesDefaults(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	var buf bytes.Buffer
	if err := ensureConfig(&buf); err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}

	configPath := filepath.Join(home, ".config", "claudette", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config.json not created: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	type cfgShape struct {
		SourceDirs    []string `json:"source_dirs"`
		ContextHeader string   `json:"context_header"`
	}
	var cfg cfgShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if len(cfg.SourceDirs) == 0 {
		t.Error("expected non-empty source_dirs in fresh config")
	}
	if cfg.ContextHeader == "" {
		t.Error("expected non-empty context_header in fresh config")
	}

	out := buf.String()
	if !strings.Contains(out, "wrote") {
		t.Errorf("expected 'wrote' in output; got %q", out)
	}
}

func TestEnsureConfig_ExistingConfig_NoChange(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	// Write a complete config first.
	cfgDir := filepath.Join(home, ".config", "claudette")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.json")
	existingCfg := `{"source_dirs":["/some/dir"],"context_header":"custom header"}`
	if err := os.WriteFile(cfgPath, []byte(existingCfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	origMtime := info.ModTime()

	var buf bytes.Buffer
	if err := ensureConfig(&buf); err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}

	info2, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !info2.ModTime().Equal(origMtime) {
		t.Error("config mtime changed — ensureConfig rewrote an existing complete config")
	}

	out := buf.String()
	if !strings.Contains(out, "(existing)") {
		t.Errorf("expected '(existing)' in output; got %q", out)
	}
}

func TestEnsureConfig_PartialConfig_FillsMissingOnly(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	cfgDir := filepath.Join(home, ".config", "claudette")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// source_dirs set, context_header empty.
	cfgPath := filepath.Join(cfgDir, "config.json")
	partialCfg := `{"source_dirs":["/my/custom/dir"]}`
	if err := os.WriteFile(cfgPath, []byte(partialCfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	if err := ensureConfig(&buf); err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	type cfgShape struct {
		SourceDirs    []string `json:"source_dirs"`
		ContextHeader string   `json:"context_header"`
	}
	var cfg cfgShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// source_dirs must be preserved as-is.
	if len(cfg.SourceDirs) != 1 || cfg.SourceDirs[0] != "/my/custom/dir" {
		t.Errorf("source_dirs changed; got %v", cfg.SourceDirs)
	}
	// context_header must be filled with the default.
	if cfg.ContextHeader == "" {
		t.Error("expected context_header to be filled with default")
	}
}

func TestEnsureConfig_WhitespaceOnlyHeader_ReplacedWithDefault(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	cfgDir := filepath.Join(home, ".config", "claudette")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.json")
	whitespaceCfg := `{"source_dirs":["/my/dir"],"context_header":"   \n\t"}`
	if err := os.WriteFile(cfgPath, []byte(whitespaceCfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	if err := ensureConfig(&buf); err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	type cfgShape struct {
		ContextHeader string `json:"context_header"`
	}
	var cfg cfgShape
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if strings.TrimSpace(cfg.ContextHeader) == "" {
		t.Error("expected whitespace-only header to be replaced with default")
	}
}

// ---------------------------------------------------------------------------
// Install / Uninstall
// ---------------------------------------------------------------------------

func TestInstall_SeedsSettings_Config_AndIndex(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	var buf bytes.Buffer
	if err := Install(&buf); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// settings.json must be created. Install wires hooks using os.Executable()
	// which is the test binary — check that the output confirms hook wiring
	// rather than scanning for the "claudette" identifier.
	s := readSettingsJSON(t, home)
	if s == nil {
		t.Fatal("settings.json was not created by Install")
	}
	upsCmds := hookCommandsForEvent(s, "UserPromptSubmit")
	ptfCmds := hookCommandsForEvent(s, "PostToolUseFailure")
	if len(upsCmds) == 0 || len(ptfCmds) == 0 {
		t.Errorf("expected both events wired; UserPromptSubmit=%v PostToolUseFailure=%v", upsCmds, ptfCmds)
	}

	// config.json must exist.
	configPath := filepath.Join(home, ".config", "claudette", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config.json missing after Install: %v", err)
	}

	// index.json must exist with at least 1 entry.
	indexPath := filepath.Join(home, ".config", "claudette", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("index.json missing after Install: %v", err)
	}
	type indexShape struct {
		Entries []any `json:"entries"`
	}
	var idx indexShape
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	if len(idx.Entries) == 0 {
		t.Error("expected at least 1 entry in index after Install")
	}

	out := buf.String()
	if !strings.Contains(out, "Installed. Hooks active on next Claude Code session.") {
		t.Errorf("expected install success message; got %q", out)
	}
}

func TestInstall_Idempotent(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupInstallEnv(t)

	var buf bytes.Buffer
	if err := Install(&buf); err != nil {
		t.Fatalf("Install first: %v", err)
	}
	buf.Reset()
	if err := Install(&buf); err != nil {
		t.Fatalf("Install second: %v", err)
	}
	out := buf.String()
	// Config must report existing on the second call (config file unchanged).
	if !strings.Contains(out, "(existing)") {
		t.Errorf("expected '(existing)' config message on second Install; got %q", out)
	}
	// Install must not error and must print the success footer.
	if !strings.Contains(out, "Installed. Hooks active on next Claude Code session.") {
		t.Errorf("expected install success footer on second Install; got %q", out)
	}
}

func TestUninstall_RemovesHooksAndConfig(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	// Seed hooks with a known "claudette" path so Uninstall can detect them.
	// Install itself uses os.Executable() which is the test binary (actions.test),
	// lacking the "claudette" identifier needed by RemoveHookEntries.
	seedClaudetteHooks(t)
	// Also run ensureConfig to create the config dir that Uninstall will remove.
	var setupBuf bytes.Buffer
	if err := ensureConfig(&setupBuf); err != nil {
		t.Fatalf("ensureConfig: %v", err)
	}

	var buf bytes.Buffer
	if err := Uninstall(&buf); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// ~/.config/claudette/ must be gone.
	configDir := filepath.Join(home, ".config", "claudette")
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Errorf("expected ~/.config/claudette to be removed; stat err=%v", err)
	}

	// settings.json must have no claudette entries.
	s := readSettingsJSON(t, home)
	if settingsContainsClaudette(s) {
		t.Error("claudette hook entries still present in settings.json after Uninstall")
	}

	out := buf.String()
	if !strings.Contains(out, "Uninstalled.") {
		t.Errorf("expected 'Uninstalled.' in output; got %q", out)
	}
}

func TestUninstall_PreservesOtherToolHooks(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	home := setupInstallEnv(t)

	// Seed claudette hooks with a recognizable path, then add another tool's hook.
	seedClaudetteHooks(t)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	otherCmd := "/usr/local/bin/othertool hook"
	hooks, _ := s["hooks"].(map[string]any)
	existing, _ := hooks["UserPromptSubmit"].([]any)
	hooks["UserPromptSubmit"] = append(existing, map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": otherCmd,
				"timeout": 3000,
			},
		},
	})
	out2, _ := json.MarshalIndent(s, "", "  ")
	out2 = append(out2, '\n')
	if err := os.WriteFile(settingsPath, out2, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	if err := Uninstall(&buf); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	s2 := readSettingsJSON(t, home)
	cmds := hookCommandsForEvent(s2, "UserPromptSubmit")
	found := false
	for _, c := range cmds {
		if c == otherCmd {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("other tool's hook %q was removed by Uninstall; cmds: %v", otherCmd, cmds)
	}
}

func TestUninstall_EmptyState_Idempotent(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupInstallEnv(t)

	var buf bytes.Buffer
	if err := Uninstall(&buf); err != nil {
		t.Fatalf("Uninstall on clean HOME: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "no claudette entries found") {
		t.Errorf("expected 'no claudette entries found'; got %q", out)
	}
}
