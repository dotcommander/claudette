package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func hookCmd(s map[string]any, event string, groupIdx, hookIdx int) string {
	hooksMap, _ := s["hooks"].(map[string]any)
	groups, _ := hooksMap[event].([]any)
	if groupIdx >= len(groups) {
		return ""
	}
	group, _ := groups[groupIdx].(map[string]any)
	hookList, _ := group["hooks"].([]any)
	if hookIdx >= len(hookList) {
		return ""
	}
	entry, _ := hookList[hookIdx].(map[string]any)
	cmd, _ := entry["command"].(string)
	return cmd
}

func eventGroupCount(s map[string]any, event string) int {
	hooksMap, _ := s["hooks"].(map[string]any)
	groups, _ := hooksMap[event].([]any)
	return len(groups)
}

func eventExists(s map[string]any, event string) bool {
	hooksMap, _ := s["hooks"].(map[string]any)
	_, ok := hooksMap[event]
	return ok
}

// ── round-trip ────────────────────────────────────────────────────────────────

func TestReadWriteRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("write_then_read_preserves_content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")

		want := map[string]any{
			"hooks": map[string]any{
				"UserPromptSubmit": []any{
					map[string]any{"matcher": "", "hooks": []any{}},
				},
			},
		}

		if err := writeSettingsAt(path, want); err != nil {
			t.Fatalf("writeSettingsAt: %v", err)
		}

		got, err := readSettingsAt(path)
		if err != nil {
			t.Fatalf("readSettingsAt: %v", err)
		}

		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(got)
		if string(wantJSON) != string(gotJSON) {
			t.Errorf("round-trip mismatch\nwant: %s\ngot:  %s", wantJSON, gotJSON)
		}
	})

	t.Run("missing_file_returns_empty_map", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent.json")

		got, err := readSettingsAt(path)
		if err != nil {
			t.Fatalf("expected nil error for missing file, got: %v", err)
		}
		if got == nil {
			t.Fatal("expected non-nil empty map, got nil")
		}
		if len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})

	t.Run("malformed_json_returns_error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")

		if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		_, err := readSettingsAt(path)
		if err == nil {
			t.Fatal("expected error for malformed JSON, got nil")
		}
	})
}

// ── UpsertHookEntry ───────────────────────────────────────────────────────────

func TestUpsertHookEntry(t *testing.T) {
	t.Parallel()

	t.Run("invalid_event_returns_error", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		_, err := UpsertHookEntry(s, "NotARealEvent", "claudette hook", "claudette")
		if err == nil {
			t.Fatal("expected error for invalid event, got nil")
		}
	})

	t.Run("empty_settings_creates_event_and_entry", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		added, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook", "claudette")
		if err != nil {
			t.Fatalf("UpsertHookEntry: %v", err)
		}
		if !added {
			t.Error("expected added=true for new entry")
		}
		if eventGroupCount(s, "UserPromptSubmit") != 1 {
			t.Errorf("expected 1 group, got %d", eventGroupCount(s, "UserPromptSubmit"))
		}
		if cmd := hookCmd(s, "UserPromptSubmit", 0, 0); cmd != "claudette hook" {
			t.Errorf("expected command %q, got %q", "claudette hook", cmd)
		}
	})

	t.Run("idempotent_when_command_matches", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook", "claudette"); err != nil {
			t.Fatalf("first upsert: %v", err)
		}
		added, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook", "claudette")
		if err != nil {
			t.Fatalf("second upsert: %v", err)
		}
		if added {
			t.Error("expected added=false for duplicate command")
		}
		if eventGroupCount(s, "UserPromptSubmit") != 1 {
			t.Errorf("expected 1 group after idempotent upsert, got %d", eventGroupCount(s, "UserPromptSubmit"))
		}
	})

	t.Run("stale_command_updated_in_place", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook --old", "claudette"); err != nil {
			t.Fatalf("initial upsert: %v", err)
		}
		updated, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook --new", "claudette")
		if err != nil {
			t.Fatalf("update upsert: %v", err)
		}
		if !updated {
			t.Error("expected added=true when command updated in place")
		}
		if cmd := hookCmd(s, "UserPromptSubmit", 0, 0); cmd != "claudette hook --new" {
			t.Errorf("expected updated command, got %q", cmd)
		}
		if eventGroupCount(s, "UserPromptSubmit") != 1 {
			t.Errorf("expected 1 group after in-place update, got %d", eventGroupCount(s, "UserPromptSubmit"))
		}
	})

	t.Run("second_upsert_different_identifier_appends_group", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook", "claudette"); err != nil {
			t.Fatalf("first upsert: %v", err)
		}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "othertool hook", "othertool"); err != nil {
			t.Fatalf("second upsert: %v", err)
		}
		if n := eventGroupCount(s, "UserPromptSubmit"); n != 2 {
			t.Errorf("expected 2 groups for distinct identifiers, got %d", n)
		}
	})

	t.Run("multiple_events_independent", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook", "claudette"); err != nil {
			t.Fatalf("upsert UserPromptSubmit: %v", err)
		}
		if _, err := UpsertHookEntry(s, "PostToolUseFailure", "claudette hook", "claudette"); err != nil {
			t.Fatalf("upsert PostToolUseFailure: %v", err)
		}
		if eventGroupCount(s, "UserPromptSubmit") != 1 {
			t.Error("UserPromptSubmit group count wrong")
		}
		if eventGroupCount(s, "PostToolUseFailure") != 1 {
			t.Error("PostToolUseFailure group count wrong")
		}
	})
}

