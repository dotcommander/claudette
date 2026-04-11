package hook

import (
	"cmp"
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

// maxStdinBytes caps stdin reads to prevent unbounded memory use.
const maxStdinBytes = 1 << 20 // 1MB

// errorSignalRe matches lines containing common error signals in tool output.
var errorSignalRe = regexp.MustCompile(`(?i)\b(error|fail|panic|undefined|cannot|not found|fatal|FAIL)\b`)

// --- Protocol types ---

// hookInput matches Claude Code's UserPromptSubmit stdin JSON.
type hookInput struct {
	Prompt string `json:"prompt"`
}

// postToolUseInput matches Claude Code's PostToolUse stdin JSON.
type postToolUseInput struct {
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

// RunPostToolUse handles the PostToolUse hook: reads tool output from stdin,
// checks for error signals, and surfaces relevant KB entries on failures.
// Successful tool outputs produce no output.
func RunPostToolUse() error {
	var status string
	defer logStatus("claudette post-tool-use", &status, time.Now())()

	data, err := readStdin()
	if err != nil {
		status = "stdin error"
		return err
	}

	var input postToolUseInput
	if err := json.Unmarshal(data, &input); err != nil {
		status = "skip: malformed input"
		return nil
	}

	resultText := anyToString(input.ToolResponse)
	if resultText == "" {
		status = "skip: empty tool response"
		return nil
	}

	if !errorSignalRe.MatchString(resultText) {
		status = "skip: no error signal"
		return nil
	}

	tokens := extractErrorTokens(resultText)
	if len(tokens) == 0 {
		status = "skip: no searchable tokens in error"
		return nil
	}

	return scoreAndRespond(tokens, "PostToolUse", &status)
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

// extractErrorTokens filters lines containing error signals and tokenizes them.
func extractErrorTokens(text string) []string {
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if errorSignalRe.MatchString(line) {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(line)
		}
	}
	return search.Tokenize(b.String(), search.DefaultStopWords())
}

func formatContext(results []search.ScoredEntry, mode string) string {
	var b strings.Builder
	b.WriteString("<related_skills_knowledge>\nScan first 10 lines of each file. Only read full files that are clearly relevant.\n")
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
	b.WriteString("\n</related_skills_knowledge>")
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

// homePrefix returns ~/.claude/ as an absolute path for trimming.
// Computed once per formatContext call rather than per entry.
func homePrefix() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude") + string(os.PathSeparator)
}

func trimHome(path, prefix string) string {
	if prefix != "" && strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}
