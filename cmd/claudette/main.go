package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/dotcommander/claudette/internal/actions"
	"github.com/dotcommander/claudette/internal/hook"
	"github.com/spf13/cobra"
)

var version = "dev"

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
	case "post-tool-use-failure":
		return true, hook.RunPostToolUseFailure()
	case "post-tool-use":
		// Back-compat: pre-v0.6.0 installs wrote this subcommand into
		// settings.json. Existing sessions keep working until the user
		// re-runs `claudette install`, which rewrites the hook entry.
		return true, hook.RunPostToolUseFailure()
	}
	return false, nil
}

func rootCmd() *cobra.Command {
	opts := actions.NewSearchOpts()

	root := &cobra.Command{
		Use:     "claudette",
		Short:   "Knowledge and skill discovery for Claude Code",
		Version: resolveVersion(),
	}
	// Cobra auto-registers --version when Version is set; keep output terse.
	root.SetVersionTemplate("{{.Version}}\n")

	root.PersistentFlags().StringVar(&opts.Format, "format", opts.Format, "Output format: text or json")
	root.PersistentFlags().IntVar(&opts.Threshold, "threshold", opts.Threshold, "Minimum score to include in results")
	root.PersistentFlags().IntVar(&opts.Limit, "limit", opts.Limit, "Maximum number of results")

	root.AddCommand(
		newSearchCmd(&opts, ""),
		newSearchCmd(&opts, "kb"),
		newSearchCmd(&opts, "skill"),
		scanCmd(),
		installCmd(),
		uninstallCmd(),
		versionCmd(),
	)

	return root
}

func newSearchCmd(opts *actions.SearchOpts, filter string) *cobra.Command {
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
			return actions.Search(os.Stdout, actions.FormatPrompt(args), filter, *opts)
		},
	}
}

func scanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Rebuild the component index",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := actions.RebuildIndex()
			if err != nil {
				return err
			}
			actions.WriteScanSummary(os.Stdout, entries)
			return nil
		},
	}
}

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "install",
		Aliases: []string{"init"},
		Short:   "Install claudette: wire hooks into ~/.claude/settings.json, write config, build index",
		Long: "Install claudette. Modifies ~/.claude/settings.json to register " +
			"UserPromptSubmit + PostToolUseFailure hooks, writes ~/.config/claudette/config.json, " +
			"and builds the initial search index. Idempotent — safe to re-run. " +
			"Migrates pre-v0.6.0 installs off the legacy PostToolUse hook.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return actions.Install(os.Stdout)
		},
	}
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall claudette: remove hooks from ~/.claude/settings.json and delete ~/.config/claudette/",
		Long: "Uninstall claudette. Removes every hook entry claudette owns from " +
			"~/.claude/settings.json (leaves other tools' hooks intact) and deletes " +
			"~/.config/claudette/. The binary is not removed — a running process cannot " +
			"reliably delete itself — but the exact rm command is printed at the end.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return actions.Uninstall(os.Stdout)
		},
	}
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

// resolveVersion prefers (in order): ldflags-injected version, module version
// from BuildInfo (set by `go install ...@vX.Y.Z`), short VCS commit, or "dev".
// When only a commit is available we also include a "-dirty" marker if the
// build was from a modified tree.
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var commit string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				commit = s.Value[:7]
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if commit == "" {
		return version
	}
	if dirty {
		return commit + "-dirty"
	}
	return commit
}
