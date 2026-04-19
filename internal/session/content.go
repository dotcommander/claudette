package session

import (
	"encoding/json"
	"time"
)

// extractContent parses a content field that is either a plain JSON string
// or an array of content blocks, returning (text, thinking, toolName, toolInput).
// First text block wins; first thinking block wins; first tool_use block wins.
func extractContent(raw json.RawMessage, _ string) (text, thinking, toolName string, toolInput json.RawMessage) {
	if len(raw) == 0 {
		return
	}

	// Try plain string first.
	var s string
	if json.Unmarshal(raw, &s) == nil {
		text = s
		return
	}

	// Try array of content blocks.
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return
	}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if text == "" {
				text = b.Text
			}
		case "thinking":
			if thinking == "" {
				thinking = b.Thinking
			}
		case "tool_use":
			if toolName == "" {
				toolName = b.Name
				toolInput = b.Input
			}
		case "tool_result":
			if text == "" {
				text = extractToolResultText(b.Content)
			}
		}
	}
	return
}

// extractToolResultText pulls plain text out of a tool_result content field,
// which may be a string or an array of {type, text} objects.
func extractToolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			return b.Text
		}
	}
	return ""
}

// parseTimestamp parses an ISO8601 timestamp string, returning fallback on failure.
// Tries RFC3339Nano first (sub-second precision), then RFC3339 (second precision).
func parseTimestamp(s string, fallback time.Time) time.Time {
	if s == "" {
		return fallback
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
	}
	if err != nil {
		return fallback
	}
	return t
}
