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
// Source: /doctor output + https://code.claude.com/docs/en/hooks
var validHookEvents = map[string]struct{}{
	"PreToolUse":         {},
	"PostToolUse":        {},
	"PostToolUseFailure": {},
	"UserPromptSubmit":   {},
	"Stop":               {},
	"SessionStart":       {},
	"SessionEnd":         {},
	"PreCompact":         {},
	"PostCompact":        {},
	"PermissionRequest":  {},
	"PermissionDenied":   {},
	"Notification":       {},
	"SubagentStop":       {},
	"Setup":              {},
	"TeammateIdle":       {},
	"TaskCreated":        {},
	"TaskCompleted":      {},
	"Elicitation":        {},
	"ElicitationResult":  {},
	"ConfigChange":       {},
	"ExtensionsLoaded":   {},
	"CwdChanged":         {},
	"FileChanged":        {},
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
// If an existing hook command contains the identifier substring and already
// matches the desired command, no change is made (idempotent). If it contains
// the identifier but the command differs, the command is updated in place.
// Returns true if a hook was added or updated.
// Returns an error if the event name is not a valid Claude Code hook event.
func UpsertHookEntry(settings map[string]any, event, command string, identifier string) (bool, error) {
	if _, ok := validHookEvents[event]; !ok {
		return false, fmt.Errorf("invalid hook event %q: see https://code.claude.com/docs/en/hooks for valid events", event)
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooksMap = make(map[string]any)
		settings["hooks"] = hooksMap
	}

	groups, _ := hooksMap[event].([]any)

	// Check if already wired; update command in place if stale.
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
			cmd, _ := entry["command"].(string)
			if !strings.Contains(cmd, identifier) {
				continue
			}
			if cmd == command {
				return false, nil // already correct
			}
			entry["command"] = command
			return true, nil // updated stale command
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

// deprecatedHookEvents are event names that older claudette versions
// incorrectly wrote. Only these specific keys are cleaned up — we never
// delete hook events we don't own.
var deprecatedHookEvents = []string{"PostToolResult"}

// RemoveInvalidHookEvents cleans up hook event keys that older claudette
// versions wrote incorrectly (e.g., "PostToolResult" instead of "PostToolUse").
// Only removes known-bad keys — never touches events registered by other tools.
func RemoveInvalidHookEvents(settings map[string]any) {
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return
	}
	for _, key := range deprecatedHookEvents {
		delete(hooksMap, key)
	}
}
