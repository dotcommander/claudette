package hook

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/search"
)

// maxStdinBytes caps stdin reads to prevent unbounded memory use.
const maxStdinBytes = 1 << 20 // 1MB

// hookContextHeader and hookContextFooter wrap the additional-context block
// emitted to Claude Code. The header instructs the model how to triage the
// surfaced entries; the footer closes the XML tag.
const (
	hookContextHeader = "<related_skills_knowledge>\nScan first 10 lines of each file. Only read full files that are clearly relevant.\n"
	hookContextFooter = "\n</related_skills_knowledge>"
)

// --- Protocol types ---

// hookInput matches Claude Code's UserPromptSubmit stdin JSON.
type hookInput struct {
	Prompt string `json:"prompt"`
}

// postToolUseFailureInput matches Claude Code's PostToolUseFailure stdin JSON.
type postToolUseFailureInput struct {
	ToolName     string `json:"tool_name"`
	ToolInput    any    `json:"tool_input"`
	ToolResponse any    `json:"tool_response"`
}

type hookResponse struct {
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// --- Public entry points ---

// Run handles the UserPromptSubmit hook: reads prompt from stdin,
// scores against the index, writes matching context to stdout.
func Run() error {
	var status string
	defer logStatus("claudette", &status, time.Now())()

	data, err := readStdin()
	if err != nil {
		status = "stdin error"
		return err
	}

	var input hookInput
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

	tokens := search.Tokenize(prompt, search.DefaultStopWords())
	if len(tokens) == 0 {
		status = "skip: no searchable tokens"
		return nil
	}

	return scoreAndRespond(tokens, "UserPromptSubmit", &status)
}

// RunPostToolUseFailure handles the PostToolUseFailure hook: fires only when
// a tool invocation fails, so the failure signal is guaranteed — we tokenize
// the tool response directly and surface matching KB entries.
func RunPostToolUseFailure() error {
	var status string
	defer logStatus("claudette post-tool-use-failure", &status, time.Now())()

	data, err := readStdin()
	if err != nil {
		status = "stdin error"
		return err
	}

	var input postToolUseFailureInput
	if err := json.Unmarshal(data, &input); err != nil {
		status = "skip: malformed input"
		return nil
	}

	resultText := anyToString(input.ToolResponse)
	if resultText == "" {
		status = "skip: empty tool response"
		return nil
	}

	tokens := search.Tokenize(resultText, search.DefaultStopWords())
	if len(tokens) == 0 {
		status = "skip: no searchable tokens"
		return nil
	}

	return scoreAndRespond(tokens, "PostToolUseFailure", &status)
}

// --- Shared pipeline ---

// scoreAndRespond loads the index, scores tokens, and writes a hook response.
// Sets status for the deferred logStatus call. Returns nil with empty status
// when results are suppressed (no matches, low confidence).
func scoreAndRespond(tokens []string, event string, status *string) error {
	results, diagStatus, err := scoreTokens(tokens)
	*status = diagStatus
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return nil
	}

	logUsage(results)

	resp := hookResponse{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     event,
			AdditionalContext: formatContext(results, outputMode()),
		},
	}
	return json.NewEncoder(os.Stdout).Encode(resp)
}

// scoreTokens loads the index and scores tokens against it.
// Returns (nil, status, nil) when results should be suppressed.
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

	names := make([]string, len(hits))
	for i, r := range hits {
		names[i] = fmt.Sprintf("%s(%d)", r.Entry.Name, r.Score)
	}
	return hits, fmt.Sprintf("%v -> %s", tokens, strings.Join(names, ", ")), nil
}

// --- Helpers ---

func readStdin() ([]byte, error) {
	return io.ReadAll(io.LimitReader(os.Stdin, maxStdinBytes))
}

// anyToString converts an arbitrary JSON-decoded value to a string.
func anyToString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		b, err := json.Marshal(s)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func formatContext(results []search.ScoredEntry, mode string) string {
	var b strings.Builder
	b.WriteString(hookContextHeader)
	prefix := homePrefix()
	for _, r := range results {
		if mode == "compact" {
			fmt.Fprintf(&b, "\n  %s — %s", r.Entry.Name, cmp.Or(r.Entry.Desc, r.Entry.Title))
		} else {
			fmt.Fprintf(&b, "\n  %s — %s", trimHome(r.Entry.FilePath, prefix), r.Entry.Title)
		}
		if len(r.Matched) > 0 {
			fmt.Fprintf(&b, " [matched: %s]", strings.Join(r.Matched, ", "))
		}
	}
	b.WriteString(hookContextFooter)
	return b.String()
}

// outputMode reads CLAUDETTE_OUTPUT from the environment.
func outputMode() string {
	if os.Getenv("CLAUDETTE_OUTPUT") == "compact" {
		return "compact"
	}
	return "full"
}

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

func logStatus(prefix string, status *string, start time.Time) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "%s: %s (%dms)\n", prefix, *status, time.Since(start).Milliseconds())
	}
}

var (
	homePrefixVal  string
	homePrefixOnce sync.Once
)

// homePrefix returns ~/.claude/ as an absolute path for trimming.
// Computed once per process via sync.Once — home dir never changes at runtime.
func homePrefix() string {
	homePrefixOnce.Do(func() {
		if home, err := os.UserHomeDir(); err == nil {
			homePrefixVal = filepath.Join(home, ".claude") + string(os.PathSeparator)
		}
	})
	return homePrefixVal
}

func trimHome(path, prefix string) string {
	if prefix != "" && strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}
