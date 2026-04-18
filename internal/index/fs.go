package index

import (
	"os"

	"github.com/dotcommander/claudette/internal/fileutil"
)

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	return fileutil.AtomicWriteFile(path, data, perm)
}

func writeJSONFile(path string, data []byte) error {
	return fileutil.WriteJSONFile(path, data)
}
