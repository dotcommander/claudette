package session

import (
	"regexp"
	"strings"
)

// maxContentLen is the per-message character cap after sanitization.
const maxContentLen = 3000

// maxArtifactLen is the character cap for artifact content stored in a Turn.
const maxArtifactLen = 500

// ansiRe matches ANSI escape sequences (e.g. color codes from terminal output).
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// systemReminderRe matches <system-reminder>...</system-reminder> blocks.
var systemReminderRe = regexp.MustCompile(`(?s)<system-reminder[^>]*>.*?</system-reminder>`)

// systemReminderSelfRe matches self-closing <system-reminder .../> tags.
var systemReminderSelfRe = regexp.MustCompile(`<system-reminder[^/]*/\s*>`)

// lineNumPrefixRe matches line-number tab prefixes inserted by the Read tool
// (e.g. "42\t" at the start of a line).
var lineNumPrefixRe = regexp.MustCompile(`(?m)^\d+\t`)

// antArtifactOpenRe matches a full <antArtifact ...>...</antArtifact> block,
// capturing the raw attribute string (group 1) and body content (group 2).
// Two-pass approach: capture all attrs as a blob, then parse per-attr below.
var antArtifactOpenRe = regexp.MustCompile(`(?s)<antArtifact([^>]*)>(.*?)</antArtifact>`)

// antArtifactAttrRe parses a single name="value" pair from the attribute blob.
var antArtifactAttrRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

// antThinkingRe matches antThinking tags in data-export messages.
var antThinkingRe = regexp.MustCompile(`<antThinking>([\s\S]*?)</antThinking>`)

// threeBlankLinesRe matches three or more consecutive blank lines.
var threeBlankLinesRe = regexp.MustCompile(`(\n\s*){3,}`)

// sanitize runs the full sanitization pipeline on a raw content string.
// Steps run in order: artifact extraction (done upstream), strip system-reminders,
// strip line-number prefixes, collapse blank lines, strip ANSI, collapse whitespace,
// then cap length. Artifact and antThinking extraction is separate (see extractArtifacts).
func sanitize(s string) string {
	if s == "" {
		return s
	}

	// Step 1-2: strip system-reminder blocks and self-closing tags.
	s = systemReminderRe.ReplaceAllString(s, "")
	s = systemReminderSelfRe.ReplaceAllString(s, "")

	// Step 3: strip line-number tab prefixes from Read tool output.
	s = lineNumPrefixRe.ReplaceAllString(s, "")

	// Step 5: collapse 3+ consecutive blank lines to one.
	s = threeBlankLinesRe.ReplaceAllString(s, "\n\n")

	// Step 6 (partial): strip ANSI escape codes.
	s = ansiRe.ReplaceAllString(s, "")

	// Collapse consecutive inline whitespace to single space.
	s = collapseInlineWhitespace(s)

	s = strings.TrimSpace(s)

	// Step 6 (cap): truncate with ellipsis.
	return truncate(s, maxContentLen)
}

// collapseInlineWhitespace replaces runs of spaces/tabs (not newlines) with a single space.
func collapseInlineWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		} else {
			inSpace = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

// truncate caps s at maxLen, appending "…" (U+2026) if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Find a safe rune boundary at or before maxLen.
	end := maxLen
	for end > 0 && s[end]&0xc0 == 0x80 {
		end--
	}
	return s[:end] + "…"
}

// extractArtifacts pulls antArtifact tags out of text, returning the cleaned
// text (tags removed) and a slice of Artifact values. Must run before sanitize.
// Uses a two-pass approach: first capture the raw attribute blob + body, then
// parse each attribute individually. This fixes the greedy [^>]* bug that
// prevented Language from being captured when it appeared before other attrs.
func extractArtifacts(text string) (string, []Artifact) {
	var artifacts []Artifact
	cleaned := antArtifactOpenRe.ReplaceAllStringFunc(text, func(match string) string {
		sub := antArtifactOpenRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return ""
		}
		attrs := map[string]string{}
		for _, am := range antArtifactAttrRe.FindAllStringSubmatch(sub[1], -1) {
			attrs[am[1]] = am[2]
		}
		artifacts = append(artifacts, Artifact{
			Identifier: attrs["identifier"],
			Type:       attrs["type"],
			Language:   attrs["language"],
			Title:      attrs["title"],
			Content:    truncate(strings.TrimSpace(sub[2]), maxArtifactLen),
		})
		return ""
	})
	return cleaned, artifacts
}

// extractThinking strips antThinking tags from text and returns (cleanedText, thinkingText).
// Used for data-export messages where thinking is embedded as XML rather than a native block.
func extractThinking(text string) (string, string) {
	var thinking strings.Builder
	cleaned := antThinkingRe.ReplaceAllStringFunc(text, func(match string) string {
		sub := antThinkingRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return ""
		}
		if thinking.Len() > 0 {
			thinking.WriteString("\n")
		}
		thinking.WriteString(sub[1])
		return ""
	})
	return cleaned, thinking.String()
}
