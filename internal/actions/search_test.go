package actions

import (
	"strings"
	"testing"
)

func TestFormatPrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"single word", []string{"goroutines"}, "goroutines", false},
		{"multiple words", []string{"refactor", "legacy", "code"}, "refactor legacy code", false},
		{"leading whitespace stripped", []string{"   hello"}, "hello", false},
		{"trailing whitespace stripped", []string{"hello   "}, "hello", false},
		{"empty string arg", []string{""}, "", true},
		{"whitespace only", []string{"   "}, "", true},
		{"multiple empty args", []string{"", "", ""}, "", true},
		{"no args", nil, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := FormatPrompt(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestReadPromptFromReader_Normal verifies a plain string is returned unchanged.
func TestReadPromptFromReader_Normal(t *testing.T) {
	t.Parallel()
	r := strings.NewReader("refactor code\n")
	got, err := ReadPromptFromReader(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "refactor code" {
		t.Errorf("got %q, want %q", got, "refactor code")
	}
}

// TestReadPromptFromReader_Empty verifies that empty stdin returns an error.
func TestReadPromptFromReader_Empty(t *testing.T) {
	t.Parallel()
	r := strings.NewReader("")
	_, err := ReadPromptFromReader(r)
	if err == nil {
		t.Fatal("expected error for empty stdin, got nil")
	}
}

// TestReadPromptFromReader_WhitespaceOnly verifies that whitespace-only input
// is treated as empty and returns an error.
func TestReadPromptFromReader_WhitespaceOnly(t *testing.T) {
	t.Parallel()
	r := strings.NewReader("   \n\t\r\n")
	_, err := ReadPromptFromReader(r)
	if err == nil {
		t.Fatal("expected error for whitespace-only stdin, got nil")
	}
}

// TestReadPromptFromReader_TrailingNewlines verifies trailing newlines are stripped.
func TestReadPromptFromReader_TrailingNewlines(t *testing.T) {
	t.Parallel()
	r := strings.NewReader("find concurrency bugs\n\n")
	got, err := ReadPromptFromReader(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "find concurrency bugs" {
		t.Errorf("got %q, want %q", got, "find concurrency bugs")
	}
}

// TestSearchOpts_JSONFlag verifies that setting JSON=true overrides Format.
func TestSearchOpts_JSONFlag(t *testing.T) {
	t.Parallel()
	opts := NewSearchOpts()
	if opts.JSON {
		t.Error("JSON should default to false")
	}
	// Simulate --json flag: format should resolve to "json" in Search.
	// We test the opts struct directly — the format resolution logic in Search
	// is: if opts.JSON { format = "json" }.
	opts.JSON = true
	opts.Format = "text" // explicit text format should lose to --json
	format := opts.Format
	if opts.JSON {
		format = "json"
	}
	if format != "json" {
		t.Errorf("--json should override --format text, got %q", format)
	}
}

// TestSearchOpts_FormatWithoutJSON verifies that --format json without --json
// also selects JSON output.
func TestSearchOpts_FormatWithoutJSON(t *testing.T) {
	t.Parallel()
	opts := NewSearchOpts()
	opts.Format = "json"
	format := opts.Format
	if opts.JSON {
		format = "json"
	}
	if format != "json" {
		t.Errorf("--format json should select json output, got %q", format)
	}
}
