package main

import (
	"fmt"
	"io"
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
		// settings.json. Silent no-op — run `claudette install` to migrate
		// to the PostToolUseFailure hook (fires on failure only, not all tools).
		return true, nil
	}
	return false, nil
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "claudette",
		Short:   "Knowledge and skill discovery for Claude Code",
		Version: resolveVersion(),
	}
	// Cobra auto-registers --version when Version is set; keep output terse.
	root.SetVersionTemplate("{{.Version}}\n")

	root.AddCommand(
		newSearchCmd("", os.Stdin),
		newSearchCmd("kb", os.Stdin),
		newSearchCmd("skill", os.Stdin),
		explainCmd(os.Stdin),
		scanCmd(),
		installCmd(),
		uninstallCmd(),
		updateCmd(),
		configCmd(),
		versionCmd(),
		projectsCmd(),
		sessionsCmd(),
		turnsCmd(),
		codifyCmd(),
	)

	return root
}

func newSearchCmd(filter string, stdin io.Reader) *cobra.Command {
	use := "search"
	short := "Search all entries (KB, skills, agents, commands)"
	if filter != "" {
		use = filter
		short = fmt.Sprintf("Search %s entries only", filter)
	}

	opts := actions.NewSearchOpts()
	example := fmt.Sprintf("  claudette %s \"how do goroutines work\"", use)
	cmd := &cobra.Command{
		Use:     use + " [prompt...]",
		Short:   short,
		Example: example,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt, err := resolvePrompt(args, stdin)
			if err != nil {
				return err
			}
			return actions.Search(os.Stdout, prompt, filter, opts)
		},
	}
	cmd.Flags().StringVar(&opts.Format, "format", opts.Format, "Output format: text or json")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Output as JSON (shorthand for --format json; takes precedence)")
	cmd.Flags().IntVar(&opts.Threshold, "threshold", opts.Threshold, "Minimum score to include in results")
	cmd.Flags().IntVar(&opts.Limit, "limit", opts.Limit, "Maximum number of results")
	return cmd
}

func explainCmd(stdin io.Reader) *cobra.Command {
	opts := actions.NewExplainOpts()
	cmd := &cobra.Command{
		Use:   "explain [prompt...]",
		Short: "Show per-token, per-entry score breakdown for a prompt",
		Long: "Runs the scorer in diagnostic mode against the cached index. " +
			"Shows tokenization, stop-word filtering, per-token/per-entry score " +
			"contributions (direct, plural, stem, alias, bigram), and entries that " +
			"were suppressed by threshold or IDF gates. Intended for diagnosing " +
			"expected-missing matches in the benchmark corpus.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt, err := resolvePrompt(args, stdin)
			if err != nil {
				return err
			}
			return actions.Explain(os.Stdout, prompt, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Output as JSON")
	cmd.Flags().IntVar(&opts.Limit, "limit", opts.Limit, "Max entries in detailed breakdown (0 = all)")
	return cmd
}

// resolvePrompt returns the search prompt from args or stdin.
// When the sole argument is "-", the prompt is read from r.
// Both paths reject empty or whitespace-only input via FormatPrompt/
// ReadPromptFromReader's shared validator.
func resolvePrompt(args []string, r io.Reader) (string, error) {
	if len(args) == 1 && args[0] == "-" {
		return actions.ReadPromptFromReader(r)
	}
	return actions.FormatPrompt(args)
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

func updateCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Upgrade claudette to the latest release",
		Long: "Run 'go install github.com/dotcommander/claudette/cmd/claudette@latest' " +
			"to replace the current binary with the latest tagged release. " +
			"Use --check to query the GitHub releases API without installing.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if check {
				return actions.CheckUpdate(cmd.Context(), os.Stdout, resolveVersion())
			}
			return actions.Update(cmd.Context(), os.Stderr)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Print current vs. latest release without installing")
	return cmd
}

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect claudette's persistent configuration",
		Long: "Read-only access to ~/.config/claudette/config.json. " +
			"Use 'config show' to print the full JSON or 'config path' to " +
			"print just the filesystem path (useful in shell pipelines).",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:     "show",
			Short:   "Print the current config as pretty-printed JSON",
			Example: "  claudette config show",
			RunE: func(cmd *cobra.Command, args []string) error {
				return actions.ShowConfig(os.Stdout)
			},
		},
		&cobra.Command{
			Use:     "path",
			Short:   "Print the filesystem path to the config file",
			Example: "  cat \"$(claudette config path)\"",
			RunE: func(cmd *cobra.Command, args []string) error {
				return actions.ShowConfigPath(os.Stdout)
			},
		},
	)
	return cmd
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

func projectsCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List all known Claude Code projects, ordered by most-recent activity",
		RunE: func(cmd *cobra.Command, args []string) error {
			return actions.RunProjects(cmd.Context(), os.Stdout, actions.ProjectsOpts{
				JSON: jsonOut,
			})
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func sessionsCmd() *cobra.Command {
	var all bool
	var limit int
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List sessions for the current project (use --all for all projects)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return actions.RunSessions(cmd.Context(), os.Stdout, actions.SessionsOpts{
				All:   all,
				Limit: limit,
				JSON:  jsonOut,
			})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "List sessions across all projects")
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of sessions to show")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func turnsCmd() *cobra.Command {
	var limit int
	var full bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "turns <transcript-path>",
		Short: "Parse a session transcript JSONL and show extracted turns",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return actions.RunTurns(cmd.Context(), os.Stdout, args[0], actions.TurnsOpts{
				Limit: limit,
				Full:  full,
				JSON:  jsonOut,
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 5, "Maximum number of turns to show")
	cmd.Flags().BoolVar(&full, "full", false, "Do not truncate text fields")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
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
