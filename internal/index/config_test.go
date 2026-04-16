package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig_ContextHeaderOrDefault_EmptyReturnsDefault(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	got := cfg.ContextHeaderOrDefault()
	if got != defaultContextHeader {
		t.Errorf("empty Config should return defaultContextHeader, got %q", got)
	}
	if !strings.Contains(got, "Only read full files") {
		t.Errorf("defaultContextHeader lost its expected content: %q", got)
	}
}

func TestConfig_ContextHeaderOrDefault_WhitespaceReturnsDefault(t *testing.T) {
	t.Parallel()
	cfg := Config{ContextHeader: "   \n\t  "}
	if got := cfg.ContextHeaderOrDefault(); got != defaultContextHeader {
		t.Errorf("whitespace-only header should fall back to default, got %q", got)
	}
}

func TestConfig_ContextHeaderOrDefault_CustomReturnsCustom(t *testing.T) {
	t.Parallel()
	custom := "My override instruction."
	cfg := Config{ContextHeader: custom}
	if got := cfg.ContextHeaderOrDefault(); got != custom {
		t.Errorf("custom header should pass through, got %q", got)
	}
}

func TestConfig_JSONRoundTrip_OmitsEmptyContextHeader(t *testing.T) {
	t.Parallel()
	cfg := Config{SourceDirs: []string{"/tmp/x"}}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "context_header") {
		t.Errorf("empty ContextHeader must be omitted via omitempty, got %s", data)
	}
}

func TestConfig_JSONRoundTrip_PreservesCustomContextHeader(t *testing.T) {
	t.Parallel()
	cfg := Config{ContextHeader: "custom"}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round Config
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.ContextHeader != "custom" {
		t.Errorf("ContextHeader round-trip failed: got %q", round.ContextHeader)
	}
}

// TestLoadConfig_ReadsContextHeaderFromDisk redirects HOME to a temp dir, so
// ConfigPath() resolves inside the sandbox and we can exercise the real
// LoadConfig path without mocking. This test cannot run with t.Parallel()
// because it mutates the HOME env var.
func TestLoadConfig_ReadsContextHeaderFromDisk(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfgDir := filepath.Join(tmp, ".config", "claudette")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{"context_header": "from disk"}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.ContextHeaderOrDefault(); got != "from disk" {
		t.Errorf("expected config-file header, got %q", got)
	}
}
