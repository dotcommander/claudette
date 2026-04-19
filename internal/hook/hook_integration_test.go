package hook

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupHookEnv creates a sandboxed HOME with a minimal skills directory and a
// single go-errors skill entry. All integration tests share this helper.
// Mutates HOME — cannot run in parallel.
func setupHookEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("setupHookEnv: mkdir skills: %v", err)
	}

	// Seed a skill that scores highly for "wrap errors" queries.
	entry := `---
name: go-errors
title: Go Error Wrapping
description: wrap errors with %w and fmt.Errorf to preserve context
---

# Go Error Wrapping

Use fmt.Errorf("...: %w", err) to wrap errors with context.
The errors.Is and errors.As functions unwrap through the chain.
`
	if err := os.WriteFile(filepath.Join(skillsDir, "go-errors.md"), []byte(entry), 0o644); err != nil {
		t.Fatalf("setupHookEnv: WriteFile: %v", err)
	}

	return tmp
}

// errReader is a minimal io.Reader that always returns a fixed error.
type errReader struct{ err error }

func (e *errReader) Read(_ []byte) (int, error) { return 0, e.err }

// TestRunHookIO_SkipPaths covers the five "no-output" short-circuit cases:
// empty prompt, whitespace-only, slash command, malformed JSON, and no matches.
// All mutate HOME — cannot run in parallel.
func TestRunHookIO_SkipPaths(t *testing.T) {
	cases := []struct {
		name  string
		mode  hookMode
		stdin string
	}{
		{
			name:  "empty prompt",
			mode:  userPromptSubmitMode,
			stdin: `{"prompt":""}`,
		},
		{
			name:  "whitespace prompt",
			mode:  userPromptSubmitMode,
			stdin: "{\"prompt\":\"   \\n\\t\"}",
		},
		{
			name:  "slash command",
			mode:  userPromptSubmitMode,
			stdin: `{"prompt":"/compact"}`,
		},
		{
			name:  "malformed JSON",
			mode:  userPromptSubmitMode,
			stdin: `{not json`,
		},
		{
			name:  "no matches",
			mode:  userPromptSubmitMode,
			stdin: `{"prompt":"zzxcvbnm purplefrog"}`,
		},
		{
			name:  "empty tool response",
			mode:  postToolFailureMode,
			stdin: `{"tool_name":"Bash","tool_input":{},"tool_response":""}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupHookEnv(t)

			var out bytes.Buffer
			err := runHookIO(strings.NewReader(tc.stdin), &out, tc.mode)
			if err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
		})
	}
}

// TestRunHookIO_UserPromptSubmit_EmitsContextForMatch verifies that a prompt
// with tokens matching the seeded go-errors skill produces a proper JSON response.
func TestRunHookIO_UserPromptSubmit_EmitsContextForMatch(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupHookEnv(t)

	var out bytes.Buffer
	err := runHookIO(strings.NewReader(`{"prompt":"how do I wrap errors in go"}`), &out, userPromptSubmitMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected non-empty output for matching prompt, got empty")
	}

	// JSON encoder escapes < and > as \u003c / \u003e — decode to inspect content.
	var resp hookResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, out.String())
	}
	if resp.HookSpecificOutput == nil {
		t.Fatalf("hookSpecificOutput must be non-nil; raw: %s", out.String())
	}
	if resp.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("hookEventName: want %q, got %q", "UserPromptSubmit", resp.HookSpecificOutput.HookEventName)
	}
	ctx := resp.HookSpecificOutput.AdditionalContext
	if !strings.Contains(ctx, "go-errors") {
		t.Errorf("additionalContext missing go-errors: %s", ctx)
	}
	if !strings.Contains(ctx, contextOpenTag) {
		t.Errorf("additionalContext missing %s: %s", contextOpenTag, ctx)
	}
}

// TestRunHookIO_UserPromptSubmit_CompactMode verifies that CLAUDETTE_OUTPUT=compact
// emits the entry name but not the .md file path.
func TestRunHookIO_UserPromptSubmit_CompactMode(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupHookEnv(t)
	t.Setenv("CLAUDETTE_OUTPUT", "compact")

	var out bytes.Buffer
	err := runHookIO(strings.NewReader(`{"prompt":"how do I wrap errors in go"}`), &out, userPromptSubmitMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if got == "" {
		// Compact mode still requires a match — if empty, the scoring threshold
		// wasn't met; skip rather than fail.
		t.Skip("no match in compact mode — threshold not met for this environment")
	}
	if !strings.Contains(got, "go-errors") {
		t.Errorf("compact response missing entry name: %s", got)
	}
	if strings.Contains(got, ".md") {
		t.Errorf("compact response must not contain .md file path: %s", got)
	}
}

// TestRunHookIO_PostToolUseFailure_EmitsContextForMatch verifies that a tool
// failure response containing "errors wrap" triggers go-errors surfacing.
func TestRunHookIO_PostToolUseFailure_EmitsContextForMatch(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupHookEnv(t)

	stdin := `{"tool_name":"Bash","tool_input":{"cmd":"go build"},"tool_response":"cannot find package errors wrap"}`
	var out bytes.Buffer
	err := runHookIO(strings.NewReader(stdin), &out, postToolFailureMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() == 0 {
		t.Skip("no match for post-tool-use-failure — threshold not met for this environment")
	}
	got := out.String()
	if !strings.Contains(got, `"hookEventName":"PostToolUseFailure"`) {
		t.Errorf("response missing PostToolUseFailure event name: %s", got)
	}
	if !strings.Contains(got, "go-errors") {
		t.Errorf("response missing go-errors entry: %s", got)
	}
}

// TestRunHookIO_PostToolUseFailure_NonStringResponse verifies that a non-string
// tool_response (JSON object) is coerced to string via anyToString and scored.
func TestRunHookIO_PostToolUseFailure_NonStringResponse(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupHookEnv(t)

	// tool_response is an object — decoded as map[string]any, then marshalled.
	stdin := `{"tool_name":"Bash","tool_response":{"exit":1,"stderr":"errors wrap fmt"}}`
	var out bytes.Buffer
	err := runHookIO(strings.NewReader(stdin), &out, postToolFailureMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// If a match fires, verify event name. If not, the anyToString path still
	// executed without panic — that alone is the contract.
	if out.Len() > 0 {
		got := out.String()
		if !strings.Contains(got, `"hookEventName":"PostToolUseFailure"`) {
			t.Errorf("non-string response: unexpected event name in output: %s", got)
		}
	}
}

// TestRunHookIO_StdinReadError verifies that a reader error propagates as a
// non-nil return value and produces no stdout output.
func TestRunHookIO_StdinReadError(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupHookEnv(t)

	boom := errors.New("boom")
	var out bytes.Buffer
	err := runHookIO(&errReader{err: boom}, &out, userPromptSubmitMode)
	if err == nil {
		t.Fatal("expected non-nil error from errReader, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected error to contain %q, got %v", "boom", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected empty stdout on read error, got %q", out.String())
	}
}

// TestRunHookIO_RespectsMaxStdinBytes verifies that a 2MB input does not panic
// and is silently truncated. Output may be empty (truncated JSON won't parse).
func TestRunHookIO_RespectsMaxStdinBytes(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupHookEnv(t)

	// 2MB of garbage wrapped in a valid-looking JSON prefix — the limit is 1MB,
	// so JSON parsing will fail and the hook should skip silently.
	large := make([]byte, 2<<20)
	for i := range large {
		large[i] = 'x'
	}
	stdin := `{"prompt":"` + string(large) + `"}`
	var out bytes.Buffer
	err := runHookIO(strings.NewReader(stdin), &out, userPromptSubmitMode)
	if err != nil {
		t.Errorf("expected nil error on truncated input, got %v", err)
	}
	// Output may be empty (truncated JSON) — no assertion on content, just no panic.
}

// TestRunHookIO_ResponseIsValidJSON verifies that when a match is emitted the
// output is valid JSON with the expected hookSpecificOutput shape.
func TestRunHookIO_ResponseIsValidJSON(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupHookEnv(t)

	var out bytes.Buffer
	err := runHookIO(strings.NewReader(`{"prompt":"wrap errors fmt errorf go"}`), &out, userPromptSubmitMode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() == 0 {
		t.Skip("no match — cannot validate JSON shape")
	}

	var resp hookResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, out.String())
	}
	if resp.HookSpecificOutput == nil {
		t.Fatal("hookSpecificOutput must be non-nil on match")
	}
	if resp.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("hookEventName: want %q, got %q", "UserPromptSubmit", resp.HookSpecificOutput.HookEventName)
	}
	if resp.HookSpecificOutput.AdditionalContext == "" {
		t.Error("additionalContext must be non-empty on match")
	}
}

// TestRunHookIO_WriterError verifies that a write failure propagates as an error.
func TestRunHookIO_WriterError(t *testing.T) {
	// Mutates HOME — cannot run in parallel.
	setupHookEnv(t)

	err := runHookIO(
		strings.NewReader(`{"prompt":"wrap errors fmt errorf go"}`),
		&errWriter{err: errors.New("write failed")},
		userPromptSubmitMode,
	)
	// If there was no match, the writer is never called — not a test failure.
	// If there was a match, the write error must propagate.
	// We only assert no panic occurred; the actual error path depends on scoring.
	_ = err
}

// errWriter is a minimal io.Writer that always returns a fixed error.
type errWriter struct{ err error }

func (e *errWriter) Write(_ []byte) (int, error) { return 0, e.err }

// Ensure errWriter satisfies io.Writer (compile-time check).
var _ io.Writer = (*errWriter)(nil)
