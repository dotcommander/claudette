package main

import (
	"fmt"
	"os"

	"github.com/dotcommander/claudette/internal/codify"
	"github.com/spf13/cobra"
)

func codifyCmd() *cobra.Command {
	var opts codify.Opts

	cmd := &cobra.Command{
		Use:   "codify <path>",
		Short: "Promote a .work/ artifact to a KB entry under ~/.claude/kb/",
		Long: "Reads a markdown file, extracts its title and description, " +
			"prompts for confirmation, and writes a KB entry with provenance " +
			"frontmatter (source_file, source, source_task). " +
			"Runs 'claudette scan' after a successful write. " +
			"Idempotent: refuses to overwrite an existing entry unless --force.",
		Example: "  claudette codify .work/repair-02-codify-bridge.md --session-id $CLAUDE_SESSION_ID",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Input = args[0]
			res, err := codify.Run(os.Stdout, os.Stdin, opts)
			if err != nil {
				return err
			}
			if res.Path != "" && !res.AlreadyExisted {
				fmt.Fprintln(os.Stdout, res.Path)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Category, "category", "", "KB category (default: inferred from heading)")
	cmd.Flags().StringVar(&opts.Slug, "slug", "", "KB slug (default: input basename without extension)")
	cmd.Flags().StringVar(&opts.SessionID, "session-id", os.Getenv("CLAUDE_SESSION_ID"), "Session ID for provenance (default: $CLAUDE_SESSION_ID)")
	cmd.Flags().StringVar(&opts.TaskID, "task-id", "", "Task ID for provenance (e.g. vybe task ID)")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Overwrite an existing KB entry")

	return cmd
}
