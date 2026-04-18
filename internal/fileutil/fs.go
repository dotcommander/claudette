package fileutil

import (
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to path via temp-file-then-rename.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) (retErr error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".claudette-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if retErr != nil {
			//nolint:errcheck // best-effort cleanup; original error takes precedence
			tmp.Close()
			//nolint:errcheck // best-effort cleanup; original error takes precedence
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// WriteJSONFile creates parent directories and atomically writes data to path.
func WriteJSONFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // 0o755 is correct for ~/.config subdirs
		return err
	}
	return AtomicWriteFile(path, data, 0o644)
}
