package index

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookTimeoutMs is the timeout (in milliseconds) written into Claude Code hook entries.
const hookTimeoutMs = 3000

// validHookEvents is the set of hook event names recognised by Claude Code.
var validHookEvents = map[string]struct{}{
	"PreToolUse":       {},
	"PostToolUse":      {},
	"UserPromptSubmit": {},
	"Stop":             {},
	"Notification":     {},
	"SubagentStop":     {},
}

// ClaudeSettingsPath returns the path to Claude Code's settings.json.
func ClaudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// ReadClaudeSettings reads and parses Claude Code's settings.json.
// Returns an empty map if the file does not exist.
func ReadClaudeSettings() (map[string]any, error) {
	path, err := ClaudeSettingsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]any), nil
		}
		return nil, err
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return settings, nil
}

// WriteClaudeSettings writes settings back to Claude Code's settings.json
// via atomic temp-file-then-rename.
func WriteClaudeSettings(settings map[string]any) error {
	path, err := ClaudeSettingsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeJSONFile(path, data)
}

// UpsertHookEntry ensures a hook command is registered for the given event.
// If any existing hook command contains the identifier substring, the event
// is skipped (idempotent). Returns true if a new hook was wired.
// Returns an error if the event name is not a valid Claude Code hook event.
func UpsertHookEntry(settings map[string]any, event, command string, identifier string) (bool, error) {
	if _, ok := validHookEvents[event]; !ok {
		return false, fmt.Errorf("invalid hook event %q: valid events are PreToolUse, PostToolUse, UserPromptSubmit, Stop, Notification, SubagentStop", event)
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooksMap = make(map[string]any)
		settings["hooks"] = hooksMap
	}

	groups, ok := hooksMap[event].([]any)
	if !ok {
		groups = nil
	}

	// Check if already wired.
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}
		hookList, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range hookList {
			entry, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := entry["command"].(string); strings.Contains(cmd, identifier) {
				return false, nil
			}
		}
	}

	// Append new group.
	newGroup := map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": command,
				"timeout": hookTimeoutMs,
			},
		},
	}
	hooksMap[event] = append(groups, newGroup)
	return true, nil
}

// RemoveInvalidHookEvents removes any hook event keys from settings that are
// not recognised by Claude Code. This cleans up damage from older claudette
// versions that wrote "PostToolResult" (invalid) instead of "PostToolUse".
func RemoveInvalidHookEvents(settings map[string]any) {
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return
	}
	for key := range hooksMap {
		if _, valid := validHookEvents[key]; !valid {
			delete(hooksMap, key)
		}
	}
}
