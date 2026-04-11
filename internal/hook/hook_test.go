package hook

import (
	"strings"
	"testing"

	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/search"
)

func makeResults(entries ...index.Entry) []search.ScoredEntry {
	results := make([]search.ScoredEntry, len(entries))
	for i, e := range entries {
		results[i] = search.ScoredEntry{Entry: e, Score: 10 - i}
	}
	return results
}

func TestFormatContext_Full(t *testing.T) {
	t.Parallel()
	results := makeResults(
		index.Entry{Name: "hook-reload", Title: "Hook Reload Caching", FilePath: "/tmp/test/hook-reload.md"},
	)
	got := formatContext(results, "full")
	if !strings.Contains(got, "read before proceeding") {
		t.Error("full mode should contain 'read before proceeding' header")
	}
	if !strings.Contains(got, "hook-reload.md") {
		t.Error("full mode should contain file path")
	}
	if !strings.Contains(got, "Hook Reload Caching") {
		t.Error("full mode should contain title")
	}
}

func TestFormatContext_Compact(t *testing.T) {
	t.Parallel()
	results := makeResults(
		index.Entry{Name: "hook-reload", Title: "Hook Reload Caching", Desc: "Session caching invalidation gotcha", FilePath: "/tmp/test/hook-reload.md"},
	)
	got := formatContext(results, "compact")
	if !strings.Contains(got, "read with Read tool") {
		t.Error("compact mode should contain 'read with Read tool' header")
	}
	if !strings.Contains(got, "hook-reload") {
		t.Error("compact mode should contain entry name")
	}
	if !strings.Contains(got, "Session caching invalidation gotcha") {
		t.Error("compact mode should contain description")
	}
	if strings.Contains(got, "hook-reload.md") {
		t.Error("compact mode should not contain file path")
	}
}

func TestFormatContext_CompactFallsBackToTitle(t *testing.T) {
	t.Parallel()
	results := makeResults(
		index.Entry{Name: "kb-entry", Title: "Some KB Title", Desc: "", FilePath: "/tmp/kb/entry.md"},
	)
	got := formatContext(results, "compact")
	if !strings.Contains(got, "Some KB Title") {
		t.Error("compact mode should fall back to title when desc is empty")
	}
}

func TestOutputMode_Default(t *testing.T) {
	t.Setenv("CLAUDETTE_OUTPUT", "")
	if got := outputMode(); got != "full" {
		t.Errorf("got %q, want %q", got, "full")
	}
}

func TestOutputMode_Compact(t *testing.T) {
	t.Setenv("CLAUDETTE_OUTPUT", "compact")
	if got := outputMode(); got != "compact" {
		t.Errorf("got %q, want %q", got, "compact")
	}
}

func TestOutputMode_Unknown(t *testing.T) {
	t.Setenv("CLAUDETTE_OUTPUT", "unknown")
	if got := outputMode(); got != "full" {
		t.Errorf("unknown value should default to full, got %q", got)
	}
}
