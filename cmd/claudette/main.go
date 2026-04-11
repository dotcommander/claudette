package main

import (
	"fmt"
	"os"
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
	// Fast path: hook mode bypasses cobra entirely for speed.
	if len(os.Args) > 1 && os.Args[1] == "hook" {
		if err := hook.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "claudette hook: %v\n", err)
			os.Exit(1)
		}
		return
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
	results := search.ScoreTop(entries, tokens, threshold, limit, idx.IDF)

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
			sourceDirs, err := index.SourceDirs()
			if err != nil {
				return fmt.Errorf("discovering sources: %w", err)
			}

			entries, maxMtime, fileCount, err := index.Scan(sourceDirs)
			if err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}

			idx := index.Index{
				Version:     index.CurrentVersion,
				BuildTime:   maxMtime,
				SourceMtime: maxMtime,
				FileCount:   fileCount,
				Entries:     entries,
				IDF:         index.ComputeIDF(entries),
			}
			if err := index.Save(idx); err != nil {
				return fmt.Errorf("saving index: %w", err)
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