// ── RemoveHookEntriesForEvent ─────────────────────────────────────────────────

func TestRemoveHookEntriesForEvent(t *testing.T) {
	t.Parallel()

	t.Run("nonexistent_event_is_noop", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		n := RemoveHookEntriesForEvent(s, "UserPromptSubmit", "claudette")
		if n != 0 {
			t.Errorf("expected 0 removed, got %d", n)
		}
	})

	t.Run("removes_matching_command_and_prunes_event", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook", "claudette"); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		n := RemoveHookEntriesForEvent(s, "UserPromptSubmit", "claudette")
		if n != 1 {
			t.Errorf("expected 1 removed, got %d", n)
		}
		if eventExists(s, "UserPromptSubmit") {
			t.Error("expected event key pruned after all entries removed")
		}
	})

	t.Run("preserves_non_matching_commands", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		// Wire claudette and othertool under the same event
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook", "claudette"); err != nil {
			t.Fatalf("upsert claudette: %v", err)
		}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "othertool hook", "othertool"); err != nil {
			t.Fatalf("upsert othertool: %v", err)
		}
		n := RemoveHookEntriesForEvent(s, "UserPromptSubmit", "claudette")
		if n != 1 {
			t.Errorf("expected 1 removed, got %d", n)
		}
		if !eventExists(s, "UserPromptSubmit") {
			t.Fatal("expected event key preserved when non-matching commands remain")
		}
		// The othertool group must survive
		if eventGroupCount(s, "UserPromptSubmit") != 1 {
			t.Errorf("expected 1 remaining group, got %d", eventGroupCount(s, "UserPromptSubmit"))
		}
		if cmd := hookCmd(s, "UserPromptSubmit", 0, 0); cmd != "othertool hook" {
			t.Errorf("expected remaining command %q, got %q", "othertool hook", cmd)
		}
	})

	t.Run("no_match_returns_zero", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "othertool hook", "othertool"); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		n := RemoveHookEntriesForEvent(s, "UserPromptSubmit", "claudette")
		if n != 0 {
			t.Errorf("expected 0 removed when identifier absent, got %d", n)
		}
		if eventGroupCount(s, "UserPromptSubmit") != 1 {
			t.Error("expected unrelated group to survive")
		}
	})
}

// ── RemoveHookEntries (cross-event) ───────────────────────────────────────────

func TestRemoveHookEntries(t *testing.T) {
	t.Parallel()

	t.Run("removes_across_all_events", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		events := []string{"UserPromptSubmit", "PostToolUseFailure", "Stop"}
		for _, ev := range events {
			if _, err := UpsertHookEntry(s, ev, "claudette hook", "claudette"); err != nil {
				t.Fatalf("upsert %s: %v", ev, err)
			}
		}
		n := RemoveHookEntries(s, "claudette")
		if n != len(events) {
			t.Errorf("expected %d removed, got %d", len(events), n)
		}
		for _, ev := range events {
			if eventExists(s, ev) {
				t.Errorf("expected event %q pruned, but still present", ev)
			}
		}
	})

	t.Run("preserves_unrelated_identifiers", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook", "claudette"); err != nil {
			t.Fatalf("upsert claudette: %v", err)
		}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "othertool hook", "othertool"); err != nil {
			t.Fatalf("upsert othertool: %v", err)
		}
		n := RemoveHookEntries(s, "claudette")
		if n != 1 {
			t.Errorf("expected 1 removed, got %d", n)
		}
		if eventGroupCount(s, "UserPromptSubmit") != 1 {
			t.Errorf("expected 1 remaining group, got %d", eventGroupCount(s, "UserPromptSubmit"))
		}
	})

	t.Run("empty_hooks_map_returns_zero", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		n := RemoveHookEntries(s, "claudette")
		if n != 0 {
			t.Errorf("expected 0 from empty settings, got %d", n)
		}
	})
}

// ── RemoveInvalidHookEvents ───────────────────────────────────────────────────

func TestRemoveInvalidHookEvents(t *testing.T) {
	t.Parallel()

	t.Run("removes_deprecated_event_key", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{
			"hooks": map[string]any{
				"PostToolResult": []any{
					map[string]any{"matcher": "", "hooks": []any{}},
				},
			},
		}
		RemoveInvalidHookEvents(s)
		if eventExists(s, "PostToolResult") {
			t.Error("expected deprecated key PostToolResult to be removed")
		}
	})

	t.Run("preserves_valid_event_keys", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{}
		if _, err := UpsertHookEntry(s, "UserPromptSubmit", "claudette hook", "claudette"); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		RemoveInvalidHookEvents(s)
		if !eventExists(s, "UserPromptSubmit") {
			t.Error("expected valid event UserPromptSubmit to be preserved")
		}
	})

	t.Run("mix_of_valid_and_invalid", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{
			"hooks": map[string]any{
				"PostToolResult": []any{},
				"UserPromptSubmit": []any{
					map[string]any{"matcher": "", "hooks": []any{}},
				},
			},
		}
		RemoveInvalidHookEvents(s)
		if eventExists(s, "PostToolResult") {
			t.Error("expected PostToolResult removed")
		}
		if !eventExists(s, "UserPromptSubmit") {
			t.Error("expected UserPromptSubmit preserved")
		}
	})

	t.Run("no_hooks_key_is_noop", func(t *testing.T) {
		t.Parallel()
		s := map[string]any{"someOtherKey": "value"}
		RemoveInvalidHookEvents(s) // must not panic
	})
}
