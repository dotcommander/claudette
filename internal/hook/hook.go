package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/search"
)

// HookInput matches Claude Code's UserPromptSubmit stdin JSON.
type HookInput struct {
	Prompt string `json:"prompt"`
}

// HookResponse matches Claude Code's hook output protocol.
type HookResponse struct {
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// HookSpecificOutput carries context to inject into the conversation.
type HookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// Run reads HookInput from stdin, scores entries, writes context to stdout.
// Logs diagnostics to stderr for visibility during Claude Code sessions.
func Run() error {
	start := time.Now()
	var status string
	defer func() {
		fmt.Fprintf(os.Stderr, "claudette: %s (%dms)\n", status, time.Since(start).Milliseconds())
	}()

	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20)) // 1MB cap
	if err != nil {
		status = "stdin error"
		return err
	}

	var input HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		status = "skip: malformed input"
		return nil
	}

	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		status = "skip: empty prompt"
		return nil
	}
	if strings.HasPrefix(prompt, "/") {
		status = "skip: slash command"
		return nil
	}

	sourceDirs, err := index.SourceDirs()
	if err != nil {
		status = "skip: source discovery failed"
		return nil
	}

	idx, err := index.LoadOrRebuild(sourceDirs)
	if err != nil {
		status = "skip: index load failed"
		return nil
	}

	stops := search.DefaultStopWords()
	tokens := search.Tokenize(prompt, stops)
	if len(tokens) == 0 {
		status = "skip: no searchable tokens"
		return nil
	}

	results := search.ScoreTop(idx.Entries, tokens, search.DefaultThreshold, search.DefaultLimit)
	if len(results) == 0 {
		status = fmt.Sprintf("%v -> no matches", tokens)
		return nil
	}

	var names []string
	for _, r := range results {
		names = append(names, fmt.Sprintf("%s(%d)", r.Entry.Name, r.Score))
	}
	status = fmt.Sprintf("%v -> %s", tokens, strings.Join(names, ", "))

	context := formatContext(results)

	resp := HookResponse{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:     "UserPromptSubmit",
			AdditionalContext: context,
		},
	}

	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(resp)
}

func formatContext(results []search.ScoredEntry) string {
	var b strings.Builder
	b.WriteString("Relevant knowledge base entries — read before proceeding:")
	for _, r := range results {
		fmt.Fprintf(&b, "\n  %s — %s", relPath(r.Entry), r.Entry.Title)
	}
	return b.String()
}

func relPath(e index.Entry) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return e.FilePath
	}
	claudeDir := filepath.Join(home, ".claude") + string(os.PathSeparator)
	if strings.HasPrefix(e.FilePath, claudeDir) {
		return strings.TrimPrefix(e.FilePath, claudeDir)
	}
	return e.FilePath
}
