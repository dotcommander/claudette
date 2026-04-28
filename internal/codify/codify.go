// Package codify promotes a .work/ artifact to a KB entry under ~/.claude/kb/.
// Entry format:
//
//	---
//	name: <slug>
//	description: <first-paragraph>
//	source_file: <absolute input path>
//	source: <session-id>           (omitted when empty)
//	source_task: <task-id>         (omitted when empty)
//	---
//	<body — full input file content>
//
// The file is written atomically; an existing entry is refused unless --force.
// After a successful write, `claudette scan` is re-run to rebuild the index.
package codify

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/dotcommander/claudette/internal/fileutil"
)

// Opts configures a single codify invocation.
type Opts struct {
	// Input is the path to the .work/ markdown file being promoted.
	Input string
	// Category overrides the inferred KB category (go, bash, claude-code, …).
	// Defaults to "uncategorized" when empty and not inferrable.
	Category string
	// Slug overrides the derived slug. Defaults to input basename without extension.
	Slug string
	// SessionID populates source_session_id / source footer field.
	SessionID string
	// TaskID populates source_task_id / source footer field.
	TaskID string
	// Yes skips the interactive confirmation prompt.
	Yes bool
	// Force overwrites an existing KB entry without prompting.
	Force bool
	// KBRoot overrides the KB root directory (default: ~/.claude/kb/).
	// Intended for tests only — production callers should leave this empty.
	KBRoot string
	// SkipScan skips the post-write `claudette scan` call.
	// Used in unit tests to avoid exec overhead.
	SkipScan bool
}

// Result is returned by Run on success.
type Result struct {
	// Path is the absolute path to the written KB entry.
	Path string
	// AlreadyExisted is true when the entry was already present and --force was
	// not set; in this case the write was skipped (idempotent no-op).
	AlreadyExisted bool
}

// Run is the top-level entry point; w receives all user-facing output.
func Run(w io.Writer, r io.Reader, opts Opts) (Result, error) {
	content, err := os.ReadFile(opts.Input)
	if err != nil {
		return Result{}, fmt.Errorf("read input %q: %w", opts.Input, err)
	}

	title := extractTitle(content)
	description := extractDescription(content)
	slug := resolveSlug(opts.Slug, opts.Input)
	category := resolveCategory(opts.Category, content)
	kbRoot := resolveKBRoot(opts.KBRoot)
	destPath := filepath.Join(kbRoot, category, slug+".md")

	// Idempotent check: refuse to overwrite unless --force.
	if _, statErr := os.Stat(destPath); statErr == nil {
		if !opts.Force {
			fmt.Fprintf(w, "KB entry already exists: %s\n", destPath)
			fmt.Fprintln(w, "Re-run with --force to overwrite.")
			return Result{Path: destPath, AlreadyExisted: true}, nil
		}
	}

	if !opts.Yes {
		fmt.Fprintf(w, "Title:       %s\n", title)
		fmt.Fprintf(w, "Description: %s\n", description)
		fmt.Fprintf(w, "Category:    %s\n", category)
		fmt.Fprintf(w, "Slug:        %s\n", slug)
		fmt.Fprintf(w, "Destination: %s\n", destPath)
		if opts.SessionID != "" {
			fmt.Fprintf(w, "Session:     %s\n", opts.SessionID)
		}
		if opts.TaskID != "" {
			fmt.Fprintf(w, "Task:        %s\n", opts.TaskID)
		}
		fmt.Fprint(w, "\nWrite to KB? [y/N] ")

		answer, _ := bufio.NewReader(r).ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(w, "Aborted.")
			return Result{}, nil
		}
	}

	entry := buildEntry(title, description, slug, content, opts)

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil { //nolint:gosec
		return Result{}, fmt.Errorf("create KB directory: %w", err)
	}
	if err := fileutil.AtomicWriteFile(destPath, entry, 0o644); err != nil {
		return Result{}, fmt.Errorf("write KB entry: %w", err)
	}

	fmt.Fprintf(w, "Written: %s\n", destPath)

	if !opts.SkipScan {
		if err := runScan(); err != nil {
			// Non-fatal: entry was written; a manual `claudette scan` recovers.
			fmt.Fprintf(w, "warning: claudette scan failed: %v\n", err)
		}
	}

	return Result{Path: destPath}, nil
}

