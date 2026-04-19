package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEncodePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"/Users/alice/go/src/claudette", "-Users-alice-go-src-claudette"},
		{"/Users/alice/go/src/my.project", "-Users-alice-go-src-my-project"},
		{"/tmp/simple", "-tmp-simple"},
		{"", ""},
		{"/", "-"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			got := EncodePath(tc.input)
			if got != tc.want {
				t.Errorf("EncodePath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDecodePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"-Users-alice-go-src-claudette", "/Users/alice/go/src/claudette"},
		{"-tmp-simple", "/tmp/simple"},
		{"", ""},
		// dotfile directories: double-dash decodes to "/<dot>"
		{"-Users-alice--claude", "/Users/alice/.claude"},
		{"-Users-alice--claude-plugins", "/Users/alice/.claude/plugins"},
		{"-tmp--cache-foo", "/tmp/.cache/foo"},
		// paths without dots round-trip cleanly
		{"-Users-alice-go-src-claudette", "/Users/alice/go/src/claudette"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			got := decodePath(tc.input)
			if got != tc.want {
				t.Errorf("decodePath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestEncodePath_RoundTrip(t *testing.T) {
	t.Parallel()

	// For paths without dots, encode→decode is a round-trip.
	paths := []string{
		"/Users/alice/go/src/claudette",
		"/tmp/simple",
		"/home/user/projects/foo",
	}

	for _, p := range paths {
		p := p
		t.Run(p, func(t *testing.T) {
			t.Parallel()

			encoded := EncodePath(p)
			decoded := decodePath(encoded)
			if decoded != p {
				t.Errorf("encode→decode(%q) = %q, want original %q", p, decoded, p)
			}
		})
	}
}

// TestEnumerateSessions_FromFixture exercises the real fixture tree which has:
//
//	-tmp-fake-project/
//	  a1b2c3d4-e5f6-7890-abcd-ef1234567890.jsonl        (no companion dir)
//	  b2c3d4e5-f6a7-8901-bcde-f12345678901.jsonl        (has companion dir)
//	  b2c3d4e5-f6a7-8901-bcde-f12345678901/subagents/agent-aaa.jsonl
func TestEnumerateSessions_FromFixture(t *testing.T) {
	t.Parallel()

	projectDir := filepath.Join("testdata", "projects", "-tmp-fake-project")
	metas, err := enumerateSessions(context.Background(), projectDir)
	if err != nil {
		t.Fatalf("enumerateSessions: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("expected 2 sessions from fixture, got %d", len(metas))
	}

	// Index by UUID for stable assertions.
	byUUID := make(map[string]SessionMeta, len(metas))
	for _, m := range metas {
		byUUID[m.UUID] = m
	}

	// Session 1: flat only, no companion dir.
	uuid1 := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	m1, ok := byUUID[uuid1]
	if !ok {
		t.Fatalf("session %q not found in results", uuid1)
	}
	if m1.UUID != uuid1 {
		t.Errorf("UUID = %q, want %q", m1.UUID, uuid1)
	}
	if m1.CompanionDir != "" {
		t.Errorf("CompanionDir = %q, want empty", m1.CompanionDir)
	}
	if m1.SubagentPaths != nil {
		t.Errorf("SubagentPaths = %v, want nil", m1.SubagentPaths)
	}
	if m1.Size == 0 {
		t.Error("Size = 0, want > 0")
	}
	if m1.MessageCount == 0 {
		t.Error("MessageCount = 0, want > 0")
	}

	// Session 2: has companion dir with one subagent.
	uuid2 := "b2c3d4e5-f6a7-8901-bcde-f12345678901"
	m2, ok := byUUID[uuid2]
	if !ok {
		t.Fatalf("session %q not found in results", uuid2)
	}
	if m2.CompanionDir == "" {
		t.Error("CompanionDir is empty, want non-empty")
	}
	if len(m2.SubagentPaths) != 1 {
		t.Errorf("SubagentPaths len = %d, want 1", len(m2.SubagentPaths))
	}
}

func TestEnumerateSessions_SkipsMemoryDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a "memory" file (should be skipped).
	if err := os.WriteFile(filepath.Join(dir, "memory.jsonl"), []byte(`{}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile memory.jsonl: %v", err)
	}
	// Create a valid flat session transcript.
	uuid := "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	transcriptPath := filepath.Join(dir, uuid+".jsonl")
	if err := os.WriteFile(transcriptPath, []byte(`{"type":"user","uuid":"x","timestamp":"2026-04-19T00:00:00Z","message":{"role":"user","content":"hi"}}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile transcript: %v", err)
	}

	metas, err := enumerateSessions(context.Background(), dir)
	if err != nil {
		t.Fatalf("enumerateSessions: %v", err)
	}
	if len(metas) != 1 {
		t.Errorf("got %d sessions, want 1 (memory.jsonl must be skipped)", len(metas))
	}
}

func TestEnumerateSessions_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// A dir with no .jsonl files is valid — empty project, not an error.
	metas, err := enumerateSessions(context.Background(), dir)
	if err != nil {
		t.Fatalf("enumerateSessions on empty dir: %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("got %d sessions, want 0", len(metas))
	}
}

func TestBuildSessionMeta_FlatOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	uuid := "cccccccc-1111-2222-3333-555555555555"
	transcriptPath := filepath.Join(dir, uuid+".jsonl")
	content := `{"type":"user","uuid":"x","timestamp":"2026-04-19T00:00:00Z","message":{"role":"user","content":"hello"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	meta, ok := buildSessionMeta(dir, uuid)
	if !ok {
		t.Fatal("buildSessionMeta returned ok=false, want ok=true")
	}
	if meta.UUID != uuid {
		t.Errorf("UUID = %q, want %q", meta.UUID, uuid)
	}
	if meta.TranscriptPath != transcriptPath {
		t.Errorf("TranscriptPath = %q, want %q", meta.TranscriptPath, transcriptPath)
	}
	if meta.CompanionDir != "" {
		t.Errorf("CompanionDir = %q, want empty (no sibling dir)", meta.CompanionDir)
	}
	if meta.SubagentPaths != nil {
		t.Errorf("SubagentPaths = %v, want nil", meta.SubagentPaths)
	}
	if meta.Size == 0 {
		t.Error("Size = 0, want > 0")
	}
	if meta.MessageCount == 0 {
		t.Error("MessageCount = 0, want > 0 (1 JSON line)")
	}
}

func TestBuildSessionMeta_WithCompanionDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	uuid := "dddddddd-2222-3333-4444-666666666666"

	// Flat transcript.
	transcriptPath := filepath.Join(dir, uuid+".jsonl")
	content := `{"type":"user","uuid":"x","timestamp":"2026-04-19T00:00:00Z","message":{"role":"user","content":"hi"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile transcript: %v", err)
	}

	// Companion dir with one subagent.
	subagentDir := filepath.Join(dir, uuid, "subagents")
	if err := os.MkdirAll(subagentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subagentDir, "agent-xyz.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile subagent: %v", err)
	}

	meta, ok := buildSessionMeta(dir, uuid)
	if !ok {
		t.Fatal("buildSessionMeta returned ok=false, want ok=true")
	}
	if meta.CompanionDir == "" {
		t.Error("CompanionDir is empty, want non-empty")
	}
	if len(meta.SubagentPaths) != 1 {
		t.Errorf("SubagentPaths len = %d, want 1", len(meta.SubagentPaths))
	}
}

func TestBuildSessionMeta_MissingTranscript(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, ok := buildSessionMeta(dir, "nonexistent-uuid")
	if ok {
		t.Error("buildSessionMeta returned ok=true for missing transcript, want ok=false")
	}
}

func TestGlobSubagentPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	subDir := filepath.Join(dir, "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write two agent files and one non-matching file.
	if err := os.WriteFile(filepath.Join(subDir, "agent-abc.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "agent-def.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "other.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	paths := globSubagentPaths(dir)
	if len(paths) != 2 {
		t.Errorf("globSubagentPaths returned %d paths, want 2", len(paths))
	}
}

func TestAllProjects_Override(t *testing.T) {
	// Mutates package-level claudeProjectsDir — must not run in parallel.

	// Build a minimal projects tree in a temp dir (flat layout):
	//   <root>/-tmp-fake-project/<uuid>.jsonl
	root := t.TempDir()
	encoded := "-tmp-fake-project"
	uuid := "dddddddd-1111-2222-3333-666666666666"
	projectDir := filepath.Join(root, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	transcriptPath := filepath.Join(projectDir, uuid+".jsonl")
	line := `{"type":"user","uuid":"x","timestamp":"2026-04-19T00:00:00Z","message":{"role":"user","content":"hi"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(line), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	orig := claudeProjectsDir
	claudeProjectsDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { claudeProjectsDir = orig })

	projects, err := AllProjects(context.Background())
	if err != nil {
		t.Fatalf("AllProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("AllProjects returned %d projects, want 1", len(projects))
	}
	if projects[0].EncodedName != encoded {
		t.Errorf("EncodedName = %q, want %q", projects[0].EncodedName, encoded)
	}
	if len(projects[0].Sessions) != 1 {
		t.Errorf("Sessions count = %d, want 1", len(projects[0].Sessions))
	}
	sess := projects[0].Sessions[0]
	if sess.UUID != uuid {
		t.Errorf("Session UUID = %q, want %q", sess.UUID, uuid)
	}
}

func TestAllProjects_EmptyRoot(t *testing.T) {
	// Mutates package-level claudeProjectsDir — must not run in parallel.
	root := t.TempDir()
	orig := claudeProjectsDir
	claudeProjectsDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { claudeProjectsDir = orig })

	projects, err := AllProjects(context.Background())
	if err != nil {
		t.Fatalf("AllProjects on empty root: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("AllProjects returned %d projects on empty root, want 0", len(projects))
	}
}

func TestCurrentProject_Override(t *testing.T) {
	// Mutates package-level claudeProjectsDir — must not run in parallel.

	// CurrentProject encodes os.Getwd() — build a session dir matching that encoding.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	encoded := EncodePath(cwd)

	root := t.TempDir()
	uuid := "eeeeeeee-aaaa-bbbb-cccc-dddddddddddd"
	projectDir := filepath.Join(root, encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	transcriptPath := filepath.Join(projectDir, uuid+".jsonl")
	line := `{"type":"user","uuid":"x","timestamp":"2026-04-19T00:00:00Z","message":{"role":"user","content":"test"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(line), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	orig := claudeProjectsDir
	claudeProjectsDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { claudeProjectsDir = orig })

	proj, err := CurrentProject(context.Background())
	if err != nil {
		t.Fatalf("CurrentProject: %v", err)
	}
	if proj.OriginalPath != cwd {
		t.Errorf("OriginalPath = %q, want %q", proj.OriginalPath, cwd)
	}
	if proj.EncodedName != encoded {
		t.Errorf("EncodedName = %q, want %q", proj.EncodedName, encoded)
	}
	if len(proj.Sessions) != 1 {
		t.Errorf("Sessions count = %d, want 1", len(proj.Sessions))
	}
	if proj.Sessions[0].UUID != uuid {
		t.Errorf("Session UUID = %q, want %q", proj.Sessions[0].UUID, uuid)
	}
}
