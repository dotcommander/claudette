package actions

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/claudette/internal/config"
	"github.com/dotcommander/claudette/internal/settings"
)

// hookIdentifier marks every hook entry claudette owns in Claude Code's
// settings.json. Uninstall uses it to find exactly what to remove without
// touching hooks registered by other tools.
const hookIdentifier = "claudette"

// Install wires hooks, writes default config, and builds the initial index.
// Every external side effect is printed with its absolute path so the user
// sees exactly what changed.
func Install(w io.Writer) error {
	binPath, err := resolvedExecutable()
	if err != nil {
		return err
	}

	settingsPath, err := settings.ClaudeSettingsPath()
	if err != nil {
		return fmt.Errorf("resolving settings path: %w", err)
	}

	fmt.Fprintln(w, "Installing claudette...")
	fmt.Fprintf(w, "  settings: %s\n", settingsPath)

	if err := wireHooks(w, binPath); err != nil {
		return err
	}
	if err := ensureConfig(w); err != nil {
		return err
	}

	entries, err := RebuildIndex()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  index:    %d entries cached\n", len(entries))

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Installed. Hooks active on next Claude Code session.")
	fmt.Fprintln(w, "Reverse with: claudette uninstall")
	return nil
}

// Uninstall reverses Install: strips claudette-owned hook entries from
// settings.json and removes ~/.config/claudette/. The binary itself is left
// in place — a running process cannot reliably delete itself — and the user
// gets a printed hint with the exact rm command.
func Uninstall(w io.Writer) error {
	binPath, err := resolvedExecutable()
	if err != nil {
		return err
	}

	settingsPath, err := settings.ClaudeSettingsPath()
	if err != nil {
		return fmt.Errorf("resolving settings path: %w", err)
	}

	fmt.Fprintln(w, "Uninstalling claudette...")
	fmt.Fprintf(w, "  settings: %s\n", settingsPath)

	s, err := settings.ReadClaudeSettings()
	if err != nil {
		return fmt.Errorf("reading settings: %w", err)
	}
	removed := settings.RemoveHookEntries(s, hookIdentifier)
	if removed > 0 {
		if err := settings.WriteClaudeSettings(s); err != nil {
			return fmt.Errorf("writing settings: %w", err)
		}
		fmt.Fprintf(w, "  hooks:    removed %d entry/entries\n", removed)
	} else {
		fmt.Fprintln(w, "  hooks:    no claudette entries found")
	}

	configPath, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	configDir := filepath.Dir(configPath)
	if err := os.RemoveAll(configDir); err != nil {
		return fmt.Errorf("removing %s: %w", configDir, err)
	}
	fmt.Fprintf(w, "  config:   removed %s\n", configDir)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Uninstalled.")
	fmt.Fprintf(w, "Binary still installed at %s\n", binPath)
	fmt.Fprintf(w, "Remove with: rm %s\n", binPath)
	return nil
}

func resolvedExecutable() (string, error) {
	binPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving binary path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(binPath); err == nil {
		binPath = resolved
	}
	return binPath, nil
}

func wireHooks(w io.Writer, binPath string) error {
	s, err := settings.ReadClaudeSettings()
	if err != nil {
		return fmt.Errorf("reading settings: %w", err)
	}

	settings.RemoveInvalidHookEvents(s)

	// Migrate pre-v0.6.0 installs: PostToolUse fired on every tool call and
	// used regex to sniff for failure text, which produced false positives and
	// wasted cycles on every success. PostToolUseFailure fires only on actual
	// failures — strictly better signal.
	migrated := settings.RemoveHookEntriesForEvent(s, "PostToolUse", hookIdentifier)

	hookCmd := binPath + " hook"
	failCmd := binPath + " post-tool-use-failure"

	wired1, err := settings.UpsertHookEntry(s, "UserPromptSubmit", hookCmd, hookIdentifier)
	if err != nil {
		return fmt.Errorf("wiring UserPromptSubmit hook: %w", err)
	}
	wired2, err := settings.UpsertHookEntry(s, "PostToolUseFailure", failCmd, hookIdentifier)
	if err != nil {
		return fmt.Errorf("wiring PostToolUseFailure hook: %w", err)
	}

	if migrated == 0 && !wired1 && !wired2 {
		fmt.Fprintln(w, "  hooks:    already wired (idempotent no-op)")
		return nil
	}

	if err := settings.WriteClaudeSettings(s); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	if migrated > 0 {
		fmt.Fprintln(w, "  hooks:    - PostToolUse (migrated to PostToolUseFailure)")
	}
	if wired1 {
		fmt.Fprintf(w, "  hooks:    + UserPromptSubmit    -> %s\n", hookCmd)
	}
	if wired2 {
		fmt.Fprintf(w, "  hooks:    + PostToolUseFailure  -> %s\n", failCmd)
	}
	return nil
}

func ensureConfig(w io.Writer) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	configPath, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}

	changed := false
	if len(cfg.SourceDirs) == 0 {
		defaults, err := config.DefaultSourceDirs()
		if err != nil {
			return fmt.Errorf("resolving default dirs: %w", err)
		}
		cfg.SourceDirs = defaults
		changed = true
	}
	if strings.TrimSpace(cfg.ContextHeader) == "" {
		cfg.ContextHeader = config.DefaultContextHeader()
		changed = true
	}

	if !changed {
		fmt.Fprintf(w, "  config:   %s (existing)\n", configPath)
		return nil
	}
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Fprintf(w, "  config:   wrote %s\n", configPath)
	return nil
}
