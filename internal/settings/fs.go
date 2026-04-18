package settings

import (
	"github.com/dotcommander/claudette/internal/fileutil"
)

func writeJSONFile(path string, data []byte) error {
	return fileutil.WriteJSONFile(path, data)
}
