package index

import "time"

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
	Type             EntryType      `json:"type"`
	Name             string         `json:"name"`
	Title            string         `json:"title"`
	Desc             string         `json:"desc,omitzero"` // one-liner from frontmatter description
	Category         string         `json:"category"`
	FilePath         string         `json:"file_path"`
	FileMtime        time.Time      `json:"file_mtime"`                  // mtime of the source .md file at scan time
	Keywords         map[string]int `json:"keywords"`                    // word -> field weight (name=3, title=2, tag=2, desc=1)
	Bigrams          []string       `json:"bigrams,omitzero"`            // consecutive word pairs from title
	HitCount         int            `json:"hit_count,omitzero"`          // raw aggregated usage count (diagnostics only)
	HitCountDecayed  float64        `json:"hit_count_decayed,omitempty"` // exponentially-decayed hit count (scorer uses this)
	SuggestedAliases []string       `json:"suggested_aliases,omitempty"` // co-occurrence-derived alias candidates (populated at rebuild; scored at weight 1)
	Source           string         `json:"source,omitzero"`             // provenance: collapsed from frontmatter source_file > source_task > source
}
