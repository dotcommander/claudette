package session

import (
	"strings"
	"testing"
)

func TestSanitize_ANSIStrip(t *testing.T) {
	t.Parallel()

	input := "\x1b[31mred\x1b[0m text \x1b[1mbold\x1b[0m"
	got := sanitize(input)
	if strings.Contains(got, "\x1b") {
		t.Errorf("sanitize did not strip ANSI codes: %q", got)
	}
	if !strings.Contains(got, "red") || !strings.Contains(got, "text") || !strings.Contains(got, "bold") {
		t.Errorf("sanitize stripped visible text: %q", got)
	}
}

func TestSanitize_WhitespaceCollapse(t *testing.T) {
	t.Parallel()

	input := "a    b\t\t\tc"
	got := sanitize(input)
	// Multiple spaces/tabs collapse to single space; newlines are preserved.
	if strings.Contains(got, "  ") {
		t.Errorf("sanitize left multiple consecutive spaces: %q", got)
	}
	if !strings.Contains(got, "a b") || !strings.Contains(got, "b c") {
		t.Errorf("sanitize garbled text: %q", got)
	}
}

func TestSanitize_TruncateCap(t *testing.T) {
	t.Parallel()

	// Input longer than maxContentLen (3000 chars) must be truncated with ellipsis.
	input := strings.Repeat("a", 4000)
	got := sanitize(input)
	if len(got) > maxContentLen+len("…") {
		t.Errorf("sanitize output len = %d, want <= %d", len(got), maxContentLen+len("…"))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("sanitize did not append ellipsis: suffix = %q", got[len(got)-5:])
	}
}

func TestSanitize_ExactlyAtLimit(t *testing.T) {
	t.Parallel()

	// Input exactly at the limit should not be truncated.
	input := strings.Repeat("b", maxContentLen)
	got := sanitize(input)
	if strings.HasSuffix(got, "…") {
		t.Errorf("sanitize truncated input at exactly maxContentLen: %q", got[len(got)-5:])
	}
}

func TestSanitize_SystemReminderBlock(t *testing.T) {
	t.Parallel()

	input := "before <system-reminder>secret context</system-reminder> after"
	got := sanitize(input)
	if strings.Contains(got, "secret context") {
		t.Errorf("system-reminder content not stripped: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("visible content stripped by mistake: %q", got)
	}
}

func TestSanitize_SystemReminderSelfClosing(t *testing.T) {
	t.Parallel()

	input := `before <system-reminder data="x"/> after`
	got := sanitize(input)
	if strings.Contains(got, "system-reminder") {
		t.Errorf("self-closing system-reminder tag not stripped: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("visible content stripped by mistake: %q", got)
	}
}

func TestSanitize_LineNumberPrefixes(t *testing.T) {
	t.Parallel()

	// Simulate Read tool output: "42\tcode here"
	input := "1\tpackage scan\n2\t\n3\tfunc Scan() {}"
	got := sanitize(input)
	if strings.Contains(got, "1\t") || strings.Contains(got, "2\t") || strings.Contains(got, "3\t") {
		t.Errorf("line-number prefixes not stripped: %q", got)
	}
	if !strings.Contains(got, "package scan") {
		t.Errorf("code content stripped by mistake: %q", got)
	}
}

func TestSanitize_Empty(t *testing.T) {
	t.Parallel()

	got := sanitize("")
	if got != "" {
		t.Errorf("sanitize(\"\") = %q, want \"\"", got)
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel…"},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc…"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			got := truncate(tc.input, tc.maxLen)
			if got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
			}
		})
	}
}

func TestExtractArtifacts_Basic(t *testing.T) {
	t.Parallel()

	input := `Some text before.
<antArtifact identifier="my-func" type="application/vnd.ant.code" language="go" title="My Function">
func hello() string { return "hi" }
</antArtifact>
Some text after.`

	cleaned, artifacts := extractArtifacts(input)
	if strings.Contains(cleaned, "antArtifact") {
		t.Errorf("antArtifact tag not removed from cleaned text: %q", cleaned)
	}
	if !strings.Contains(cleaned, "Some text before") {
		t.Error("surrounding text stripped from cleaned output")
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifacts count = %d, want 1", len(artifacts))
	}
	a := artifacts[0]
	if a.Identifier != "my-func" {
		t.Errorf("Identifier = %q, want 'my-func'", a.Identifier)
	}
	if a.Type != "application/vnd.ant.code" {
		t.Errorf("Type = %q, want 'application/vnd.ant.code'", a.Type)
	}
	if a.Language != "go" {
		t.Errorf("Language = %q, want 'go'", a.Language)
	}
	if a.Title != "My Function" {
		t.Errorf("Title = %q, want 'My Function'", a.Title)
	}
	if !strings.Contains(a.Content, "hello") {
		t.Errorf("Content = %q, want to contain 'hello'", a.Content)
	}
}

