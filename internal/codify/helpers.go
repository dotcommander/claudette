package codify

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// knownCategories maps lowercase keyword substrings to KB category names.
// Order matters — first match wins, so more-specific prefixes come first.
var knownCategories = []struct {
	keyword  string
	category string
}{
	{"claude-code", "claude-code"},
	{"claude code", "claude-code"},
	{"claudette", "claude-code"},
	{"postgres", "postgres"},
	{"pgx", "postgres"},
	{"openai", "llm"},
	{"gemini", "llm"},
	{"anthropic", "llm"},
	{"fantasy", "llm"},
	{"bash", "bash"},
	{"shell", "bash"},
	{"zsh", "bash"},
	{"golang", "go"},
	{" go ", "go"},
	{"goroutine", "go"},
	{"errgroup", "go"},
	{"piglet", "piglet"},
	{"zai", "zai"},
	{"refactor", "refactoring"},
}

// resolveCategory returns the explicit category if set, otherwise infers one
// from the content heading. Falls back to "uncategorized".
func resolveCategory(explicit string, content []byte) string {
	if explicit != "" {
		return sanitizeSlug(explicit)
	}
	heading := strings.ToLower(extractTitle(content))
	for _, kc := range knownCategories {
		if strings.Contains(heading, kc.keyword) {
			return kc.category
		}
	}
	return "uncategorized"
}

// resolveKBRoot returns the effective KB root. Override is for tests only.
func resolveKBRoot(override string) string {
	if override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".claude", "kb")
	}
	return filepath.Join(home, ".claude", "kb")
}

// runScan execs `claudette scan` using the same binary currently running.
// Non-zero exit is returned as an error; callers treat this as non-fatal.
func runScan() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	cmd := exec.Command(self, "scan") //nolint:gosec
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}
