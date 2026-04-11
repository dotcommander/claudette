package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

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

const (
	defaultThreshold = 2
	defaultLimit     = 5
)

// Run reads HookInput from stdin, scores entries, writes context to stdout.
// Silent exit (no output) when nothing matches.
func Run() error {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20)) // 1MB cap
	if err != nil {
		return err
	}

	var input HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil // Malformed input: exit silently
	}

	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" || strings.HasPrefix(prompt, "/") {
		return nil
	}

	sourceDirs, err := index.SourceDirs()
	if err != nil {
		return nil // Can't discover dirs: exit silently
	}

	idx, err := index.LoadOrRebuild(sourceDirs)
	if err != nil {
		return nil // Index failure: exit silently
	}

	stops := search.DefaultStopWords()
	tokens := search.Tokenize(prompt, stops)
	if len(tokens) == 0 {
		return nil
	}

	results := search.ScoreTop(idx.Entries, tokens, defaultThreshold, defaultLimit)
	if len(results) == 0 {
		return nil
	}

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
	b.WriteString("Relevant knowledge base entries \u2014 read before proceeding:")
	for _, r := range results {
		fmt.Fprintf(&b, "\n  %s \u2014 %s", relPath(r.Entry), r.Entry.Title)
	}
	return b.String()
}

func relPath(e index.Entry) string {
	// Show path relative to ~/.claude/ for readability
	home, err := os.UserHomeDir()
	if err != nil {
		return e.FilePath
	}
	claudeDir := home + "/.claude/"
	if strings.HasPrefix(e.FilePath, claudeDir) {
		return strings.TrimPrefix(e.FilePath, claudeDir)
	}
	return e.FilePath
}
