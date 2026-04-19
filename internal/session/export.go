package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ExportData holds the parsed contents of a Claude.ai data export directory.
// Fields are nil/empty when the corresponding file was absent from the export.
type ExportData struct {
	// Conversations is populated from conversations.json.
	Conversations []ExportConversation
	// Projects is populated from projects.json (Claude.ai projects, NOT Claude Code projects).
	Projects []ExportProject
	// Memories is populated from memories.json (newer exports only).
	Memories []ExportMemory
	// Users is populated from users.json (newer exports only).
	Users []ExportUser
	// CustomStyles is the raw JSON from custom_styles.json (newer exports only).
	CustomStyles json.RawMessage
}

// ExportConversation is one conversation from conversations.json.
type ExportConversation struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Account   struct {
		UUID string `json:"uuid"`
	} `json:"account"`
	ChatMessages []ExportMessage `json:"chat_messages"`
}

// ExportMessage is one message within an ExportConversation.
type ExportMessage struct {
	UUID              string               `json:"uuid"`
	Text              string               `json:"text"`
	Content           []ExportContentBlock `json:"content"`
	Sender            string               `json:"sender"` // "human" or "assistant"
	CreatedAt         string               `json:"created_at"`
	UpdatedAt         string               `json:"updated_at"`
	ParentMessageUUID string               `json:"parent_message_uuid"`
	Attachments       []ExportAttachment   `json:"attachments"`
	Files             []ExportFile         `json:"files"`
}

// ExportContentBlock is one element of a message's content array.
type ExportContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"` // tool_result: string or [{type,text}]
}

// ExportAttachment is a file attachment on an export message.
type ExportAttachment struct {
	FileName         string `json:"file_name"`
	ExtractedContent string `json:"extracted_content"`
}

// ExportFile is a file reference on an export message.
type ExportFile struct {
	FileUUID string `json:"file_uuid"`
	FileName string `json:"file_name"`
}

// ExportProject is one Claude.ai project from projects.json.
// These are NOT Claude Code projects — they are claude.ai project containers.
type ExportProject struct {
	UUID           string `json:"uuid"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	PromptTemplate string `json:"prompt_template"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ExportMemory is one entry from memories.json.
type ExportMemory struct {
	ConversationsMemory string            `json:"conversations_memory"`
	ProjectMemories     map[string]string `json:"project_memories"`
	AccountUUID         string            `json:"account_uuid"`
}

// ExportUser is one entry from users.json.
type ExportUser struct {
	UUID                string  `json:"uuid"`
	FullName            string  `json:"full_name"`
	EmailAddress        string  `json:"email_address"`
	VerifiedPhoneNumber *string `json:"verified_phone_number"`
}

// ParseExportDir reads a Claude.ai data export directory and returns its contents.
// Each file is read independently; absent files are silently skipped.
// Returns a non-nil error only when the directory itself cannot be accessed.
func ParseExportDir(ctx context.Context, dir string) (ExportData, error) {
	if _, err := os.Stat(dir); err != nil {
		return ExportData{}, fmt.Errorf("export dir %q: %w", dir, err)
	}

	var data ExportData
	var errs []error

	if err := ctx.Err(); err != nil {
		return data, err
	}

	if convs, err := parseJSONFile[[]ExportConversation](filepath.Join(dir, "conversations.json")); err == nil {
		data.Conversations = convs
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("conversations.json: %w", err))
	}

	if err := ctx.Err(); err != nil {
		return data, err
	}

	if projs, err := parseJSONFile[[]ExportProject](filepath.Join(dir, "projects.json")); err == nil {
		data.Projects = projs
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("projects.json: %w", err))
	}

	if err := ctx.Err(); err != nil {
		return data, err
	}

	if mems, err := parseJSONFile[[]ExportMemory](filepath.Join(dir, "memories.json")); err == nil {
		data.Memories = mems
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("memories.json: %w", err))
	}

	if err := ctx.Err(); err != nil {
		return data, err
	}

	if users, err := parseJSONFile[[]ExportUser](filepath.Join(dir, "users.json")); err == nil {
		data.Users = users
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("users.json: %w", err))
	}

	if err := ctx.Err(); err != nil {
		return data, err
	}

	if raw, err := os.ReadFile(filepath.Join(dir, "custom_styles.json")); err == nil {
		data.CustomStyles = raw
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("custom_styles.json: %w", err))
	}

	return data, errors.Join(errs...)
}

// parseJSONFile reads path and JSON-decodes it into T.
func parseJSONFile[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return zero, fmt.Errorf("decode %q: %w", path, err)
	}
	return v, nil
}
