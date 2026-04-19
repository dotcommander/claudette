package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseExportDir_FullBatch(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "exports", "full_batch")
	data, err := ParseExportDir(context.Background(), dir)
	if err != nil {
		t.Fatalf("ParseExportDir full_batch: %v", err)
	}

	if len(data.Conversations) == 0 {
		t.Error("Conversations is empty, want >= 1")
	}
	if data.Conversations[0].UUID != "conv1" {
		t.Errorf("Conversations[0].UUID = %q, want 'conv1'", data.Conversations[0].UUID)
	}

	if len(data.Projects) == 0 {
		t.Error("Projects is empty, want >= 1")
	}
	if data.Projects[0].UUID != "proj1" {
		t.Errorf("Projects[0].UUID = %q, want 'proj1'", data.Projects[0].UUID)
	}

	if len(data.Memories) == 0 {
		t.Error("Memories is empty, want >= 1")
	}
	if data.Memories[0].AccountUUID != "acct1" {
		t.Errorf("Memories[0].AccountUUID = %q, want 'acct1'", data.Memories[0].AccountUUID)
	}

	if len(data.Users) == 0 {
		t.Error("Users is empty, want >= 1")
	}
	if data.Users[0].UUID != "user1" {
		t.Errorf("Users[0].UUID = %q, want 'user1'", data.Users[0].UUID)
	}
	if data.Users[0].VerifiedPhoneNumber != nil {
		t.Errorf("VerifiedPhoneNumber = %v, want nil", data.Users[0].VerifiedPhoneNumber)
	}

	if len(data.CustomStyles) == 0 {
		t.Error("CustomStyles is empty, want non-empty raw JSON")
	}
}

func TestParseExportDir_OldBatch(t *testing.T) {
	t.Parallel()

	// old_batch has only conversations.json and projects.json — three files missing.
	dir := filepath.Join("testdata", "exports", "old_batch")
	data, err := ParseExportDir(context.Background(), dir)
	if err != nil {
		t.Fatalf("ParseExportDir old_batch: unexpected error for missing optional files: %v", err)
	}

	if len(data.Conversations) == 0 {
		t.Error("Conversations is empty, want >= 1")
	}
	if len(data.Projects) == 0 {
		t.Error("Projects is empty, want >= 1")
	}

	// Optional files must be nil/empty, not an error.
	if len(data.Memories) != 0 {
		t.Errorf("Memories = %d entries, want 0 (file absent)", len(data.Memories))
	}
	if len(data.Users) != 0 {
		t.Errorf("Users = %d entries, want 0 (file absent)", len(data.Users))
	}
	if len(data.CustomStyles) != 0 {
		t.Errorf("CustomStyles non-empty, want empty (file absent)")
	}
}

func TestParseExportDir_MissingDir(t *testing.T) {
	t.Parallel()

	_, err := ParseExportDir(context.Background(), filepath.Join("testdata", "exports", "nonexistent_batch"))
	if err == nil {
		t.Error("expected error for missing export dir, got nil")
	}
}

func TestParseExportDir_MalformedJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write a malformed conversations.json.
	if err := os.WriteFile(filepath.Join(dir, "conversations.json"), []byte(`[{bad json`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Write a valid projects.json so we can confirm partial loading works.
	if err := os.WriteFile(filepath.Join(dir, "projects.json"), []byte(`[{"uuid":"p1","name":"P1"}]`), 0o644); err != nil {
		t.Fatalf("WriteFile projects.json: %v", err)
	}

	data, err := ParseExportDir(context.Background(), dir)
	if err == nil {
		t.Error("expected error wrapping filename for malformed conversations.json, got nil")
	}
	// Error should mention the file name.
	if err != nil && len(err.Error()) == 0 {
		t.Error("error message is empty")
	}
	// Projects should still be parsed (other files are independent).
	if len(data.Projects) == 0 {
		t.Error("Projects is empty, want the valid projects.json to load despite conversations error")
	}
}

func TestParseExportDir_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Must not panic; may return partial data or context error.
	_, _ = ParseExportDir(ctx, filepath.Join("testdata", "exports", "full_batch"))
}

func TestParseExportDir_ConversationMessages(t *testing.T) {
	t.Parallel()

	data, err := ParseExportDir(context.Background(), filepath.Join("testdata", "exports", "full_batch"))
	if err != nil {
		t.Fatalf("ParseExportDir: %v", err)
	}
	if len(data.Conversations) == 0 || len(data.Conversations[0].ChatMessages) == 0 {
		t.Fatal("no chat messages in first conversation")
	}
	msg := data.Conversations[0].ChatMessages[0]
	if msg.Sender != "human" {
		t.Errorf("Sender = %q, want 'human'", msg.Sender)
	}
	if len(msg.Content) == 0 {
		t.Error("Content blocks are empty")
	}
}
