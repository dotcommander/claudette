package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/dotcommander/claudette/internal/hook"
	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/output"
	"github.com/dotcommander/claudette/internal/search"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	// Fast path: bypass cobra entirely for hot code paths.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hook":
			if err := hook.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "claudette hook: %v\n", err)
				os.Exit(1)
			}
			return
		case "post-tool-use":
			if err := hook.RunPostToolUse(); err != nil {
				fmt.Fprintf(os.Stderr, "claudette post-tool-use: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var (
		format    string
		threshold int
		limit     int
	)

	root := &cobra.Command{
		Use:   "claudette",
		Short: "Knowledge and skill discovery for Claude Code",
		Long:  "Lightweight CLI that surfaces relevant knowledge base entries, skills, agents, and commands for Claude Code.",
	}

	root.PersistentFlags().StringVar(&format, "format", "text", "Output format: text or json")
	root.PersistentFlags().IntVar(&threshold, "threshold", search.DefaultThreshold, "Minimum score to include in results")
	root.PersistentFlags().IntVar(&limit, "limit", search.DefaultLimit, "Maximum number of results")

	root.AddCommand(
		searchCmd(&format, &threshold, &limit, ""),
		searchCmd(&format, &threshold, &limit, "kb"),
		searchCmd(&format, &threshold, &limit, "skill"),
		scanCmd(),
		initCmd(),
		versionCmd(),
	)

	return root
}

func searchCmd(format *string, threshold, limit *int, filter string) *cobra.Command {
	use := "search"
	short := "Search all entries (KB, skills, agents, commands)"
	if filter != "" {
		use = filter
		short = fmt.Sprintf("Search %s entries only", filter)
	}

	return &cobra.Command{
		Use:   use + " [prompt...]",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := strings.Join(args, " ")
			return runSearch(prompt, *format, *threshold, *limit, filter)
		},
	}
}

func runSearch(prompt, format string, threshold, limit int, filter string) error {
	sourceDirs, err := index.SourceDirs()
	if err != nil {
		return fmt.Errorf("discovering sources: %w", err)
	}

	idx, err := index.LoadOrRebuild(sourceDirs)
	if err != nil {
		return fmt.Errorf("loading index: %w", err)
	}

	entries := idx.Entries
	if filter != "" {
		var t index.EntryType
		switch filter {
		case "kb":
			t = index.TypeKB
		case "skill":
			t = index.TypeSkill
		case "agent":
			t = index.TypeAgent
		case "command":
			t = index.TypeCommand
		default:
			return fmt.Errorf("unknown filter type: %q", filter)
		}
		entries = search.FilterByType(entries, t)
	}

	stops := search.DefaultStopWords()
	tokens := search.Tokenize(prompt, stops)
	results := search.ScoreTop(entries, tokens, threshold, limit, idx.IDF, idx.AvgFieldLen)

	switch format {
	case "json":
		return output.WriteJSON(os.Stdout, results)
	default:
		output.WriteText(os.Stdout, results)
		return nil
	}
}

func scanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Rebuild the component index",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := rebuildIndex()
			if err != nil {
				return err
			}

			counts := make(map[string]int)
			for _, e := range entries {
				counts[string(e.Type)]++
			}
			output.WriteScanSummary(os.Stdout, counts, len(entries))
			return nil
		},
	}
}

// rebuildIndex discovers sources, scans, and saves the index.
func rebuildIndex() ([]index.Entry, error) {
	sourceDirs, err := index.SourceDirs()
	if err != nil {
		return nil, fmt.Errorf("discovering sources: %w", err)
	}
	idx, err := index.ForceRebuild(sourceDirs)
	if err != nil {
		return nil, err
	}
	return idx.Entries, nil
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Wire hooks into Claude Code and build the initial index",
		Long:  "Registers claudette as a UserPromptSubmit and PostToolUse hook in ~/.claude/settings.json, writes default config, and runs the first scan. Safe to re-run — idempotent.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit()
		},
	}
}

func runInit() error {
	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving binary path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(binPath); err == nil {
		binPath = resolved
	}

	// Wire hooks into Claude Code settings.
	settings, err := index.ReadClaudeSettings()
	if err != nil {
		return fmt.Errorf("reading settings: %w", err)
	}

	hookCmd := binPath + " hook"
	postCmd := binPath + " post-tool-use"

	// Clean up invalid hook event from older claudette versions.
	index.RemoveInvalidHookEvents(settings)

	wired1, err := index.UpsertHookEntry(settings, "UserPromptSubmit", hookCmd, "claudette")
	if err != nil {
		return fmt.Errorf("wiring UserPromptSubmit hook: %w", err)
	}
	wired2, err := index.UpsertHookEntry(settings, "PostToolUse", postCmd, "claudette")
	if err != nil {
		return fmt.Errorf("wiring PostToolUse hook: %w", err)
	}

	if wired1 || wired2 {
		if err := index.WriteClaudeSettings(settings); err != nil {
			return fmt.Errorf("writing settings: %w", err)
		}
		if wired1 {
			fmt.Fprintf(os.Stdout, "Wired UserPromptSubmit hook -> %s\n", hookCmd)
		}
		if wired2 {
			fmt.Fprintf(os.Stdout, "Wired PostToolUse hook -> %s\n", postCmd)
		}
	} else {
		fmt.Fprintln(os.Stdout, "Hooks already wired.")
	}

	// Write default config if missing.
	cfg, err := index.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if len(cfg.SourceDirs) == 0 {
		defaults, err := index.DefaultSourceDirs()
		if err != nil {
			return fmt.Errorf("resolving default dirs: %w", err)
		}
		cfg.SourceDirs = defaults
		if err := index.SaveConfig(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		configPath, _ := index.ConfigPath()
		fmt.Fprintf(os.Stdout, "Config written to %s\n", configPath)
	}

	// Run initial scan.
	entries, err := rebuildIndex()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Index built: %d entries cached\n", len(entries))
	fmt.Fprintln(os.Stdout, "Ready — hooks active on next Claude Code session.")
	return nil
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(resolveVersion())
		},
	}
}

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return s.Value[:7]
			}
		}
	}
	return version
}
