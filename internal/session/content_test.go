package session

import (
	"encoding/json"
	"testing"
	"time"
)

func TestExtractContent_PlainString(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`"hello from user"`)
	text, thinking, toolName, toolInput := extractContent(raw, "")
	if text != "hello from user" {
		t.Errorf("text = %q, want 'hello from user'", text)
	}
	if thinking != "" {
		t.Errorf("thinking = %q, want empty", thinking)
	}
	if toolName != "" {
		t.Errorf("toolName = %q, want empty", toolName)
	}
	if len(toolInput) != 0 {
		t.Errorf("toolInput = %s, want empty", toolInput)
	}
}

func TestExtractContent_BlockArray_Text(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`[{"type":"text","text":"block text"}]`)
	text, thinking, toolName, toolInput := extractContent(raw, "")
	if text != "block text" {
		t.Errorf("text = %q, want 'block text'", text)
	}
	if thinking != "" || toolName != "" || len(toolInput) != 0 {
		t.Error("unexpected non-empty fields for text-only block")
	}
}

func TestExtractContent_BlockArray_Thinking(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`[{"type":"thinking","thinking":"chain of thought","signature":""},{"type":"text","text":"final answer"}]`)
	text, thinking, toolName, toolInput := extractContent(raw, "")
	if text != "final answer" {
		t.Errorf("text = %q, want 'final answer'", text)
	}
	if thinking != "chain of thought" {
		t.Errorf("thinking = %q, want 'chain of thought'", thinking)
	}
	if toolName != "" || len(toolInput) != 0 {
		t.Error("unexpected tool fields in thinking+text block")
	}
}

func TestExtractContent_BlockArray_ToolUse(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`[{"type":"tool_use","name":"Read","id":"call_1","input":{"file_path":"foo.go"}}]`)
	text, thinking, toolName, toolInput := extractContent(raw, "")
	if text != "" {
		t.Errorf("text = %q, want empty for tool_use block", text)
	}
	if toolName != "Read" {
		t.Errorf("toolName = %q, want 'Read'", toolName)
	}
	if len(toolInput) == 0 {
		t.Error("toolInput is empty, want non-empty JSON")
	}
	var inp struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(toolInput, &inp); err != nil {
		t.Fatalf("unmarshal toolInput: %v", err)
	}
	if inp.FilePath != "foo.go" {
		t.Errorf("toolInput.file_path = %q, want 'foo.go'", inp.FilePath)
	}
	_ = thinking
}

func TestExtractContent_BlockArray_ToolResult_String(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"result text"}]`)
	text, _, _, _ := extractContent(raw, "")
	if text != "result text" {
		t.Errorf("text = %q, want 'result text'", text)
	}
}

func TestExtractContent_BlockArray_ToolResult_Blocks(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"nested text"}]}]`)
	text, _, _, _ := extractContent(raw, "")
	if text != "nested text" {
		t.Errorf("text = %q, want 'nested text'", text)
	}
}

func TestExtractContent_FirstTextBlockWins(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`[{"type":"text","text":"first"},{"type":"text","text":"second"}]`)
	text, _, _, _ := extractContent(raw, "")
	if text != "first" {
		t.Errorf("text = %q, want 'first' (first text block wins)", text)
	}
}

func TestExtractContent_Empty(t *testing.T) {
	t.Parallel()

	text, thinking, toolName, toolInput := extractContent(nil, "")
	if text != "" || thinking != "" || toolName != "" || len(toolInput) != 0 {
		t.Error("expected all zero values for nil input")
	}
}

func TestExtractContent_InvalidJSON(t *testing.T) {
	t.Parallel()

	// Invalid JSON should return empty without panic.
	text, thinking, toolName, toolInput := extractContent(json.RawMessage(`{invalid}`), "")
	if text != "" || thinking != "" || toolName != "" || len(toolInput) != 0 {
		t.Error("expected all zero values for invalid JSON")
	}
}

func TestParseTimestamp_RFC3339Nano(t *testing.T) {
	t.Parallel()

	fallback := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	got := parseTimestamp("2026-04-19T08:00:00.123456789Z", fallback)
	if got.Year() != 2026 || got.Month() != 4 || got.Day() != 19 {
		t.Errorf("parseTimestamp = %v, want 2026-04-19", got)
	}
}

func TestParseTimestamp_RFC3339(t *testing.T) {
	t.Parallel()

	fallback := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	got := parseTimestamp("2026-04-19T08:00:00Z", fallback)
	if got.Year() != 2026 || got.Month() != 4 || got.Day() != 19 {
		t.Errorf("parseTimestamp = %v, want 2026-04-19", got)
	}
}

func TestParseTimestamp_Empty(t *testing.T) {
	t.Parallel()

	fallback := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	got := parseTimestamp("", fallback)
	if !got.Equal(fallback) {
		t.Errorf("parseTimestamp(\"\") = %v, want fallback %v", got, fallback)
	}
}

func TestParseTimestamp_Invalid(t *testing.T) {
	t.Parallel()

	fallback := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	got := parseTimestamp("not-a-timestamp", fallback)
	if !got.Equal(fallback) {
		t.Errorf("parseTimestamp(invalid) = %v, want fallback %v", got, fallback)
	}
}

func TestExtractToolResultText_StringContent(t *testing.T) {
	t.Parallel()

	got := extractToolResultText(json.RawMessage(`"plain text result"`))
	if got != "plain text result" {
		t.Errorf("extractToolResultText = %q, want 'plain text result'", got)
	}
}

func TestExtractToolResultText_BlockContent(t *testing.T) {
	t.Parallel()

	got := extractToolResultText(json.RawMessage(`[{"type":"text","text":"block result"}]`))
	if got != "block result" {
		t.Errorf("extractToolResultText = %q, want 'block result'", got)
	}
}

func TestExtractToolResultText_Empty(t *testing.T) {
	t.Parallel()

	got := extractToolResultText(nil)
	if got != "" {
		t.Errorf("extractToolResultText(nil) = %q, want empty", got)
	}
}
