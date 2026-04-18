package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/search"
)

func TestWriteText_ContainsExpected(t *testing.T) {
	t.Parallel()

	results := []search.ScoredEntry{
		{
			Entry: index.Entry{
				Type:     index.TypeSkill,
				Name:     "code-clean-code",
				Title:    "Clean Code",
				Category: "skill",
				FilePath: "/skills/code-clean-code.md",
			},
			Score:   5,
			Matched: []string{"refactor", "solid"},
		},
		{
			Entry: index.Entry{
				Type:     index.TypeKB,
				Name:     "go-patterns",
				Title:    "Go Patterns",
				Category: "go",
				FilePath: "/kb/go/go-patterns.md",
			},
			Score:   3,
			Matched: nil,
		},
	}

	var buf bytes.Buffer
	WriteText(&buf, results)
	out := buf.String()

	for _, sub := range []string{
		"Clean Code",
		"Go Patterns",
		"/skills/code-clean-code.md",
		"/kb/go/go-patterns.md",
		"refactor",
		"solid",
	} {
		if !strings.Contains(out, sub) {
			t.Errorf("expected %q in text output; got:\n%s", sub, out)
		}
	}
}

func TestWriteText_ScoresAppear(t *testing.T) {
	t.Parallel()

	results := []search.ScoredEntry{
		{
			Entry: index.Entry{Type: index.TypeAgent, Name: "read-agent", Title: "Read Agent", Category: "agent"},
			Score: 7,
		},
	}

	var buf bytes.Buffer
	WriteText(&buf, results)

	if !strings.Contains(buf.String(), "7") {
		t.Errorf("expected score 7 in text output; got:\n%s", buf.String())
	}
}

func TestWriteText_Empty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	WriteText(&buf, nil)
	out := buf.String()

	if out == "" {
		t.Error("expected non-empty output for empty results (should print no-match message)")
	}
	if !strings.Contains(out, "No matching") {
		t.Errorf("expected 'No matching' in empty output; got: %q", out)
	}
}

func TestWriteScanSummary_Format(t *testing.T) {
	t.Parallel()

	counts := map[string]int{
		"skill":   12,
		"kb":      34,
		"agent":   5,
		"command": 8,
	}

	var buf bytes.Buffer
	WriteScanSummary(&buf, counts, 59)
	out := buf.String()

	for _, sub := range []string{
		"59",
		"skill",
		"kb",
		"agent",
		"command",
		"12",
		"34",
		"5",
		"8",
	} {
		if !strings.Contains(out, sub) {
			t.Errorf("expected %q in scan summary; got:\n%s", sub, out)
		}
	}
}

func TestWriteScanSummary_Empty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	WriteScanSummary(&buf, nil, 0)
	out := buf.String()

	if !strings.Contains(out, "0") {
		t.Errorf("expected '0' in empty scan summary; got: %q", out)
	}
}
