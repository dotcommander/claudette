package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EncodePath encodes a filesystem path the same way Claude Code does:
// leading and interior "/" become "-", every "." becomes "-".
// e.g. "/Users/alice/go/src/my.project" → "-Users-alice-go-src-my-project".
func EncodePath(path string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(path)
}

// claudeProjectsDir returns ~/.claude/projects/.
// Declared as a var so tests can override it with t.Cleanup to restore.
var claudeProjectsDir = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// CurrentProject discovers the Project for the current working directory.
// Returns an error if no matching project directory exists.
func CurrentProject(ctx context.Context) (Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Project{}, fmt.Errorf("getwd: %w", err)
	}
	encoded := EncodePath(cwd)

	projectsRoot, err := claudeProjectsDir()
	if err != nil {
		return Project{}, err
	}
	sessionsDir := filepath.Join(projectsRoot, encoded)
	if _, err := os.Stat(sessionsDir); err != nil {
		if os.IsNotExist(err) {
			return Project{}, fmt.Errorf("no Claude Code project directory for %q", cwd)
		}
		return Project{}, fmt.Errorf("stat sessions dir: %w", err)
	}

	metas, err := enumerateSessions(ctx, sessionsDir)
	if err != nil {
		return Project{}, err
	}
	return Project{
		OriginalPath: cwd,
		EncodedName:  encoded,
		SessionsDir:  sessionsDir,
		Sessions:     metas,
	}, nil
}

// AllProjects scans ~/.claude/projects/ and returns all discovered projects.
// Each project gets its sessions enumerated (metadata only, no parsing).
// Missing or inaccessible entries are silently skipped.
func AllProjects(ctx context.Context) ([]Project, error) {
	projectsRoot, err := claudeProjectsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read projects dir: %w", err)
	}

	var projects []Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return projects, err
		}
		encoded := e.Name()
		sessionsDir := filepath.Join(projectsRoot, encoded)
		metas, err := enumerateSessions(ctx, sessionsDir)
		if err != nil {
			continue // inaccessible — skip silently
		}
		projects = append(projects, Project{
			OriginalPath: decodePath(encoded),
			EncodedName:  encoded,
			SessionsDir:  sessionsDir,
			Sessions:     metas,
		})
	}
	return projects, nil
}

// decodePath reverses EncodePath on a best-effort basis.
// Single "-" → "/". Double "--" → "/." (dot-prefixed segment, e.g. ".claude").
// Still lossy when a path segment legitimately contains a "." or "-"
// (e.g. "foo.bar" vs "foo/bar"), but handles the dominant case of dotfile
// directories (.claude, .config, .git, .cache) correctly.
func decodePath(encoded string) string {
	if encoded == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(encoded))
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == '-' {
			if i+1 < len(encoded) && encoded[i+1] == '-' {
				b.WriteString("/.")
				i++ // skip the second dash
				continue
			}
			b.WriteByte('/')
			continue
		}
		b.WriteByte(encoded[i])
	}
	return b.String()
}

// enumerateSessions walks sessionsDir and collects SessionMeta for each
// flat <uuid>.jsonl transcript file at depth 1. For each transcript, it
// checks whether a sibling directory with the same UUID name exists; if yes,
// it sets CompanionDir and globs subagents/agent-*.jsonl inside it.
// The "memory" entry (file or dir) is skipped.
func enumerateSessions(ctx context.Context, sessionsDir string) ([]SessionMeta, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir %q: %w", sessionsDir, err)
	}

	var metas []SessionMeta
	for _, e := range entries {
		if e.IsDir() {
			continue // companion dirs are resolved from the transcript side
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		uuid := strings.TrimSuffix(name, ".jsonl")
		if uuid == "memory" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return metas, err
		}
		meta, ok := buildSessionMeta(sessionsDir, uuid)
		if !ok {
			continue
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

// buildSessionMeta constructs a SessionMeta for a single session transcript file.
// uuid is the UUID string (no extension). Returns (meta, true) on success,
// (zero, false) if the transcript file is absent or unreadable.
func buildSessionMeta(sessionsDir, uuid string) (SessionMeta, bool) {
	transcriptPath := filepath.Join(sessionsDir, uuid+".jsonl")

	info, err := os.Stat(transcriptPath)
	if err != nil {
		return SessionMeta{}, false
	}

	// Check for optional sibling companion directory.
	companionDir := ""
	var subagentPaths []string
	candidateDir := filepath.Join(sessionsDir, uuid)
	if fi, err := os.Stat(candidateDir); err == nil && fi.IsDir() {
		companionDir = candidateDir
		subagentPaths = globSubagentPaths(companionDir)
	}

	return SessionMeta{
		UUID:           uuid,
		TranscriptPath: transcriptPath,
		CompanionDir:   companionDir,
		SubagentPaths:  subagentPaths,
		ModTime:        info.ModTime(),
		Size:           info.Size(),
		MessageCount:   countLines(transcriptPath),
	}, true
}

// countLines returns a fast line count for a file. Returns 0 on error.
func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close() //nolint:errcheck // read-only, close error irrelevant

	buf := make([]byte, 32*1024)
	count := 0
	for {
		n, err := f.Read(buf)
		for _, b := range buf[:n] {
			if b == '\n' {
				count++
			}
		}
		if err != nil {
			break
		}
	}
	return count
}

// globSubagentPaths returns all agent-*.jsonl paths under <sessionDir>/subagents/.
func globSubagentPaths(sessionDir string) []string {
	pattern := filepath.Join(sessionDir, "subagents", "agent-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	var paths []string
	for _, m := range matches {
		if isFile(m) {
			paths = append(paths, m)
		}
	}
	return paths
}

// isFile reports whether path refers to a regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
