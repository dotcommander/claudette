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

	"github.com/dotcommander/claudette/internal/config"
	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/search"
	"github.com/dotcommander/claudette/internal/usage"
)

// maxStdinBytes caps stdin reads to prevent unbounded memory use.
const maxStdinBytes = 1 << 20 // 1MB

// Differential suppression tuning — hook path only (not search/explain).
// When the top score is below softCeiling and the gap to 2nd place is smaller
// than minScoreGap, confidence is too low to return multiple entries — suppress to 1.
const (
	minScoreGap = 2  // top must lead 2nd by this margin when score is below ceiling
	softCeiling = 10 // at or above this score, confidence is high enough — return all
)

// contextOpenTag and contextCloseTag are the XML protocol markers wrapping
// the additional-context block emitted to Claude Code. The triage
// instruction between them is user-configurable via Config.ContextHeader;
// the tags themselves are not, because downstream tooling matches on them.
const (
	contextOpenTag  = "<related_skills_knowledge>"
	contextCloseTag = "</related_skills_knowledge>"
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

// --- Hook modes ---

// hookMode parameterizes the shared runHook pipeline. Each entry point
// (UserPromptSubmit, PostToolUseFailure) is one hookMode value applied to
// the same 5-step pipeline: readStdin → decode → extract → tokenize → score.
type hookMode struct {
	event        string                      // hookEventName in the response
	statusPrefix string                      // prefix for stderr logStatus line
	emptyReason  string                      // status when extracted text is empty
	rejectSlash  bool                        // true => reject text starting with "/"
	extract      func([]byte) (string, bool) // decode stdin; ok=false means malformed
}

var (
	userPromptSubmitMode = hookMode{
		event:        "UserPromptSubmit",
		statusPrefix: "claudette",
		emptyReason:  "skip: empty prompt",
		rejectSlash:  true,
		extract: func(data []byte) (string, bool) {
			var input hookInput
			if err := json.Unmarshal(data, &input); err != nil {
				return "", false
			}
			return strings.TrimSpace(input.Prompt), true
		},
	}

	postToolFailureMode = hookMode{
		event:        "PostToolUseFailure",
		statusPrefix: "claudette post-tool-use-failure",
		emptyReason:  "skip: empty tool response",
		rejectSlash:  false,
		extract: func(data []byte) (string, bool) {
			var input postToolUseFailureInput
			if err := json.Unmarshal(data, &input); err != nil {
				return "", false
			}
			return anyToString(input.ToolResponse), true
		},
	}
)

// --- Public entry points ---

// Run handles the UserPromptSubmit hook: reads prompt from stdin,
// scores against the index, writes matching context to stdout.
func Run() error { return runHook(userPromptSubmitMode) }

// RunPostToolUseFailure handles the PostToolUseFailure hook: fires only when
// a tool invocation fails, so the failure signal is guaranteed — we tokenize
// the tool response directly and surface matching KB entries.
func RunPostToolUseFailure() error { return runHook(postToolFailureMode) }

// runHook is the production entry point bound to os.Stdin / os.Stdout.
func runHook(m hookMode) error {
	return runHookIO(os.Stdin, os.Stdout, m)
}

// runHookIO is the testable pipeline: reads input from r, writes response to w.
// Keeps the entire 5-step pipeline (stdin → decode → extract → tokenize → score)
// behind a single io.Reader/io.Writer seam so tests can exercise it without
// swapping global file descriptors.
// Returns nil on all skip paths; returns err only on stdin read failure.
func runHookIO(r io.Reader, w io.Writer, m hookMode) error {
	var hs HookStatus
	var diagStatus string
	defer logStatus(m.statusPrefix, &diagStatus, &hs, time.Now())()

	data, err := readAllLimited(r)
	if err != nil {
		diagStatus = "stdin error"
		return err
	}

	text, ok := m.extract(data)
	if !ok {
		diagStatus = "skip: malformed input"
		return nil
	}
	if text == "" {
		diagStatus = m.emptyReason
		return nil
	}
	if m.rejectSlash && strings.HasPrefix(text, "/") {
		diagStatus = "skip: slash command"
		return nil
	}

	tokens := search.Tokenize(text, search.DefaultStopWords())
	if len(tokens) == 0 {
		diagStatus = "skip: no searchable tokens"
		return nil
	}

	return scoreAndRespondTo(w, tokens, m.event, &diagStatus, &hs)
}

// --- Shared pipeline ---

// scoreResult bundles the outputs of scoreTokens to stay within the 3-return limit.
type scoreResult struct {
	hits       []search.ScoredEntry
	rebuildDur time.Duration
	status     string
	idx        index.Index
}

// scoreAndRespondTo loads the index, scores tokens, and writes a hook response to w.
// Sets diagStatus for the deferred logStatus call. Returns nil with empty diagStatus
// when results are suppressed (no matches, low confidence).
func scoreAndRespondTo(w io.Writer, tokens []string, event string, diagStatus *string, hs *HookStatus) error {
	now := time.Now()
	sr := scoreTokens(tokens)
	*diagStatus = sr.status
	hs.RebuildMs = int(sr.rebuildDur.Milliseconds())
	populateAdvisoryFields(hs, &sr.idx, now)
	if len(sr.hits) == 0 {
		return nil
	}

	logUsage(sr.hits)
	logCoOccurrence(tokens, sr.hits)

	cfg, _ := config.LoadConfig() // zero-value Config falls back to defaults
	ctx := formatContext(sr.hits, outputMode(), cfg.ContextHeaderOrDefault())
	resp := hookResponse{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     event,
			AdditionalContext: ctx,
		},
	}
	return json.NewEncoder(w).Encode(resp)
}

