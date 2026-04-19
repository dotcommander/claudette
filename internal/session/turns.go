package session

import (
	"encoding/json"
	"regexp"
	"strings"
)

// frustrationRe matches common correction and frustration signals in user text.
// Uppercase-only messages (all caps) are caught by the allCapsRe check below.
var frustrationRe = regexp.MustCompile(
	`(?i)\b(no not that|stop|wrong|i said|don't do that|that's wrong|` +
		`not what i asked|not what i said|what the hell|wtf|ffs|` +
		`how many times|listen to me|you're not listening)\b`,
)

// allCapsRe detects short all-uppercase messages (≥5 alpha chars, no lowercase).
var allCapsRe = regexp.MustCompile(`^[^a-z]{5,}$`)

// toolCategories maps Claude Code tool names to their file classification.
// Tools absent from the map go into ToolsUsed only.
var toolCategories = map[string]string{
	"Read":         "read",
	"Glob":         "read",
	"Grep":         "read",
	"NotebookRead": "read",
	"Edit":         "edit",
	"Write":        "edit",
	"MultiEdit":    "edit",
	"NotebookEdit": "edit",
}

// bashFileReadCmds are Bash command verbs that imply a file-read operation.
var bashFileReadCmds = map[string]bool{"cat": true, "head": true, "tail": true}

// bashFileEditCmds are Bash command verbs that imply a file-edit operation.
var bashFileEditCmds = map[string]bool{"rm": true, "mv": true}

// bashSkipCmds are Bash command prefixes that should not produce file paths.
var bashSkipCmds = map[string]bool{
	"go":  true,
	"git": true,
}

// ExtractTurns groups messages into user/assistant pairs.
// A turn starts at each non-tool-result human message and collects everything
// until the next such message (or end of session).
// AssistantSummary is the first non-empty text block from assistant messages.
// Tool calls populate FilesRead, FilesEdited, and ToolsUsed.
func ExtractTurns(messages []Message) []Turn {
	var turns []Turn
	var current *Turn

	flush := func() {
		if current != nil {
			current.FilesRead = dedup(current.FilesRead)
			current.FilesEdited = dedup(current.FilesEdited)
			current.ToolsUsed = dedup(current.ToolsUsed)
			turns = append(turns, *current)
			current = nil
		}
	}

	for i := range messages {
		msg := &messages[i]

		switch {
		case msg.Type == "user" && !msg.IsToolResult:
			// New human turn.
			flush()
			t := &Turn{
				Timestamp:   msg.Timestamp,
				UserContent: msg.Content,
				Frustrated:  isFrustrated(msg.Content),
			}
			current = t

		case msg.Type == "assistant" && current != nil:
			collectAssistant(current, msg)

		case msg.Type == "user" && msg.IsToolResult && current != nil:
			// Tool result — supplement file tracking from the structured result.
			collectToolResult(current, msg)
		}
	}
	flush()

	return turns
}

// collectAssistant merges one assistant message into the current turn.
func collectAssistant(turn *Turn, msg *Message) {
	if turn.AssistantSummary == "" && msg.Content != "" {
		turn.AssistantSummary = msg.Content
	}
	if turn.Thinking == "" && msg.Thinking != "" {
		turn.Thinking = msg.Thinking
	}
	if msg.ToolName != "" {
		classifyTool(turn, msg.ToolName, msg.ToolInput)
	}
	turn.Artifacts = append(turn.Artifacts, msg.Artifacts...)
}

// collectToolResult supplements file tracking from a tool-result user record.
// The assistant's tool_use block is the primary source; this catches anything missed.
func collectToolResult(turn *Turn, msg *Message) {
	if len(msg.ToolUseResult) == 0 {
		return
	}
	var result struct {
		FilePath string `json:"filePath"`
		File     struct {
			FilePath string `json:"filePath"`
		} `json:"file"`
	}
	if err := json.Unmarshal(msg.ToolUseResult, &result); err != nil {
		return
	}
	if p := coalesce(result.FilePath, result.File.FilePath); p != "" {
		// We don't know the original tool name here; record as read (conservative).
		turn.FilesRead = append(turn.FilesRead, p)
	}
}

// classifyTool routes a tool call to the appropriate Turn fields.
func classifyTool(turn *Turn, name string, input json.RawMessage) {
	turn.ToolsUsed = append(turn.ToolsUsed, name)

	category, known := toolCategories[name]
	if !known {
		if name == "Bash" {
			classifyBash(turn, input)
		}
		return
	}

	path := extractToolFilePath(name, input)
	switch category {
	case "read":
		if path != "" {
			turn.FilesRead = append(turn.FilesRead, path)
		}
	case "edit":
		if path != "" {
			turn.FilesEdited = append(turn.FilesEdited, path)
		}
	}
}

// extractToolFilePath pulls the relevant file path from a tool's input JSON.
// Different tools use different field names.
func extractToolFilePath(name string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var fields struct {
		FilePath     string `json:"file_path"`
		Pattern      string `json:"pattern"`
		Path         string `json:"path"`
		NotebookPath string `json:"notebook_path"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	switch name {
	case "Read", "Edit", "Write", "MultiEdit":
		return fields.FilePath
	case "Glob":
		return fields.Pattern
	case "Grep":
		return fields.Path
	case "NotebookRead", "NotebookEdit":
		return fields.NotebookPath
	}
	return ""
}

// classifyBash applies conservative heuristics to extract file paths from
// a Bash command string. Only cat/head/tail (read) and rm/mv (edit) are
// recognized; everything else goes to ToolsUsed only.
func classifyBash(turn *Turn, input json.RawMessage) {
	if len(input) == 0 {
		return
	}
	var fields struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return
	}
	cmd := strings.TrimSpace(fields.Command)
	if cmd == "" {
		return
	}

	tokens := strings.Fields(cmd)
	if len(tokens) == 0 {
		return
	}
	verb := tokens[0]
	if bashSkipCmds[verb] {
		return
	}
	if !bashFileReadCmds[verb] && !bashFileEditCmds[verb] {
		return
	}

	for _, tok := range tokens[1:] {
		if looksLikePath(tok) {
			if bashFileReadCmds[verb] {
				turn.FilesRead = append(turn.FilesRead, tok)
			} else {
				turn.FilesEdited = append(turn.FilesEdited, tok)
			}
		}
	}
}

// looksLikePath reports whether a token looks like a filesystem path.
// Conservative: must contain "/" or end in a recognized extension.
func looksLikePath(tok string) bool {
	if strings.Contains(tok, "/") {
		return true
	}
	exts := []string{".go", ".ts", ".js", ".py", ".rs", ".md", ".json", ".yaml", ".yml", ".txt", ".sh"}
	for _, ext := range exts {
		if strings.HasSuffix(tok, ext) {
			return true
		}
	}
	return false
}

// isFrustrated reports whether a message contains correction/frustration signals.
func isFrustrated(text string) bool {
	if text == "" {
		return false
	}
	if frustrationRe.MatchString(text) {
		return true
	}
	// Short all-caps messages are a frustration signal.
	words := strings.Fields(text)
	if len(words) <= 8 && allCapsRe.MatchString(strings.Join(words, "")) {
		return true
	}
	return false
}

// dedup returns a slice with duplicate strings removed, preserving order.
func dedup(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// coalesce returns the first non-empty string.
func coalesce(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
