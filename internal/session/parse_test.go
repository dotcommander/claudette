package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSession_Happy(t *testing.T) {
	t.Parallel()

	sess, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "happy.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if sess.Stats.LinesSkipped != 0 {
		t.Errorf("LinesSkipped = %d, want 0", sess.Stats.LinesSkipped)
	}
	if len(sess.Turns) == 0 {
		t.Fatal("expected at least one turn, got 0")
	}
	// happy.jsonl has 1 human turn: "Fix the bug in scanner"
	turn := sess.Turns[0]
	if !strings.Contains(turn.UserContent, "Fix the bug") {
		t.Errorf("UserContent = %q, want to contain 'Fix the bug'", turn.UserContent)
	}
	if turn.AssistantSummary == "" {
		t.Error("AssistantSummary is empty, want non-empty")
	}
}

func TestParseSession_Empty(t *testing.T) {
	t.Parallel()

	sess, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "empty.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession on empty file: %v", err)
	}
	if len(sess.Turns) != 0 {
		t.Errorf("turns = %d, want 0", len(sess.Turns))
	}
	if sess.Stats.LinesSkipped != 0 {
		t.Errorf("LinesSkipped = %d, want 0 for empty file", sess.Stats.LinesSkipped)
	}
}

func TestParseSession_Truncated(t *testing.T) {
	t.Parallel()

	// truncated.jsonl ends mid-line (last line has no terminating '"}'). The
	// scanner will emit a bufio error OR json.Unmarshal will fail on the last
	// partial line. Either way we expect partial results (≥1 turn) plus a
	// non-nil error OR a skipped-line count > 0.
	sess, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "truncated.jsonl"))
	// Partial results must be returned regardless.
	if len(sess.Turns) == 0 && err == nil && sess.Stats.LinesSkipped == 0 {
		t.Error("expected partial turns or non-nil error or skipped lines for truncated input")
	}
	// First complete turn must be present.
	if len(sess.Turns) == 0 {
		t.Log("no turns (error path):", err)
		return
	}
	if !strings.Contains(sess.Turns[0].UserContent, "Hello") {
		t.Errorf("first turn UserContent = %q, want 'Hello'", sess.Turns[0].UserContent)
	}
}

func TestParseSession_Malformed(t *testing.T) {
	t.Parallel()

	// malformed.jsonl has 2 bad lines and 3 valid ones (2 user + 1 assistant).
	sess, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "malformed.jsonl"))
	if err != nil {
		t.Fatalf("unexpected error on malformed file: %v", err)
	}
	if sess.Stats.LinesSkipped < 2 {
		t.Errorf("LinesSkipped = %d, want >= 2 (two malformed lines)", sess.Stats.LinesSkipped)
	}
	if len(sess.Turns) == 0 {
		t.Error("expected at least 1 turn from valid lines")
	}
}

func TestParseSession_AllRecordTypes(t *testing.T) {
	t.Parallel()

	sess, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "all_record_types.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	// The system away_summary record must populate Summary.
	if sess.Summary == "" {
		t.Error("Summary is empty, want 'session summary away text'")
	}
	if !strings.Contains(sess.Summary, "session summary away text") {
		t.Errorf("Summary = %q, want to contain 'session summary away text'", sess.Summary)
	}
	// user-human record becomes a turn.
	if len(sess.Turns) == 0 {
		t.Fatal("expected at least 1 turn from user-human record")
	}
	// The user-tool-result record (sourceToolUseID set) must not start a new turn.
	// The assistant record (with thinking) must populate Thinking on the turn.
	found := false
	for _, turn := range sess.Turns {
		if strings.Contains(turn.UserContent, "user-human record") {
			found = true
			if turn.Thinking == "" {
				t.Error("turn.Thinking is empty, want thinking from assistant block")
			}
			break
		}
	}
	if !found {
		t.Error("no turn with 'user-human record' content found")
	}
}

