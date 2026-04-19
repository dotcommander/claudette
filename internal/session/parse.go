package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// scannerBufSize is the maximum line length the JSONL scanner will accept.
// Tool outputs can exceed 100 KB; 10 MB covers all known cases.
const scannerBufSize = 10 * 1024 * 1024

// rawRecord is the top-level shape of every JSONL line in a session transcript.
// Fields are decoded selectively; unrecognised fields are ignored.
type rawRecord struct {
	Type        string `json:"type"`
	UUID        string `json:"uuid"`
	Timestamp   string `json:"timestamp"`
	SessionID   string `json:"sessionId"`
	GitBranch   string `json:"gitBranch"`
	CWD         string `json:"cwd"`
	IsSidechain bool   `json:"isSidechain"`
	// user + assistant
	Message *rawMessage `json:"message"`
	// tool-result user record
	SourceToolUseID string          `json:"sourceToolUseID"`
	ToolUseResult   json.RawMessage `json:"toolUseResult"`
	// system subtypes
	Content string `json:"content"`
}

// rawMessage is the nested message object in user/assistant records.
type rawMessage struct {
	Role    string          `json:"role"`
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"` // string OR []contentBlock
}

// contentBlock is one element of a content array.
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"` // tool_result content (string or [{type,text}])
}

// ParseSession reads a session JSONL transcript and returns structured messages.
// path must point to a main transcript file (<uuid>/<uuid>.jsonl).
// Lines exceeding the 10 MB buffer are counted in Stats.OversizedLines and skipped.
// Malformed JSON and missing type fields are counted in Stats.LinesSkipped and skipped.
// Returns partial results plus a wrapped error when the file ends unexpectedly.
func ParseSession(ctx context.Context, path string) (Session, error) {
	meta, err := metaFromPath(path)
	if err != nil {
		return Session{}, fmt.Errorf("session meta for %q: %w", path, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return Session{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), scannerBufSize)

	var (
		msgs    []Message
		summary string
		stats   ParseStats
		seen    = make(map[string]struct{})
	)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return buildSession(meta, msgs, summary, stats), err
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec rawRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			stats.LinesSkipped++
			continue
		}
		if rec.Type == "" {
			stats.LinesSkipped++
			continue
		}

		switch rec.Type {
		case "user", "assistant":
			msg, ok := parseMessageRecord(rec, meta.ModTime)
			if !ok {
				stats.LinesSkipped++
				continue
			}
			if isDuplicate(seen, msg) {
				stats.DuplicatesDropped++
				continue
			}
			msgs = append(msgs, msg)

		case "system":
			if rec.Content != "" {
				summary = rec.Content
			}

		default:
			// attachment, permission-mode, last-prompt, file-history-snapshot, queue-operation — skip
		}
	}

	if err := scanner.Err(); err != nil {
		// Partial read — return what we have plus the error.
		return buildSession(meta, msgs, summary, stats), fmt.Errorf("scanning %q: %w", path, err)
	}

	return buildSession(meta, msgs, summary, stats), nil
}

// buildSession assembles the final Session from its parsed parts.
func buildSession(meta SessionMeta, msgs []Message, summary string, stats ParseStats) Session {
	turns := ExtractTurns(msgs)
	return Session{
		Meta:    meta,
		Turns:   turns,
		Summary: summary,
		Stats:   stats,
	}
}

// metaFromPath builds a SessionMeta from a transcript file path without enumerating
// subagents; used when the caller passes a path directly to ParseSession.
// path must be a flat transcript: <encoded>/<uuid>.jsonl. UUID is derived
// from the filename; the companion dir (<encoded>/<uuid>/) is detected as a sibling.
func metaFromPath(path string) (SessionMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return SessionMeta{}, err
	}
	uuid := uuidFromFilename(path)
	sessionsDir := parentDir(path)

	// Check for optional companion dir: sibling of the transcript file.
	companionDir := ""
	var subagents []string
	if uuid != "" {
		candidate := sessionsDir + "/" + uuid
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			companionDir = candidate
			subagents = globSubagentPaths(companionDir)
		}
	}

	return SessionMeta{
		UUID:           uuid,
		TranscriptPath: path,
		CompanionDir:   companionDir,
		SubagentPaths:  subagents,
		ModTime:        info.ModTime(),
		Size:           info.Size(),
		MessageCount:   countLines(path),
	}, nil
}

// uuidFromFilename extracts the UUID from a transcript filename:
// "/path/to/<uuid>.jsonl" → "<uuid>".
func uuidFromFilename(path string) string {
	base := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			base = path[i+1:]
			break
		}
	}
	const suffix = ".jsonl"
	if len(base) > len(suffix) && base[len(base)-len(suffix):] == suffix {
		return base[:len(base)-len(suffix)]
	}
	return base
}

// parentDir returns the directory component of a path.
func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

// isDuplicate reports whether msg is a duplicate of a previously seen message.
// Dedup key is "<timestamp>|<content[:64]>". Inserts the key on first occurrence.
func isDuplicate(seen map[string]struct{}, msg Message) bool {
	prefix := msg.Content
	if len(prefix) > 64 {
		prefix = prefix[:64]
	}
	key := msg.Timestamp.Format(time.RFC3339Nano) + "|" + prefix
	if _, ok := seen[key]; ok {
		return true
	}
	seen[key] = struct{}{}
	return false
}

// parseMessageRecord converts a rawRecord into a Message.
// Returns (msg, false) for records that cannot yield useful content.
func parseMessageRecord(rec rawRecord, fallbackTime time.Time) (Message, bool) {
	if rec.Message == nil {
		return Message{}, false
	}

	ts := parseTimestamp(rec.Timestamp, fallbackTime)
	msg := Message{
		Type:            rec.Type,
		Timestamp:       ts,
		Role:            rec.Message.Role,
		Model:           rec.Message.Model,
		SessionID:       rec.SessionID,
		GitBranch:       rec.GitBranch,
		CWD:             rec.CWD,
		IsSidechain:     rec.IsSidechain,
		IsToolResult:    rec.SourceToolUseID != "",
		SourceToolUseID: rec.SourceToolUseID,
		ToolUseResult:   rec.ToolUseResult,
	}

	// Normalize role for user records that contain tool results.
	if rec.Type == "user" && msg.Role == "" {
		msg.Role = "user"
	}

	msg.Content, msg.Thinking, msg.ToolName, msg.ToolInput = extractContent(rec.Message.Content, rec.Message.Model)

	// Step 0: extract antArtifact and antThinking tags before sanitization.
	// antArtifact tags are removed from text and stored on the message;
	// ExtractTurns aggregates them onto Turn.Artifacts.
	msg.Content, msg.Artifacts = extractArtifacts(msg.Content)

	var thinkingFromTag string
	msg.Content, thinkingFromTag = extractThinking(msg.Content)
	if msg.Thinking == "" && thinkingFromTag != "" {
		msg.Thinking = thinkingFromTag
	}

	msg.Content = sanitize(msg.Content)

	return msg, true
}
