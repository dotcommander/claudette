package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var baseTime = time.Date(2026, 4, 19, 8, 0, 0, 0, time.UTC)

// makeUserMsg returns a minimal human user Message.
func makeUserMsg(content string, ts time.Time) Message {
	return Message{
		Type:         "user",
		Role:         "user",
		Content:      content,
		Timestamp:    ts,
		IsToolResult: false,
	}
}

// makeAssistantMsg returns a minimal assistant Message.
func makeAssistantMsg(content, toolName string, toolInput json.RawMessage, ts time.Time) Message {
	return Message{
		Type:      "assistant",
		Role:      "assistant",
		Content:   content,
		ToolName:  toolName,
		ToolInput: toolInput,
		Timestamp: ts,
	}
}

// makeToolResultMsg returns a user Message that is a tool result.
func makeToolResultMsg(sourceID string, toolUseResult json.RawMessage) Message {
	return Message{
		Type:            "user",
		Role:            "user",
		IsToolResult:    true,
		SourceToolUseID: sourceID,
		ToolUseResult:   toolUseResult,
	}
}

func TestExtractTurns_Empty(t *testing.T) {
	t.Parallel()

	turns := ExtractTurns(nil)
	if len(turns) != 0 {
		t.Errorf("expected empty result, got %d turns", len(turns))
	}
}

func TestExtractTurns_Happy(t *testing.T) {
	t.Parallel()

	msgs := []Message{
		makeUserMsg("fix the bug", baseTime),
		makeAssistantMsg("I will fix it", "", nil, baseTime.Add(1)),
	}
	turns := ExtractTurns(msgs)
	if len(turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(turns))
	}
	if !strings.Contains(turns[0].UserContent, "fix the bug") {
		t.Errorf("UserContent = %q, want 'fix the bug'", turns[0].UserContent)
	}
	if !strings.Contains(turns[0].AssistantSummary, "I will fix it") {
		t.Errorf("AssistantSummary = %q, want 'I will fix it'", turns[0].AssistantSummary)
	}
}

func TestExtractTurns_MultiTurn(t *testing.T) {
	t.Parallel()

	msgs := []Message{
		makeUserMsg("turn one", baseTime),
		makeAssistantMsg("response one", "", nil, baseTime.Add(1)),
		makeUserMsg("turn two", baseTime.Add(2)),
		makeAssistantMsg("response two", "", nil, baseTime.Add(3)),
	}
	turns := ExtractTurns(msgs)
	if len(turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(turns))
	}
	if !strings.Contains(turns[0].UserContent, "turn one") {
		t.Errorf("turn[0].UserContent = %q", turns[0].UserContent)
	}
	if !strings.Contains(turns[1].UserContent, "turn two") {
		t.Errorf("turn[1].UserContent = %q", turns[1].UserContent)
	}
}

func TestExtractTurns_ToolClassification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		toolName  string
		input     json.RawMessage
		wantRead  []string
		wantEdit  []string
		wantTools []string
	}{
		{
			name:     "Read",
			toolName: "Read",
			input:    json.RawMessage(`{"file_path":"internal/scan.go"}`),
			wantRead: []string{"internal/scan.go"},
		},
		{
			name:     "Edit",
			toolName: "Edit",
			input:    json.RawMessage(`{"file_path":"pkg/foo.go"}`),
			wantEdit: []string{"pkg/foo.go"},
		},
		{
			name:     "Write",
			toolName: "Write",
			input:    json.RawMessage(`{"file_path":"pkg/bar.go"}`),
			wantEdit: []string{"pkg/bar.go"},
		},
		{
			name:     "Glob",
			toolName: "Glob",
			input:    json.RawMessage(`{"pattern":"internal/**/*.go"}`),
			wantRead: []string{"internal/**/*.go"},
		},
		{
			name:     "Grep",
			toolName: "Grep",
			input:    json.RawMessage(`{"path":"internal/","pattern":"func"}`),
			wantRead: []string{"internal/"},
		},
		{
			name:      "Agent",
			toolName:  "Agent",
			input:     json.RawMessage(`{}`),
			wantTools: []string{"Agent"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msgs := []Message{
				makeUserMsg("do work", baseTime),
				makeAssistantMsg("", tc.toolName, tc.input, baseTime.Add(1)),
			}
			turns := ExtractTurns(msgs)
			if len(turns) == 0 {
				t.Fatal("no turns")
			}
			turn := turns[0]

			for _, want := range tc.wantRead {
				if !contains(turn.FilesRead, want) {
					t.Errorf("FilesRead = %v, want %q", turn.FilesRead, want)
				}
			}
			for _, want := range tc.wantEdit {
				if !contains(turn.FilesEdited, want) {
					t.Errorf("FilesEdited = %v, want %q", turn.FilesEdited, want)
				}
			}
			for _, want := range tc.wantTools {
				if !contains(turn.ToolsUsed, want) {
					t.Errorf("ToolsUsed = %v, want %q", turn.ToolsUsed, want)
				}
			}
		})
	}
}