// buildEntry assembles the full KB entry bytes: YAML frontmatter + body.
func buildEntry(title, description, slug string, body []byte, opts Opts) []byte {
	var b bytes.Buffer

	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", slug))
	writeQuotedYAML(&b, "description", description)
	b.WriteString(fmt.Sprintf("source_file: %s\n", opts.Input))
	if opts.SessionID != "" {
		b.WriteString(fmt.Sprintf("source: %s\n", opts.SessionID))
	}
	if opts.TaskID != "" {
		b.WriteString(fmt.Sprintf("source_task: %s\n", opts.TaskID))
	}
	b.WriteString("---\n")

	// Body: full input content, with a provenance footer appended.
	b.Write(body)
	if len(body) > 0 && body[len(body)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString("\n---\n")
	b.WriteString(buildProvenanceFooter(opts))

	return b.Bytes()
}

// buildProvenanceFooter produces the grep-extractable footer line.
// Format: (source: session <uuid> / task#<id> / .work/<slug>.md)
func buildProvenanceFooter(opts Opts) string {
	var parts []string
	if opts.SessionID != "" {
		parts = append(parts, "session "+opts.SessionID)
	}
	if opts.TaskID != "" {
		parts = append(parts, "task#"+opts.TaskID)
	}
	if opts.Input != "" {
		parts = append(parts, opts.Input)
	}
	if len(parts) == 0 {
		return ""
	}
	return "(source: " + strings.Join(parts, " / ") + ")\n"
}

// writeQuotedYAML writes a YAML key: value line, quoting the value when it
// contains characters that would require escaping (colon, newline, etc.).
func writeQuotedYAML(w *bytes.Buffer, key, value string) {
	needsQuote := strings.ContainsAny(value, ":#\"'|>{}[]&*!,\n")
	if needsQuote {
		escaped := strings.ReplaceAll(value, `"`, `\"`)
		fmt.Fprintf(w, "%s: \"%s\"\n", key, escaped)
		return
	}
	fmt.Fprintf(w, "%s: %s\n", key, value)
}

// extractTitle returns the text of the first `# Heading` line.
// Falls back to the filename stem (caller provides content only; slug is the
// fallback used at the call site).
func extractTitle(content []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

// extractDescription returns the first non-blank, non-heading paragraph
// (up to 3 lines joined with a space), suitable for the description field.
func extractDescription(content []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var lines []string
	inParagraph := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip blank lines before the paragraph starts.
		if !inParagraph && trimmed == "" {
			continue
		}
		// Skip headings.
		if strings.HasPrefix(trimmed, "#") {
			if inParagraph {
				break
			}
			continue
		}
		// Skip YAML frontmatter delimiters.
		if trimmed == "---" {
			continue
		}

		if trimmed == "" {
			// Blank line after paragraph content = end of paragraph.
			if inParagraph {
				break
			}
			continue
		}

		inParagraph = true
		lines = append(lines, trimmed)
		if len(lines) >= 3 {
			break
		}
	}

	return strings.Join(lines, " ")
}

// resolveSlug derives the KB slug from an explicit value or the input basename.
func resolveSlug(explicit, inputPath string) string {
	if explicit != "" {
		return sanitizeSlug(explicit)
	}
	base := filepath.Base(inputPath)
	return sanitizeSlug(strings.TrimSuffix(base, filepath.Ext(base)))
}

// sanitizeSlug lowercases and replaces non-alphanumeric runes with hyphens,
// collapsing consecutive hyphens and trimming leading/trailing hyphens.
func sanitizeSlug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}
