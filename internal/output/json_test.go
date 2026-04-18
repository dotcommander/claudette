package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/search"
)

func fixtureResults() []search.ScoredEntry {
	return []search.ScoredEntry{
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
			Matched: []string{"go"},
		},
		{
			Entry: index.Entry{
				Type:     index.TypeAgent,
				Name:     "read-agent",
				Title:    "Read Agent",
				Category: "agent",
				FilePath: "/agents/read-agent.md",
			},
			Score:   2,
			Matched: nil,
		},
	}
}

func TestWriteJSON_Shape(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := WriteJSON(&buf, fixtureResults()); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var result SearchResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%s", err, buf.String())
	}

	if result.Total != 3 {
		t.Errorf("Total = %d; want 3", result.Total)
	}
	if len(result.Matches) != 3 {
		t.Fatalf("len(Matches) = %d; want 3", len(result.Matches))
	}

	m := result.Matches[0]
	if m.Type != "skill" {
		t.Errorf("Matches[0].Type = %q; want %q", m.Type, "skill")
	}
	if m.Name != "code-clean-code" {
		t.Errorf("Matches[0].Name = %q; want %q", m.Name, "code-clean-code")
	}
	if m.Score != 5 {
		t.Errorf("Matches[0].Score = %d; want 5", m.Score)
	}
	if len(m.Matched) != 2 {
		t.Errorf("Matches[0].Matched = %v; want [refactor solid]", m.Matched)
	}
}

func TestWriteJSON_ExpectedSubstrings(t *testing.T) {
	t.Parallel()

	cases := []string{
		`"matches"`,
		`"total"`,
		`"code-clean-code"`,
		`"go-patterns"`,
		`"read-agent"`,
		`"type"`,
		`"score"`,
		`"matched"`,
	}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, fixtureResults()); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	raw := buf.String()

	for _, sub := range cases {
		if !strings.Contains(raw, sub) {
			t.Errorf("expected %q in JSON output; raw=%s", sub, raw)
		}
	}
}

func TestWriteJSON_Empty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := WriteJSON(&buf, nil); err != nil {
		t.Fatalf("WriteJSON(nil): %v", err)
	}

	var result SearchResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total = %d; want 0", result.Total)
	}
	if len(result.Matches) != 0 {
		t.Errorf("len(Matches) = %d; want 0", len(result.Matches))
	}
}
