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
	Type     EntryType `json:"type"`
	Name     string    `json:"name"`      // filename stem or frontmatter name
	Title    string    `json:"title"`     // # heading or frontmatter description
	Category string    `json:"category"`  // parent dir (go, openai) or type name
	FilePath string    `json:"file_path"` // absolute path to source file
	Keywords []string  `json:"keywords"`  // pre-extracted lowercase tokens
}
