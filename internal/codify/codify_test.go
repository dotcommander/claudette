package codify_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotcommander/claudette/internal/codify"
)

// fixture builds a minimal .work/ markdown file in a temp dir and returns its path.
func fixture(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

const sampleDoc = `# Go Race Condition in Slice Append

Concurrent appends to the same slice without a mutex cause data races.
Use a mutex or a channel to coordinate writes.

## Details

More detail here.
`

func TestExtractTitleAndDescription(t *testing.T) {
	t.Parallel()
	input := fixture(t, "race.md", sampleDoc)
	kbDir := t.TempDir()

	var out bytes.Buffer
	res, err := codify.Run(&out, strings.NewReader("y\n"), codify.Opts{
		Input:    input,
		KBRoot:   kbDir,
		Yes:      true,
		SkipScan: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Path == "" {
		t.Fatal("expected a non-empty result path")
	}

	written, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	got := string(written)

	// Slug derives from the input filename ("race.md" → "race"), not the title.
	if !strings.Contains(got, "name: race") {
		t.Errorf("slug not found in frontmatter:\n%s", got)
	}
	if !strings.Contains(got, "Concurrent appends") {
		t.Errorf("description not extracted:\n%s", got)
	}
	if !strings.Contains(got, "source_file: "+input) {
		t.Errorf("source_file not in frontmatter:\n%s", got)
	}
}

func TestProvenanceFooter(t *testing.T) {
	t.Parallel()
	input := fixture(t, "myfeature.md", sampleDoc)
	kbDir := t.TempDir()

	var out bytes.Buffer
	res, err := codify.Run(&out, strings.NewReader(""), codify.Opts{
		Input:     input,
		KBRoot:    kbDir,
		Yes:       true,
		SkipScan:  true,
		SessionID: "sess-abc-123",
		TaskID:    "task-42",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	written, _ := os.ReadFile(res.Path)
	got := string(written)

	if !strings.Contains(got, "(source: session sess-abc-123 / task#task-42 /") {
		t.Errorf("provenance footer missing or malformed:\n%s", got)
	}
	// source and source_task must also appear in frontmatter.
	if !strings.Contains(got, "source: sess-abc-123") {
		t.Errorf("source field missing from frontmatter:\n%s", got)
	}
	if !strings.Contains(got, "source_task: task-42") {
		t.Errorf("source_task field missing from frontmatter:\n%s", got)
	}
}

func TestIdempotentRefuse(t *testing.T) {
	t.Parallel()
	input := fixture(t, "idempotent.md", sampleDoc)
	kbDir := t.TempDir()

	opts := codify.Opts{
		Input:    input,
		KBRoot:   kbDir,
		Yes:      true,
		SkipScan: true,
	}

	var out bytes.Buffer
	res1, err := codify.Run(&out, strings.NewReader(""), opts)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if res1.AlreadyExisted {
		t.Fatal("first run should not report AlreadyExisted")
	}

	out.Reset()
	res2, err := codify.Run(&out, strings.NewReader(""), opts)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if !res2.AlreadyExisted {
		t.Error("second run should report AlreadyExisted")
	}
	if !strings.Contains(out.String(), "already exists") {
		t.Errorf("expected 'already exists' message, got: %s", out.String())
	}
}

func TestForceOverwrite(t *testing.T) {
	t.Parallel()
	input := fixture(t, "force.md", sampleDoc)
	kbDir := t.TempDir()

	opts := codify.Opts{
		Input:    input,
		KBRoot:   kbDir,
		Yes:      true,
		SkipScan: true,
	}

	if _, err := codify.Run(&bytes.Buffer{}, strings.NewReader(""), opts); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	opts.Force = true
	var out bytes.Buffer
	res, err := codify.Run(&out, strings.NewReader(""), opts)
	if err != nil {
		t.Fatalf("force Run: %v", err)
	}
	if res.AlreadyExisted {
		t.Error("--force run should not report AlreadyExisted")
	}
}

func TestCategoryInference(t *testing.T) {
	t.Parallel()
	cases := []struct {
		title   string
		wantCat string
	}{
		{"# Go Goroutine Leak", "go"},
		{"# Bash Shell Escaping", "bash"},
		{"# Claude Code Hook Design", "claude-code"},
		{"# OpenAI API Retry Logic", "llm"},
		{"# Refactoring the Repository Layer", "refactoring"},
		{"# Some Random Note", "uncategorized"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()
			input := fixture(t, "test.md", tc.title+"\n\nSome description.\n")
			kbDir := t.TempDir()

			res, err := codify.Run(&bytes.Buffer{}, strings.NewReader(""), codify.Opts{
				Input:    input,
				KBRoot:   kbDir,
				Yes:      true,
				SkipScan: true,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !strings.Contains(res.Path, "/"+tc.wantCat+"/") {
				t.Errorf("want category %q in path, got: %s", tc.wantCat, res.Path)
			}
		})
	}
}

func TestSlugSanitization(t *testing.T) {
	t.Parallel()
	cases := []struct {
		filename string
		wantSlug string
	}{
		{"my-feature.md", "my-feature"},
		{"Repair_02_Codify.md", "repair-02-codify"},
		{"some file with spaces.md", "some-file-with-spaces"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.filename, func(t *testing.T) {
			t.Parallel()
			input := fixture(t, tc.filename, "# Title\n\nBody.\n")
			kbDir := t.TempDir()

			res, err := codify.Run(&bytes.Buffer{}, strings.NewReader(""), codify.Opts{
				Input:    input,
				KBRoot:   kbDir,
				Yes:      true,
				SkipScan: true,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !strings.HasSuffix(res.Path, "/"+tc.wantSlug+".md") {
				t.Errorf("want slug %q, got path: %s", tc.wantSlug, res.Path)
			}
		})
	}
}

func TestAbortOnNoConfirm(t *testing.T) {
	t.Parallel()
	input := fixture(t, "abort.md", sampleDoc)
	kbDir := t.TempDir()

	var out bytes.Buffer
	res, err := codify.Run(&out, strings.NewReader("n\n"), codify.Opts{
		Input:    input,
		KBRoot:   kbDir,
		SkipScan: true,
		// Yes: false — interactive mode
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Path != "" {
		t.Errorf("expected empty path on abort, got: %s", res.Path)
	}
	if !strings.Contains(out.String(), "Aborted") {
		t.Errorf("expected Aborted message, got: %s", out.String())
	}
	// Verify nothing was written to kbDir.
	entries, _ := os.ReadDir(kbDir)
	if len(entries) != 0 {
		t.Errorf("expected empty kbDir after abort, found %d entries", len(entries))
	}
}
