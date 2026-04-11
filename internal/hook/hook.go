package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/search"
)

// errorSignalRe matches lines containing common error signals in tool output.
// Compiled once at package level to avoid per-call overhead.
var errorSignalRe = regexp.MustCompile(`(?i)\b(error|fail|panic|undefined|cannot|not found|fatal|FAIL)\b`)

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

// logStatus emits a diagnostic line to stderr on return from the calling hook.
// Usage: defer logStatus("claudette", &status, time.Now())()
func logStatus(prefix string, status *string, start time.Time) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "%s: %s (%dms)\n", prefix, *status, time.Since(start).Milliseconds())
	}
}

// Run reads HookInput from stdin, scores entries, writes context to stdout.
// Logs diagnostics to stderr for visibility during Claude Code sessions.
func Run() error {
	var status string
	defer logStatus("claudette", &status, time.Now())()

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

	stops := search.DefaultStopWords()
	tokens := search.Tokenize(prompt, stops)
	if len(tokens) == 0 {
		status = "skip: no searchable tokens"
		return nil
	}

	results, status, err := scoreTokens(tokens)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return nil
	}

	logUsage(results)

	resp := HookResponse{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:     "UserPromptSubmit",
			AdditionalContext: formatContext(results, outputMode()),
		},
	}

	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(resp)
}

// scoreTokens loads (or rebuilds) the index and scores tokens against it.
// Returns the results, a diagnostic status string, and any hard error.
// When the result slice is empty the caller should return early; status explains why.
func scoreTokens(tokens []string) ([]search.ScoredEntry, string, error) {
	sourceDirs, err := index.SourceDirs()
	if err != nil {
		return nil, "skip: source discovery failed", nil
	}

	idx, err := index.LoadOrRebuild(sourceDirs)
	if err != nil {
		return nil, "skip: index load failed", nil
	}

	hits := search.ScoreTop(idx.Entries, tokens, search.DefaultThreshold, search.DefaultLimit, idx.IDF, idx.AvgFieldLen)
	if len(hits) == 0 {
		return nil, fmt.Sprintf("%v -> no matches", tokens), nil
	}
	if hits[0].Score < search.DefaultThreshold*search.DefaultConfidenceMultiplier {
		return nil, fmt.Sprintf("%v -> suppressed (low confidence, top score %d)", tokens, hits[0].Score), nil
	}
	if len(hits[0].Matched) < 2 && hits[0].Score < search.DefaultSingleTokenFloor {
		return nil, fmt.Sprintf("%v -> suppressed (single-token weak match, score %d)", tokens, hits[0].Score), nil
	}

	var names []string
	for _, r := range hits {
		names = append(names, fmt.Sprintf("%s(%d)", r.Entry.Name, r.Score))
	}
	return hits, fmt.Sprintf("%v -> %s", tokens, strings.Join(names, ", ")), nil
}

func formatContext(results []search.ScoredEntry, mode string) string {
	var b strings.Builder
	if mode == "compact" {
		b.WriteString("Relevant entries (read with Read tool if needed):")
		for _, r := range results {
			desc := r.Entry.Desc
			if desc == "" {
				desc = r.Entry.Title
			}
			fmt.Fprintf(&b, "\n  %s — %s", r.Entry.Name, desc)
		}
	} else {
		b.WriteString("Relevant knowledge base entries — read before proceeding:")
		for _, r := range results {
			fmt.Fprintf(&b, "\n  %s — %s", relPath(r.Entry), r.Entry.Title)
		}
	}
	return b.String()
}

// outputMode reads CLAUDETTE_OUTPUT from the environment.
func outputMode() string {
	if os.Getenv("CLAUDETTE_OUTPUT") == "compact" {
		return "compact"
	}
	return "full"
}

// logUsage records surfaced entries to the append-only usage log.
func logUsage(results []search.ScoredEntry) {
	now := time.Now()
	records := make([]index.UsageRecord, len(results))
	for i, r := range results {
		records[i] = index.UsageRecord{
			Timestamp: now,
			Name:      r.Entry.Name,
			Score:     r.Score,
		}
	}
	_ = index.AppendUsageLog(records)
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

// PostToolResultInput matches Claude Code's PostToolResult hook stdin JSON.
type PostToolResultInput struct {
	ToolName   string `json:"tool_name"`
	ToolInput  any    `json:"tool_input"`
	ToolResult any    `json:"tool_result"`
}

// RunPostToolResult reads PostToolResultInput from stdin, checks for error signals,
// and surfaces relevant KB entries when the tool result indicates a failure.
// Successful tool results produce no output, preserving hook performance.
func RunPostToolResult() error {
	var status string
	defer logStatus("claudette post-tool-result", &status, time.Now())()

	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20)) // 1MB cap
	if err != nil {
		status = "stdin error"
		return err
	}

	var input PostToolResultInput
	if err := json.Unmarshal(data, &input); err != nil {
		status = "skip: malformed input"
		return nil
	}

	// Convert ToolResult to a searchable string regardless of its JSON type.
	var resultText string
	switch v := input.ToolResult.(type) {
	case string:
		resultText = v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			status = "skip: unserializable tool result"
			return nil
		}
		resultText = string(b)
	}

	// Only surface on error signals — successful results produce no output.
	if !errorSignalRe.MatchString(resultText) {
		status = "skip: no error signal"
		return nil
	}

	// Extract lines that contain error signals for focused token extraction.
	var errorLines []string
	for _, line := range strings.Split(resultText, "\n") {
		if errorSignalRe.MatchString(line) {
			errorLines = append(errorLines, line)
		}
	}
	errorText := strings.Join(errorLines, " ")

	stops := search.DefaultStopWords()
	tokens := search.Tokenize(errorText, stops)
	if len(tokens) == 0 {
		status = "skip: no searchable tokens in error"
		return nil
	}

	results, status, err := scoreTokens(tokens)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return nil
	}

	logUsage(results)

	resp := HookResponse{
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:     "PostToolResult",
			AdditionalContext: formatContext(results, outputMode()),
		},
	}

	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(resp)
}
