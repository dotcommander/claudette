package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/dotcommander/claudette/internal/actions"
	"github.com/dotcommander/claudette/internal/hook"
	"github.com/dotcommander/claudette/internal/search"
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
	case "post-tool-use":
		return true, hook.RunPostToolUse()
	}
	return false, nil
}

func rootCmd() *cobra.Command {
	var opts actions.SearchOpts

	root := &cobra.Command{
		Use:   "claudette",
		Short: "Knowledge and skill discovery for Claude Code",
	}

	root.PersistentFlags().StringVar(&opts.Format, "format", "text", "Output format: text or json")
	root.PersistentFlags().IntVar(&opts.Threshold, "threshold", search.DefaultThreshold, "Minimum score to include in results")
	root.PersistentFlags().IntVar(&opts.Limit, "limit", search.DefaultLimit, "Maximum number of results")

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

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Wire hooks, write config, and build the initial index (idempotent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return actions.Init(os.Stdout)
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
