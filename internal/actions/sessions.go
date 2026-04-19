package actions

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dotcommander/claudette/internal/output"
	"github.com/dotcommander/claudette/internal/session"
)

// ProjectsOpts controls output format for the projects subcommand.
type ProjectsOpts struct {
	JSON bool
}

// SessionsOpts controls behavior of the sessions subcommand.
type SessionsOpts struct {
	All   bool // list across all projects, not just cwd
	Limit int  // max sessions to emit (<=0 means no cap)
	JSON  bool
}

// TurnsOpts controls behavior of the turns subcommand.
type TurnsOpts struct {
	Limit int  // max turns to emit (<=0 means no cap)
	Full  bool // do not truncate text fields
	JSON  bool
}

// RunProjects lists all known Claude Code projects ordered by most-recent
// session mtime descending. Empty results exit 0 (not an error).
func RunProjects(ctx context.Context, w io.Writer, opts ProjectsOpts) error {
	projects, err := session.AllProjects(ctx)
	if err != nil {
		return fmt.Errorf("listing projects: %w", err)
	}

	// Order: most recent session mtime first.
	sort.Slice(projects, func(i, j int) bool {
		return projectLatestMtime(projects[i]).After(projectLatestMtime(projects[j]))
	})

	if opts.JSON {
		return output.WriteProjectsJSON(w, projects)
	}
	output.WriteProjectsText(w, projects)
	return nil
}

// RunSessions lists sessions for the current project (default) or all projects
// (--all). Results are sorted newest-first and capped by opts.Limit.
// An empty result exits 0.
func RunSessions(ctx context.Context, w io.Writer, opts SessionsOpts) error {
	metas, err := collectSessionMetas(ctx, opts.All)
	if err != nil {
		return err
	}

	// Sort newest first.
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].ModTime.After(metas[j].ModTime)
	})

	if opts.Limit > 0 && len(metas) > opts.Limit {
		metas = metas[:opts.Limit]
	}

	if opts.JSON {
		return output.WriteSessionsJSON(w, metas)
	}
	output.WriteSessionsText(w, metas)
	return nil
}

// RunTurns parses a single JSONL transcript and emits extracted turns.
// path must be an existing .jsonl file (not a directory).
func RunTurns(ctx context.Context, w io.Writer, path string, opts TurnsOpts) error {
	if err := validateTranscriptPath(path); err != nil {
		return err
	}

	sess, err := session.ParseSession(ctx, path)
	if err != nil {
		return fmt.Errorf("parsing transcript %q: %w", path, err)
	}

	turns := sess.Turns
	if opts.Limit > 0 && len(turns) > opts.Limit {
		turns = turns[:opts.Limit]
	}

	if opts.JSON {
		return output.WriteTurnsJSON(w, turns, opts.Full)
	}
	output.WriteTurnsText(w, turns, opts.Full)
	return nil
}

// validateTranscriptPath checks that path exists, is a regular file, and ends
// in ".jsonl". Returns a descriptive error for each invalid condition.
func validateTranscriptPath(path string) error {
	if !strings.HasSuffix(path, ".jsonl") {
		return fmt.Errorf("transcript path must end in .jsonl: %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %q", path)
		}
		return fmt.Errorf("stat %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path is not a regular file: %q", path)
	}
	return nil
}

// collectSessionMetas returns SessionMeta entries for the current project or
// all projects depending on the all flag.
// A missing cwd project is not an error when all is false — returns empty slice.
func collectSessionMetas(ctx context.Context, all bool) ([]session.SessionMeta, error) {
	if all {
		projects, err := session.AllProjects(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing projects: %w", err)
		}
		var out []session.SessionMeta
		for _, p := range projects {
			out = append(out, p.Sessions...)
		}
		return out, nil
	}

	proj, err := session.CurrentProject(ctx)
	if err != nil {
		// No project dir for cwd — treat as empty, not an error.
		return nil, nil //nolint:nilerr
	}
	return proj.Sessions, nil
}

// projectLatestMtime returns the most recent session ModTime for a project,
// or the zero time when the project has no sessions.
func projectLatestMtime(p session.Project) time.Time {
	var latest time.Time
	for _, s := range p.Sessions {
		if s.ModTime.After(latest) {
			latest = s.ModTime
		}
	}
	return latest
}
