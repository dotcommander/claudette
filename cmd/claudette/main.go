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

// filterTypes maps CLI filter names to index entry types.
var filterTypes = map[string]index.EntryType{
	"kb":      index.TypeKB,
	"skill":   index.TypeSkill,
	"agent":   index.TypeAgent,
	"command": index.TypeCommand,
}

func main() {
	// Fast path: bypass cobra for hook latency (<50ms target).
	if len(os.Args) > 1 {
		handled, err := fastPath(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "claudette %s: %v\n", os.Args[1], err)
			os.Exit(1)
		}
		if handled {
			return
		}
	}

	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// fastPath handles hot-path subcommands without cobra overhead.
// Returns (true, nil) on success, (true, err) on failure,
// or (false, nil) for commands that should fall through to cobra.
func fastPath(cmd string) (bool, error) {
	switch cmd {
	case "hook":
		return true, hook.Run()
	case "post-tool-use":
		return true, hook.RunPostToolUse()
	}
	return false, nil
}

func rootCmd() *cobra.Command {
	var opts searchOpts

	root := &cobra.Command{
		Use:   "claudette",
		Short: "Knowledge and skill discovery for Claude Code",
	}

	root.PersistentFlags().StringVar(&opts.format, "format", "text", "Output format: text or json")
	root.PersistentFlags().IntVar(&opts.threshold, "threshold", search.DefaultThreshold, "Minimum score to include in results")
	root.PersistentFlags().IntVar(&opts.limit, "limit", search.DefaultLimit, "Maximum number of results")

	root.AddCommand(
		newSearchCmd(&opts, ""),
		newSearchCmd(&opts, "kb"),
		newSearchCmd(&opts, "skill"),
		scanCmd(),
		initCmd(),
		versionCmd(),
	)

	return root
}

type searchOpts struct {
	format    string
	threshold int
	limit     int
}

func newSearchCmd(opts *searchOpts, filter string) *cobra.Command {
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
			return runSearch(strings.Join(args, " "), filter, opts)
		},
	}
}

func runSearch(prompt, filter string, opts *searchOpts) error {
	idx, err := loadIndex()
	if err != nil {
		return err
	}

	entries := idx.Entries
	if filter != "" {
		t, ok := filterTypes[filter]
		if !ok {
			return fmt.Errorf("unknown filter type: %q", filter)
		}
		entries = search.FilterByType(entries, t)
	}

	tokens := search.Tokenize(prompt, search.DefaultStopWords())
	results := search.ScoreTop(entries, tokens, opts.threshold, opts.limit, idx.IDF, idx.AvgFieldLen)

	switch opts.format {
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

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Wire hooks, write config, and build the initial index (idempotent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit()
		},
	}
}

func runInit() error {
	binPath, err := resolvedExecutable()
	if err != nil {
		return err
	}

	if err := wireHooks(binPath); err != nil {
		return err
	}
	if err := ensureConfig(); err != nil {
		return err
	}

	entries, err := rebuildIndex()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Index built: %d entries cached\n", len(entries))
	fmt.Fprintln(os.Stdout, "Ready — hooks active on next Claude Code session.")
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

func wireHooks(binPath string) error {
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
		fmt.Fprintln(os.Stdout, "Hooks already wired.")
		return nil
	}

	if err := index.WriteClaudeSettings(settings); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	if wired1 {
		fmt.Fprintf(os.Stdout, "Wired UserPromptSubmit hook -> %s\n", hookCmd)
	}
	if wired2 {
		fmt.Fprintf(os.Stdout, "Wired PostToolUse hook -> %s\n", postCmd)
	}
	return nil
}

func ensureConfig() error {
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
	fmt.Fprintf(os.Stdout, "Config written to %s\n", configPath)
	return nil
}

// loadIndex discovers source dirs and loads (or rebuilds) the cached index.
func loadIndex() (index.Index, error) {
	sourceDirs, err := index.SourceDirs()
	if err != nil {
		return index.Index{}, fmt.Errorf("discovering sources: %w", err)
	}
	return index.LoadOrRebuild(sourceDirs)
}

// rebuildIndex forces a full rescan and saves the index.
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
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return s.Value[:7]
			}
		}
	}
	return version
}
