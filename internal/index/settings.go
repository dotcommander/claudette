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

// RemoveHookEntries removes every hook entry across all events whose command
// contains identifier. Prunes empty hook groups and empty event arrays so the
// file doesn't accumulate dangling keys. Returns the number of entries removed.
func RemoveHookEntries(settings map[string]any, identifier string) int {
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return 0
	}
	removed := 0
	// Snapshot keys: pruneEventHooks may delete entries from hooksMap.
	events := make([]string, 0, len(hooksMap))
	for event := range hooksMap {
		events = append(events, event)
	}
	for _, event := range events {
		removed += pruneEventHooks(hooksMap, event, identifier)
	}
	return removed
}

// RemoveHookEntriesForEvent removes entries from a single event whose command
// contains identifier. Used to migrate claudette off previously-wired event
// names (e.g. PostToolUse -> PostToolUseFailure). Returns the count removed.
func RemoveHookEntriesForEvent(settings map[string]any, event, identifier string) int {
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return 0
	}
	return pruneEventHooks(hooksMap, event, identifier)
}

// pruneEventHooks strips entries for a single event whose command contains
// identifier. Drops emptied groups and deletes the event key if everything
// was ours. Returns the number of hook entries removed.
func pruneEventHooks(hooksMap map[string]any, event, identifier string) int {
	groups, ok := hooksMap[event].([]any)
	if !ok {
		return 0
	}
	removed := 0
	keptGroups := make([]any, 0, len(groups))
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			keptGroups = append(keptGroups, g)
			continue
		}
		hookList, ok := group["hooks"].([]any)
		if !ok {
			keptGroups = append(keptGroups, g)
			continue
		}
		keptHooks := make([]any, 0, len(hookList))
		for _, h := range hookList {
			entry, ok := h.(map[string]any)
			if !ok {
				keptHooks = append(keptHooks, h)
				continue
			}
			cmd, _ := entry["command"].(string)
			if strings.Contains(cmd, identifier) {
				removed++
				continue
			}
			keptHooks = append(keptHooks, h)
		}
		if len(keptHooks) == 0 {
			continue // drop group whose hooks are all ours
		}
		group["hooks"] = keptHooks
		keptGroups = append(keptGroups, group)
	}
	if len(keptGroups) == 0 {
		delete(hooksMap, event)
	} else {
		hooksMap[event] = keptGroups
	}
	return removed
}

// deprecatedHookEvents are event names that older claudette versions
// incorrectly wrote. Only these specific keys are cleaned up — we never
// delete hook events we don't own.
var deprecatedHookEvents = []string{"PostToolResult"}

// RemoveInvalidHookEvents deletes hook event keys listed in
// deprecatedHookEvents — event names that older claudette versions wrote but
// that Claude Code never recognised (e.g., "PostToolResult"). Distinct from
// RemoveHookEntriesForEvent, which migrates entries off a valid-but-retired
// event like "PostToolUse". Only touches known-bad keys.
func RemoveInvalidHookEvents(settings map[string]any) {
	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return
	}
	for _, key := range deprecatedHookEvents {
		delete(hooksMap, key)
	}
}
