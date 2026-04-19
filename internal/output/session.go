package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/dotcommander/claudette/internal/session"
)

// -- Projects -----------------------------------------------------------------

// ProjectRow is the JSON output shape for a single project.
type ProjectRow struct {
	EncodedName  string `json:"encoded_name"`
	OriginalPath string `json:"original_path"`
	SessionCount int    `json:"session_count"`
}

// ProjectsResult is the top-level JSON envelope for project listings.
type ProjectsResult struct {
	Projects []ProjectRow `json:"projects"`
	Total    int          `json:"total"`
}

// WriteProjectsText writes a human-readable project list.
func WriteProjectsText(w io.Writer, projects []session.Project) {
	if len(projects) == 0 {
		_, _ = fmt.Fprintln(w, "no projects found")
		return
	}
	for _, p := range projects {
		_, _ = fmt.Fprintf(w, "%-4d  %s\n", len(p.Sessions), p.OriginalPath)
		_, _ = fmt.Fprintf(w, "       (%s)\n", p.EncodedName)
	}
}

// WriteProjectsJSON writes projects as structured JSON.
func WriteProjectsJSON(w io.Writer, projects []session.Project) error {
	rows := make([]ProjectRow, len(projects))
	for i, p := range projects {
		rows[i] = ProjectRow{
			EncodedName:  p.EncodedName,
			OriginalPath: p.OriginalPath,
			SessionCount: len(p.Sessions),
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(ProjectsResult{Projects: rows, Total: len(rows)})
}

// -- Sessions -----------------------------------------------------------------

// SessionRow is the JSON output shape for a single session.
type SessionRow struct {
	UUID           string `json:"uuid"`
	TranscriptPath string `json:"transcript_path"`
	ModTime        string `json:"mtime"`
	MessageCount   int    `json:"message_count"`
}

// SessionsResult is the top-level JSON envelope for session listings.
type SessionsResult struct {
	Sessions []SessionRow `json:"sessions"`
	Total    int          `json:"total"`
}

// WriteSessionsText writes a human-readable session list.
func WriteSessionsText(w io.Writer, metas []session.SessionMeta) {
	if len(metas) == 0 {
		_, _ = fmt.Fprintln(w, "no sessions found")
		return
	}
	for _, m := range metas {
		_, _ = fmt.Fprintf(w, "%s  %s  msgs=%-4d  %s\n",
			m.UUID,
			m.ModTime.Format("2006-01-02 15:04"),
			m.MessageCount,
			m.TranscriptPath,
		)
	}
}

// WriteSessionsJSON writes sessions as structured JSON.
func WriteSessionsJSON(w io.Writer, metas []session.SessionMeta) error {
	rows := make([]SessionRow, len(metas))
	for i, m := range metas {
		rows[i] = SessionRow{
			UUID:           m.UUID,
			TranscriptPath: m.TranscriptPath,
			ModTime:        m.ModTime.Format("2006-01-02T15:04:05Z07:00"),
			MessageCount:   m.MessageCount,
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(SessionsResult{Sessions: rows, Total: len(rows)})
}

// -- Turns --------------------------------------------------------------------

const defaultTruncate = 200

// TurnRow is the JSON output shape for a single turn.
type TurnRow struct {
	Timestamp        string   `json:"ts"`
	UserContent      string   `json:"user"`
	AssistantSummary string   `json:"assistant,omitempty"`
	FilesRead        []string `json:"files_read,omitempty"`
	FilesEdited      []string `json:"files_edited,omitempty"`
	ToolsUsed        []string `json:"tools,omitempty"`
	Frustrated       bool     `json:"frustrated,omitempty"`
}

// TurnsResult is the top-level JSON envelope for turn listings.
type TurnsResult struct {
	Turns []TurnRow `json:"turns"`
	Total int       `json:"total"`
}

// truncate shortens s to maxLen chars, appending "…" when cut.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}

// WriteTurnsText writes a human-readable turn list.
func WriteTurnsText(w io.Writer, turns []session.Turn, full bool) {
	if len(turns) == 0 {
		_, _ = fmt.Fprintln(w, "no turns found")
		return
	}
	for i, t := range turns {
		user := t.UserContent
		asst := t.AssistantSummary
		if !full {
			user = truncate(user, defaultTruncate)
			asst = truncate(asst, defaultTruncate)
		}
		_, _ = fmt.Fprintf(w, "Turn %d  [%s]%s\n",
			i+1,
			t.Timestamp.Format("2006-01-02 15:04"),
			frustrationMark(t.Frustrated),
		)
		_, _ = fmt.Fprintf(w, "  user:      %s\n", user)
		if asst != "" {
			_, _ = fmt.Fprintf(w, "  assistant: %s\n", asst)
		}
		if len(t.ToolsUsed) > 0 {
			_, _ = fmt.Fprintf(w, "  tools:     %s\n", strings.Join(t.ToolsUsed, ", "))
		}
		_, _ = fmt.Fprintln(w)
	}
}

// frustrationMark returns " [!]" when frustrated, else "".
func frustrationMark(f bool) string {
	if f {
		return " [!]"
	}
	return ""
}

// WriteTurnsJSON writes turns as structured JSON.
func WriteTurnsJSON(w io.Writer, turns []session.Turn, full bool) error {
	rows := make([]TurnRow, len(turns))
	for i, t := range turns {
		user := t.UserContent
		asst := t.AssistantSummary
		if !full {
			user = truncate(user, defaultTruncate)
			asst = truncate(asst, defaultTruncate)
		}
		rows[i] = TurnRow{
			Timestamp:        t.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			UserContent:      user,
			AssistantSummary: asst,
			FilesRead:        t.FilesRead,
			FilesEdited:      t.FilesEdited,
			ToolsUsed:        t.ToolsUsed,
			Frustrated:       t.Frustrated,
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(TurnsResult{Turns: rows, Total: len(rows)})
}