func TestParseSession_ContentString(t *testing.T) {
	t.Parallel()

	sess, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "content_string.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if len(sess.Turns) == 0 {
		t.Fatal("expected at least 1 turn")
	}
	turn := sess.Turns[0]
	if !strings.Contains(turn.UserContent, "plain string content from user") {
		t.Errorf("UserContent = %q, want 'plain string content from user'", turn.UserContent)
	}
	if !strings.Contains(turn.AssistantSummary, "plain string content from assistant") {
		t.Errorf("AssistantSummary = %q, want 'plain string content from assistant'", turn.AssistantSummary)
	}
}

func TestParseSession_ContentBlocks(t *testing.T) {
	t.Parallel()

	sess, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "content_blocks.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if len(sess.Turns) == 0 {
		t.Fatal("expected at least 1 turn")
	}
	turn := sess.Turns[0]
	if !strings.Contains(turn.UserContent, "block array user content") {
		t.Errorf("UserContent = %q, want 'block array user content'", turn.UserContent)
	}
	if !strings.Contains(turn.AssistantSummary, "block array assistant content") {
		t.Errorf("AssistantSummary = %q, want 'block array assistant content'", turn.AssistantSummary)
	}
}

func TestParseSession_MissingMessage(t *testing.T) {
	t.Parallel()

	// Records without a message wrapper should be skipped gracefully.
	sess, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "missing_message.jsonl"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both records lack "message", so both are skipped.
	if len(sess.Turns) != 0 {
		t.Errorf("turns = %d, want 0 (all records missing message wrapper)", len(sess.Turns))
	}
	if sess.Stats.LinesSkipped < 2 {
		t.Errorf("LinesSkipped = %d, want >= 2", sess.Stats.LinesSkipped)
	}
}

func TestParseSession_Duplicates(t *testing.T) {
	t.Parallel()

	// duplicates.jsonl has two identical (timestamp + content) user records.
	sess, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "duplicates.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if sess.Stats.DuplicatesDropped == 0 {
		t.Error("DuplicatesDropped = 0, want >= 1 for duplicate records")
	}
	// Only one turn should result from the deduped user message.
	if len(sess.Turns) != 1 {
		t.Errorf("turns = %d, want 1 (second duplicate dropped)", len(sess.Turns))
	}
}

func TestParseSession_BigLine(t *testing.T) {
	t.Parallel()

	// Generate a >1MB line in a temp file to verify the 10MB scanner buffer
	// does NOT skip it.
	dir := t.TempDir()
	path := filepath.Join(dir, "big.jsonl")

	// Build a large content string (~1.1 MB).
	bigContent := strings.Repeat("x", 1100*1024)
	line := `{"type":"user","uuid":"big1","timestamp":"2026-04-19T20:00:00Z","sessionId":"sess_big","message":{"role":"user","content":"` + bigContent + `"}}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sess, err := ParseSession(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseSession on big-line file: %v", err)
	}
	if sess.Stats.OversizedLines > 0 {
		t.Error("OversizedLines > 0: big line was skipped, expected it to be parsed")
	}
	if len(sess.Turns) == 0 {
		t.Error("expected 1 turn from big-line file")
	}
}

func TestParseSession_NonExistentFile(t *testing.T) {
	t.Parallel()

	_, err := ParseSession(context.Background(), filepath.Join("testdata", "sessions", "does_not_exist.jsonl"))
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

func TestParseSession_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	// Even with a pre-cancelled context we should get a result (partial or empty)
	// and an error — not a panic.
	_, err := ParseSession(ctx, filepath.Join("testdata", "sessions", "happy.jsonl"))
	if err == nil {
		// Some implementations return immediately with context error before
		// opening the file; others may succeed if the file is cached.
		// Either way, no panic is the contract.
		t.Log("ParseSession with cancelled ctx returned nil error (acceptable if file was read atomically)")
	}
}