// scoreTokens loads the index and scores tokens against it.
// sr.hits is nil when results should be suppressed; sr.idx is always set on
// successful load so callers can populate advisory fields (e.g. IndexAgeDays).
func scoreTokens(tokens []string) scoreResult {
	sourceDirs, err := index.SourceDirs()
	if err != nil {
		return scoreResult{status: "skip: source discovery failed"}
	}

	idx, rebuildDur, err := index.LoadOrRebuild(sourceDirs)
	if err != nil {
		return scoreResult{status: "skip: index load failed"}
	}

	hits := search.ScoreTop(idx.Entries, tokens, search.DefaultThreshold, search.DefaultLimit, idx.IDF, idx.AvgFieldLen)
	if len(hits) == 0 {
		// Record the miss for cluster analysis; best-effort (failure is silent).
		_ = usage.AppendMissLog(tokens)
		return scoreResult{rebuildDur: rebuildDur, status: fmt.Sprintf("%v -> no matches", tokens), idx: idx}
	}
	if hits[0].Score < search.DefaultThreshold*search.DefaultConfidenceMultiplier {
		return scoreResult{rebuildDur: rebuildDur, status: fmt.Sprintf("%v -> suppressed (low confidence, top score %d)", tokens, hits[0].Score), idx: idx}
	}
	if len(hits[0].Matched) < 2 && hits[0].Score < search.DefaultSingleTokenFloor {
		return scoreResult{rebuildDur: rebuildDur, status: fmt.Sprintf("%v -> suppressed (single-token weak match, score %d)", tokens, hits[0].Score), idx: idx}
	}

	hits = applyDifferentialGate(hits)

	names := make([]string, len(hits))
	for i, r := range hits {
		names[i] = fmt.Sprintf("%s(%d)", r.Entry.Name, r.Score)
	}
	return scoreResult{hits: hits, rebuildDur: rebuildDur, status: fmt.Sprintf("%v -> %s", tokens, strings.Join(names, ", ")), idx: idx}
}

// applyDifferentialGate suppresses to 1 result when confidence is diffuse:
// top score is below softCeiling AND the gap to 2nd place is smaller than
// minScoreGap. Above softCeiling the margin between entries is irrelevant —
// return all. Single-result slices are a no-op.
func applyDifferentialGate(hits []search.ScoredEntry) []search.ScoredEntry {
	if len(hits) >= 2 && hits[0].Score < softCeiling && (hits[0].Score-hits[1].Score) < minScoreGap {
		return hits[:1]
	}
	return hits
}

// --- Helpers ---

// readAllLimited reads from r up to maxStdinBytes to prevent unbounded memory use.
func readAllLimited(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxStdinBytes))
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

func formatContext(results []search.ScoredEntry, mode, header string) string {
	var b strings.Builder
	b.WriteString(contextOpenTag)
	b.WriteByte('\n')
	b.WriteString(header)
	b.WriteByte('\n')
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
	b.WriteByte('\n')
	b.WriteString(contextCloseTag)
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
	records := make([]usage.UsageRecord, len(results))
	for i, r := range results {
		records[i] = usage.UsageRecord{
			Timestamp: now,
			Name:      r.Entry.Name,
			Score:     r.Score,
		}
	}
	_ = usage.AppendUsageLog(records)
}

// logCoOccurrence records which prompt tokens went unmatched alongside which
// entries were returned. Used to surface alias candidates during index rebuild.
// All errors are silently swallowed — co-occurrence recording must never block.
func logCoOccurrence(tokens []string, hits []search.ScoredEntry) {
	if len(hits) == 0 {
		return
	}

	// Build the set of tokens already matched by the hit entries.
	matchedSet := make(map[string]bool)
	for _, h := range hits {
		for kw := range h.Entry.Keywords {
			matchedSet[strings.ToLower(kw)] = true
		}
	}

	unmatched := computeUnmatched(tokens, matchedSet)
	if len(unmatched) == 0 {
		return
	}

	// Cap hit entries at 5 to keep records small.
	n := len(hits)
	if n > 5 {
		n = 5
	}
	hitEntries := make([]string, n)
	for i := range hitEntries {
		hitEntries[i] = hits[i].Entry.Name
	}

	coPath, err := usage.CoOccurrenceLogPath()
	if err != nil {
		return
	}
	_ = usage.AppendCoOccurrenceLog(coPath, usage.CoOccurrenceRecord{
		Timestamp:       time.Now(),
		UnmatchedTokens: unmatched,
		HitEntries:      hitEntries,
	})
}

// computeUnmatched returns tokens that do not appear in matchedSet, deduplicated
// and capped at MaxUnmatchedPerRecord. Lowercased to match keyword storage.
func computeUnmatched(tokens []string, matchedSet map[string]bool) []string {
	seen := make(map[string]bool, len(tokens))
	var out []string
	for _, tok := range tokens {
		lower := strings.ToLower(tok)
		if matchedSet[lower] || seen[lower] {
			continue
		}
		seen[lower] = true
		out = append(out, lower)
		if len(out) >= usage.MaxUnmatchedPerRecord {
			break
		}
	}
	return out
}

// logStatus emits the stderr diagnostic line. When all HookStatus advisory
// fields are zero, advisoryStatusSegment returns "" and the output is
// byte-identical to the pre-advisory format: "<prefix>: <status> (NNms)\n".
func logStatus(prefix string, status *string, hs *HookStatus, start time.Time) func() {
	return func() {
		seg := advisoryStatusSegment(*hs)
		fmt.Fprintf(os.Stderr, "%s: %s%s (%dms)\n", prefix, *status, seg, time.Since(start).Milliseconds())
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
