package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config holds claudette's runtime configuration.
type Config struct {
	SourceDirs    []string `json:"source_dirs,omitempty"`
	ContextHeader string   `json:"context_header,omitempty"`
}

// defaultContextHeader is the triage instruction injected between the
// <related_skills_knowledge> open and close tags. Users may override it by
// setting "context_header" in ~/.config/claudette/config.json. The tags
// themselves are protocol markers and are not user-configurable.
const defaultContextHeader = "Scan first 10 lines of each file. Only read full files that are clearly relevant."

// DefaultContextHeader returns the built-in triage instruction written to
// fresh configs. Callers that need the same default at runtime without a
// loaded Config should use this rather than duplicating the string.
func DefaultContextHeader() string {
	return defaultContextHeader
}

// ContextHeader returns the configured context header, or the built-in
// default when the config omits it. Callers should use this accessor rather
// than reading Config.ContextHeader directly so the default is honoured
// consistently.
func (c Config) ContextHeaderOrDefault() string {
	if strings.TrimSpace(c.ContextHeader) == "" {
		return defaultContextHeader
	}
	return c.ContextHeader
}

// ConfigFilePath returns ~/.config/claudette/<name>.
// Hardcodes ~/.config/ for consistent cross-platform paths.
func ConfigFilePath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "claudette", name), nil
}

// ConfigPath returns the path to claudette's config file.
func ConfigPath() (string, error) {
	return ConfigFilePath("config.json")
}

// DefaultSourceDirs returns the built-in source directories under ~/.claude/.
func DefaultSourceDirs() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return []string{
		filepath.Join(home, ".claude", "kb"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".claude", "agents"),
		filepath.Join(home, ".claude", "commands"),
	}, nil
}

// LoadConfig reads config from disk. Returns a zero-value Config if file missing.
func LoadConfig() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// SaveConfig writes config to disk via atomic temp-file-then-rename.
func SaveConfig(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONFile(path, data)
}
