package index

// EntryType identifies the source of a searchable entry.
type EntryType string

const (
	TypeKB      EntryType = "kb"
	TypeSkill   EntryType = "skill"
	TypeAgent   EntryType = "agent"
	TypeCommand EntryType = "command"
)

// Entry is a single searchable item in the index.
type Entry struct {
	Type     EntryType      `json:"type"`
	Name     string         `json:"name"`
	Title    string         `json:"title"`
	Category string         `json:"category"`
	FilePath string         `json:"file_path"`
	Keywords map[string]int `json:"keywords"`         // word -> field weight (name=3, title=2, tag=2, desc=1)
	Bigrams  []string       `json:"bigrams,omitzero"` // consecutive word pairs from title
}
