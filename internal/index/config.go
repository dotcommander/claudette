package index

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// atomicWriteFile writes data to path via temp-file-then-rename.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".claudette-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// Config holds claudette's runtime configuration.
type Config struct {
	SourceDirs []string `json:"source_dirs,omitempty"`
}

// configFilePath returns ~/.config/claudette/<name>.
// Hardcodes ~/.config/ for consistent cross-platform paths.
func configFilePath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "claudette", name), nil
}

// ConfigPath returns the path to claudette's config file.
func ConfigPath() (string, error) {
	return configFilePath("config.json")
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

// writeJSONFile creates parent directories and atomically writes data to path.
func writeJSONFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0o644)
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