func TestExtractTurns_BashFileExtraction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		command   string
		wantRead  []string
		wantEdit  []string
		wantEmpty bool
	}{
		{
			name:     "cat reads a file",
			command:  "cat internal/scan.go",
			wantRead: []string{"internal/scan.go"},
		},
		{
			name:     "head reads a file",
			command:  "head -n 10 pkg/foo.go",
			wantRead: []string{"pkg/foo.go"},
		},
		{
			name:     "tail reads a file",
			command:  "tail -20 pkg/bar.go",
			wantRead: []string{"pkg/bar.go"},
		},
		{
			name:     "rm edits a file",
			command:  "rm pkg/old.go",
			wantEdit: []string{"pkg/old.go"},
		},
		{
			name:      "go test does not extract paths",
			command:   "go test ./foo/bar.go",
			wantEmpty: true,
		},
		{
			name:      "git diff does not extract paths",
			command:   "git diff HEAD",
			wantEmpty: true,
		},
		{
			name:      "unknown command does not extract paths",
			command:   "ls -la",
			wantEmpty: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := json.RawMessage(`{"command":` + jsonString(tc.command) + `}`)
			msgs := []Message{
				makeUserMsg("run something", baseTime),
				makeAssistantMsg("", "Bash", input, baseTime.Add(1)),
			}
			turns := ExtractTurns(msgs)
			if len(turns) == 0 {
				t.Fatal("no turns")
			}
			turn := turns[0]

			if tc.wantEmpty {
				if len(turn.FilesRead) != 0 || len(turn.FilesEdited) != 0 {
					t.Errorf("expected no file paths, got read=%v edited=%v", turn.FilesRead, turn.FilesEdited)
				}
				return
			}
			for _, want := range tc.wantRead {
				if !contains(turn.FilesRead, want) {
					t.Errorf("FilesRead = %v, want %q", turn.FilesRead, want)
				}
			}
			for _, want := range tc.wantEdit {
				if !contains(turn.FilesEdited, want) {
					t.Errorf("FilesEdited = %v, want %q", turn.FilesEdited, want)
				}
			}
		})
	}
}

func TestExtractTurns_FrustrationMarkers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		text string
		want bool
	}{
		{"no not that, please redo it", true},
		{"stop doing that", true},
		{"that's wrong", true},
		{"i said use the other file", true},
		{"don't do that", true},
		{"what the hell is going on", true},
		{"wtf", true},
		{"how many times do I have to say this", true},
		{"REDO IT NOW", true}, // all-caps short message
		{"please fix this", false},
		{"can you redo the function?", false},
		{"thanks", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()

			got := isFrustrated(tc.text)
			if got != tc.want {
				t.Errorf("isFrustrated(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestExtractTurns_OrphanToolResult(t *testing.T) {
	t.Parallel()

	// A tool result with no prior human turn must not panic.
	msgs := []Message{
		makeToolResultMsg("orphan_id", json.RawMessage(`{"filePath":"foo.go"}`)),
	}
	turns := ExtractTurns(msgs) // must not panic
	_ = turns
}

func TestExtractTurns_ToolResultNoCurrentTurn(t *testing.T) {
	t.Parallel()

	// Tool result before any human message — no current turn, must not panic.
	msgs := []Message{
		{
			Type:            "user",
			Role:            "user",
			IsToolResult:    true,
			SourceToolUseID: "call_x",
			ToolUseResult:   json.RawMessage(`{"filePath":"bar.go"}`),
		},
		makeUserMsg("now a real turn", baseTime),
	}
	turns := ExtractTurns(msgs)
	if len(turns) != 1 {
		t.Errorf("turns = %d, want 1", len(turns))
	}
}

func TestExtractTurns_ThinkingPropagated(t *testing.T) {
	t.Parallel()

	msgs := []Message{
		makeUserMsg("explain this", baseTime),
		{
			Type:      "assistant",
			Role:      "assistant",
			Content:   "here is the explanation",
			Thinking:  "I am reasoning about this",
			Timestamp: baseTime.Add(1),
		},
	}
	turns := ExtractTurns(msgs)
	if len(turns) == 0 {
		t.Fatal("no turns")
	}
	if turns[0].Thinking == "" {
		t.Error("Thinking is empty, want propagated from assistant message")
	}
	if !strings.Contains(turns[0].Thinking, "I am reasoning") {
		t.Errorf("Thinking = %q", turns[0].Thinking)
	}
}

func TestExtractTurns_ArtifactsPropagated(t *testing.T) {
	t.Parallel()

	artifact := Artifact{Identifier: "art1", Type: "application/vnd.ant.code", Language: "go", Title: "Example"}
	msgs := []Message{
		makeUserMsg("write me some code", baseTime),
		{
			Type:      "assistant",
			Role:      "assistant",
			Content:   "here is some code",
			Artifacts: []Artifact{artifact},
			Timestamp: baseTime.Add(1),
		},
	}
	turns := ExtractTurns(msgs)
	if len(turns) == 0 {
		t.Fatal("no turns")
	}
	if len(turns[0].Artifacts) == 0 {
		t.Error("Artifacts is empty, want artifact propagated from assistant message")
	}
	if turns[0].Artifacts[0].Identifier != "art1" {
		t.Errorf("Artifacts[0].Identifier = %q, want 'art1'", turns[0].Artifacts[0].Identifier)
	}
}

func TestCoalesce(t *testing.T) {
	t.Parallel()

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"", "", "first"}, "first"},
		{[]string{"first", "second"}, "first"},
		{[]string{"", ""}, ""},
		{[]string{}, ""},
		{[]string{"only"}, "only"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.want+"_"+strings.Join(tc.args, "|"), func(t *testing.T) {
			t.Parallel()

			got := coalesce(tc.args...)
			if got != tc.want {
				t.Errorf("coalesce(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// contains reports whether s appears in ss.
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// jsonString returns a JSON-encoded string literal.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
