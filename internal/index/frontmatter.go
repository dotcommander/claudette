package index

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds parsed YAML header from component .md files.
type Frontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

var fmDelimiter = []byte("---\n")

// ParseFrontmatter extracts YAML between --- delimiters.
// Returns zero Frontmatter if no valid frontmatter found.
func ParseFrontmatter(content []byte) (Frontmatter, error) {
	if !bytes.HasPrefix(content, fmDelimiter) {
		return Frontmatter{}, nil
	}

	rest := content[len(fmDelimiter):]
	end := bytes.Index(rest, fmDelimiter)
	if end < 0 {
		return Frontmatter{}, nil
	}

	var fm Frontmatter
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		return Frontmatter{}, err
	}
	return fm, nil
}
