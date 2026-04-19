package actions

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupExplainEnv creates a sandboxed HOME with a minimal skill so the index
// builds at least one entry for explain tests.
func setupExplainEnv(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	skillsDir := filepath.Join(tmp, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("setupExplainEnv: mkdir skills: %v", err)
	}
	// Write a skill that's deliberately findable by the "channel" token.
	const skill = `---
name: channel-patterns
title: Channel Patterns
description: Go channel usage and concurrency patterns.
---

# Channel Patterns

Goroutine channel concurrency pipeline.
`
	if err := os.WriteFile(filepath.Join(skillsDir, "channel-patterns.md"), []byte(skill), 0o644); err != nil {
		t.Fatalf("setupExplainEnv: WriteFile: %v", err)
	}
	// Config dir must exist for index to be written.
	cfgDir := filepath.Join(tmp, ".config", "claudette")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("setupExplainEnv: mkdir config: %v", err)
	}
}

// TestExplain_EmptyPrompt_ReturnsError verifies empty prompt is rejected before
// any index access.
func TestExplain_EmptyPrompt_ReturnsError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := Explain(&buf, "", NewExplainOpts())
	if err == nil {
		t.Fatal("expected error for empty prompt, got nil")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty prompt, got %q", buf.String())
	}
}

// TestExplain_RendersText verifies text output contains expected sections.
// Cannot be parallel: uses t.Setenv to sandbox HOME.
func TestExplain_RendersText(t *testing.T) {
	setupExplainEnv(t)

	var buf bytes.Buffer
	err := Explain(&buf, "channel patterns", NewExplainOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"tokens", "corpus", "explain:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing section %q; got:\n%s", want, out)
		}
	}
}

// TestExplain_RendersJSON verifies JSON output is valid and has expected shape.
// Cannot be parallel: uses t.Setenv to sandbox HOME.
func TestExplain_RendersJSON(t *testing.T) {
	setupExplainEnv(t)

	opts := NewExplainOpts()
	opts.JSON = true

	var buf bytes.Buffer
	err := Explain(&buf, "channel patterns", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v\noutput: %s", err, buf.String())
	}

	for _, key := range []string{"prompt", "tokens", "corpus", "diagnostics"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("JSON missing key %q; keys present: %v", key, mapKeys(parsed))
		}
	}
}

// TestExplain_LimitClamp verifies that Limit=2 restricts the breakdown.
// Limit=0 means all entries.
// Cannot be parallel: uses t.Setenv to sandbox HOME.
func TestExplain_LimitClamp(t *testing.T) {
	setupExplainEnv(t)

	opts2 := NewExplainOpts()
	opts2.Limit = 2
	opts2.JSON = true

	var buf2 bytes.Buffer
	if err := Explain(&buf2, "channel patterns", opts2); err != nil {
		t.Fatalf("Limit=2: unexpected error: %v", err)
	}
	var p2 map[string]any
	if err := json.Unmarshal(buf2.Bytes(), &p2); err != nil {
		t.Fatalf("Limit=2 JSON parse error: %v", err)
	}
	diags2, _ := p2["diagnostics"].([]any)
	if len(diags2) > 2 {
		t.Errorf("Limit=2: expected at most 2 diagnostics, got %d", len(diags2))
	}
}

// mapKeys returns the keys of a map[string]any for diagnostic messages.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
