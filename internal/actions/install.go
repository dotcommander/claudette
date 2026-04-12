package actions

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dotcommander/claudette/internal/index"
)

// Init wires hooks, writes default config, and builds the initial index.
func Init(w io.Writer) error {
	binPath, err := resolvedExecutable()
	if err != nil {
		return err
	}

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

	fmt.Fprintf(w, "Index built: %d entries cached\n", len(entries))
	fmt.Fprintln(w, "Ready — hooks active on next Claude Code session.")
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
	settings, err := index.ReadClaudeSettings()
	if err != nil {
		return fmt.Errorf("reading settings: %w", err)
	}

	index.RemoveInvalidHookEvents(settings)

	hookCmd := binPath + " hook"
	postCmd := binPath + " post-tool-use"

	wired1, err := index.UpsertHookEntry(settings, "UserPromptSubmit", hookCmd, "claudette")
	if err != nil {
		return fmt.Errorf("wiring UserPromptSubmit hook: %w", err)
	}
	wired2, err := index.UpsertHookEntry(settings, "PostToolUse", postCmd, "claudette")
	if err != nil {
		return fmt.Errorf("wiring PostToolUse hook: %w", err)
	}

	if !wired1 && !wired2 {
		fmt.Fprintln(w, "Hooks already wired.")
		return nil
	}

	if err := index.WriteClaudeSettings(settings); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	if wired1 {
		fmt.Fprintf(w, "Wired UserPromptSubmit hook -> %s\n", hookCmd)
	}
	if wired2 {
		fmt.Fprintf(w, "Wired PostToolUse hook -> %s\n", postCmd)
	}
	return nil
}

func ensureConfig(w io.Writer) error {
	cfg, err := index.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if len(cfg.SourceDirs) > 0 {
		return nil
	}

	defaults, err := index.DefaultSourceDirs()
	if err != nil {
		return fmt.Errorf("resolving default dirs: %w", err)
	}
	cfg.SourceDirs = defaults
	if err := index.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	configPath, _ := index.ConfigPath()
	fmt.Fprintf(w, "Config written to %s\n", configPath)
	return nil
}