func TestExtractArtifacts_LanguageAnyAttrOrder(t *testing.T) {
	t.Parallel()

	// language appears before type — two-pass attr parser must handle any order.
	input := `<antArtifact language="typescript" identifier="ts-fn" type="application/vnd.ant.code" title="TS Func">const x = 1</antArtifact>`
	_, artifacts := extractArtifacts(input)
	if len(artifacts) != 1 {
		t.Fatalf("artifacts count = %d, want 1", len(artifacts))
	}
	a := artifacts[0]
	if a.Language != "typescript" {
		t.Errorf("Language = %q, want 'typescript'", a.Language)
	}
	if a.Identifier != "ts-fn" {
		t.Errorf("Identifier = %q, want 'ts-fn'", a.Identifier)
	}
}

func TestExtractArtifacts_NoLanguage(t *testing.T) {
	t.Parallel()

	// Artifacts without a language attribute should have Language == "".
	input := `<antArtifact identifier="plain" type="text/plain" title="Plain">hello world</antArtifact>`
	_, artifacts := extractArtifacts(input)
	if len(artifacts) != 1 {
		t.Fatalf("artifacts count = %d, want 1", len(artifacts))
	}
	if artifacts[0].Language != "" {
		t.Errorf("Language = %q, want empty string for non-code artifact", artifacts[0].Language)
	}
}

func TestExtractArtifacts_ContentCap(t *testing.T) {
	t.Parallel()

	// Content exceeding maxArtifactLen must be truncated.
	longBody := strings.Repeat("x", maxArtifactLen+100)
	input := `<antArtifact identifier="big" type="text/plain" language="" title="Big">` + longBody + `</antArtifact>`

	_, artifacts := extractArtifacts(input)
	if len(artifacts) != 1 {
		t.Fatalf("artifacts count = %d, want 1", len(artifacts))
	}
	if len(artifacts[0].Content) > maxArtifactLen+len("…") {
		t.Errorf("artifact content len = %d, want <= %d+ellipsis", len(artifacts[0].Content), maxArtifactLen)
	}
}

func TestExtractArtifacts_NoArtifacts(t *testing.T) {
	t.Parallel()

	input := "plain text with no artifacts"
	cleaned, artifacts := extractArtifacts(input)
	if cleaned != input {
		t.Errorf("cleaned = %q, want unchanged %q", cleaned, input)
	}
	if len(artifacts) != 0 {
		t.Errorf("artifacts count = %d, want 0", len(artifacts))
	}
}

func TestExtractThinking_Basic(t *testing.T) {
	t.Parallel()

	input := "response text <antThinking>my reasoning</antThinking> more text"
	cleaned, thinking := extractThinking(input)
	if strings.Contains(cleaned, "antThinking") {
		t.Errorf("antThinking tag not removed: %q", cleaned)
	}
	if !strings.Contains(cleaned, "response text") {
		t.Error("surrounding text stripped")
	}
	if thinking != "my reasoning" {
		t.Errorf("thinking = %q, want 'my reasoning'", thinking)
	}
}

func TestExtractThinking_None(t *testing.T) {
	t.Parallel()

	input := "just a normal response"
	cleaned, thinking := extractThinking(input)
	if cleaned != input {
		t.Errorf("cleaned = %q, want unchanged %q", cleaned, input)
	}
	if thinking != "" {
		t.Errorf("thinking = %q, want empty", thinking)
	}
}

func TestCollapseInlineWhitespace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"a  b", "a b"},
		{"a\t\tb", "a b"},
		{"a\n  b", "a\n b"}, // newline preserved, trailing spaces on next line collapsed
		{"no change", "no change"},
		{"", ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			got := collapseInlineWhitespace(tc.input)
			if got != tc.want {
				t.Errorf("collapseInlineWhitespace(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
