package actions

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShowConfig_MissingFile_WritesEmptyObject(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp) // no ~/.config/claudette/ exists

	var buf bytes.Buffer
	if err := ShowConfig(&buf); err != nil {
		t.Fatalf("ShowConfig: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "{}" {
		t.Errorf("empty config should render as %q, got %q", "{}", got)
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("output must end with newline, got %q", buf.String())
	}
}

func TestShowConfig_WithFile_PrettyPrintsJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgDir := filepath.Join(tmp, ".config", "claudette")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{"context_header":"from disk"}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var buf bytes.Buffer
	if err := ShowConfig(&buf); err != nil {
		t.Fatalf("ShowConfig: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"context_header": "from disk"`) {
		t.Errorf("output missing context_header field: %q", out)
	}
	// Pretty-print invariant: indented output contains a leading newline+spaces before fields.
	if !strings.Contains(out, "\n  \"context_header\"") {
		t.Errorf("output not indented with 2 spaces: %q", out)
	}
}

func TestShowConfigPath_ReturnsExpectedPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	var buf bytes.Buffer
	if err := ShowConfigPath(&buf); err != nil {
		t.Fatalf("ShowConfigPath: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	want := filepath.Join(tmp, ".config", "claudette", "config.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
