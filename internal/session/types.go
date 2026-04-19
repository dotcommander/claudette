// Package session discovers Claude Code project directories, parses JSONL
// session transcripts, and extracts structured turn data for offline analysis.
// No network I/O — all reads are local filesystem only.
package session

import (
	"encoding/json"
	"time"
)

// Project represents a Claude Code project directory under ~/.claude/projects/.
type Project struct {
	// OriginalPath is the decoded filesystem path, e.g. "/Users/alice/go/src/claudette".
	OriginalPath string `json:"original_path"`
	// EncodedName is the directory name under ~/.claude/projects/, e.g. "-Users-alice-go-src-claudette".
	EncodedName string `json:"encoded_name"`
	// SessionsDir is the absolute path to the project's session directory.
	SessionsDir string `json:"sessions_dir"`
	// Sessions holds lightweight metadata for each discovered session UUID dir.
	Sessions []SessionMeta `json:"sessions"`
}

// SessionMeta is lightweight metadata for one session transcript file.
// No content is parsed — only filesystem stats and path pointers.
type SessionMeta struct {
	// UUID is derived from the transcript filename (<uuid>.jsonl → <uuid>).
	UUID string `json:"uuid"`
	// TranscriptPath is the absolute path to the flat JSONL file (<encoded>/<uuid>.jsonl).
	TranscriptPath string `json:"transcript_path"`
	// CompanionDir is the optional sibling directory <encoded>/<uuid>/ (empty when absent).
	CompanionDir string `json:"companion_dir,omitempty"`
	// SubagentPaths holds paths matching subagents/agent-*.jsonl inside CompanionDir (nil when CompanionDir is "").
	SubagentPaths []string `json:"subagent_paths,omitempty"`
	// ModTime is the mtime of TranscriptPath.
	ModTime time.Time `json:"mtime"`
	// Size is the byte size of TranscriptPath.
	Size int64 `json:"size"`
	// MessageCount is a quick line count (newline-delimited JSON lines).
	MessageCount int `json:"message_count"`
}

// Message is a single parsed record from a JSONL session transcript.
// Tool results are distinguished from human prompts via IsToolResult.
type Message struct {
	// Type is the JSONL record discriminator: "user", "assistant", "system", "attachment", etc.
	Type string `json:"type"`
	// Timestamp is parsed from the record's ISO8601 timestamp field.
	Timestamp time.Time `json:"timestamp"`
	// Content holds extracted plain text (from string or content-block arrays).
	Content string `json:"content"`
	// Role is normalized to "user" or "assistant".
	Role string `json:"role"`
	// ToolName is set when this message represents a tool_use content block.
	ToolName string `json:"tool_name,omitempty"`
	// ToolInput is the raw JSON input passed to the tool.
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	// IsToolResult is true when this is a user record carrying a tool result (has sourceToolUseID).
	IsToolResult bool `json:"is_tool_result,omitempty"`
	// SourceToolUseID links a tool-result user record to its originating tool_use call.
	SourceToolUseID string `json:"source_tool_use_id,omitempty"`
	// ToolUseResult holds the structured result payload from tool execution.
	ToolUseResult json.RawMessage `json:"tool_use_result,omitempty"`
	// Thinking holds chain-of-thought text (assistant messages only).
	Thinking string `json:"thinking,omitempty"`
	// Model is the LLM model identifier (assistant messages only).
	Model string `json:"model,omitempty"`
	// SessionID is the session UUID from the record.
	SessionID string `json:"session_id,omitempty"`
	// GitBranch is the active git branch when the record was written.
	GitBranch string `json:"git_branch,omitempty"`
	// CWD is the working directory when the record was written.
	CWD string `json:"cwd,omitempty"`
	// IsSidechain indicates the record originated in a subagent transcript.
	IsSidechain bool `json:"is_sidechain,omitempty"`
	// Artifacts holds code artifacts extracted from antArtifact tags in Content.
	Artifacts []Artifact `json:"artifacts,omitempty"`
}

// Turn is one user prompt paired with its assistant response.
// A turn begins at each non-tool-result human message.
type Turn struct {
	// Timestamp is the time of the user message that opened this turn.
	Timestamp time.Time `json:"ts"`
	// UserContent is the sanitized text of the user's prompt.
	UserContent string `json:"user"`
	// AssistantSummary is the first meaningful text block from the assistant response.
	AssistantSummary string `json:"assistant,omitempty"`
	// Thinking holds chain-of-thought text extracted from the assistant response.
	Thinking string `json:"thinking,omitempty"`
	// FilesRead holds file paths from Read/Glob/Grep tool calls.
	FilesRead []string `json:"files_read,omitempty"`
	// FilesEdited holds file paths from Edit/Write tool calls.
	FilesEdited []string `json:"files_edited,omitempty"`
	// ToolsUsed holds distinct tool names invoked in this turn.
	ToolsUsed []string `json:"tools,omitempty"`
	// Frustrated is true when the user message contains correction/frustration signals.
	Frustrated bool `json:"frustrated,omitempty"`
	// Artifacts holds code artifacts extracted from antArtifact tags.
	Artifacts []Artifact `json:"artifacts,omitempty"`
}

// Session is a fully parsed session JSONL file.
type Session struct {
	// Meta holds filesystem metadata for the source file.
	Meta SessionMeta `json:"meta"`
	// Turns is the ordered list of extracted conversation turns.
	Turns []Turn `json:"turns"`
	// Summary is populated from a system record's away_summary content.
	Summary string `json:"summary,omitempty"`
	// Stats holds parse-time counters (skipped lines, duplicates dropped).
	Stats ParseStats `json:"stats"`
}

// ParseStats carries counters from a single ParseSession call.
// Non-zero fields indicate incomplete or noisy input.
type ParseStats struct {
	// LinesSkipped counts JSONL lines that could not be parsed (malformed JSON or missing type).
	LinesSkipped int `json:"lines_skipped,omitempty"`
	// DuplicatesDropped counts messages removed by dedup (same timestamp + content[:64]).
	DuplicatesDropped int `json:"duplicates_dropped,omitempty"`
	// OversizedLines counts lines that exceeded the 10MB scanner buffer (logged, not errored).
	OversizedLines int `json:"oversized_lines,omitempty"`
}

// Artifact is a code artifact extracted from an antArtifact tag in assistant text.
type Artifact struct {
	// Identifier is the value of the identifier attribute.
	Identifier string `json:"identifier"`
	// Type is the MIME-like type, e.g. "application/vnd.ant.code".
	Type string `json:"type"`
	// Language is the programming language (when Type is a code type).
	Language string `json:"language,omitempty"`
	// Title is the human-readable title attribute.
	Title string `json:"title,omitempty"`
	// Content holds the artifact body, truncated to 500 chars for indexing.
	Content string `json:"content"`
}
