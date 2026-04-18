package index

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/dotcommander/claudette/internal/config"
	"gopkg.in/yaml.v3"
)

// aliasOverridesPath returns ~/.config/claudette/aliases.yaml.
func aliasOverridesPath() (string, error) {
	return config.ConfigFilePath("aliases.yaml")
}

// aliasOverridesFile is the on-disk YAML structure.
type aliasOverridesFile struct {
	Aliases map[string][]string `yaml:"aliases"`
}

// loadAliasOverrides reads ~/.config/claudette/aliases.yaml and returns the
// slug→aliases map and the file's mtime. Returns (nil, zero, nil) when the
// file is absent. Returns (nil, zero, err) only for unexpected I/O failures;
// malformed YAML is logged to stderr and treated as empty (no crash).
func loadAliasOverrides(path string) (map[string][]string, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, time.Time{}, nil
		}
		return nil, time.Time{}, fmt.Errorf("stat aliases.yaml: %w", err)
	}
	mtime := info.ModTime()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read aliases.yaml: %w", err)
	}

	var f aliasOverridesFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		fmt.Fprintf(os.Stderr, "claudette: warning: malformed aliases.yaml: %v\n", err)
		return nil, mtime, nil
	}

	return f.Aliases, mtime, nil
}

// applyAliasOverrides merges user-defined aliases into entries by slug.
// The slug is entry.Name (set from frontmatter or file basename). Tokens from
// each matched alias phrase are inserted into entry.Keywords at weight 1 via
// makeAdder (which keeps the highest weight seen and deduplicates).
func applyAliasOverrides(entries []Entry, overrides map[string][]string) {
	if len(overrides) == 0 {
		return
	}
	for i := range entries {
		phrases, ok := overrides[entries[i].Name]
		if !ok {
			continue
		}
		add := makeAdder(entries[i].Keywords)
		for _, phrase := range phrases {
			for _, tok := range splitWords(phrase) {
				add(tok, 1)
			}
		}
	}
}
