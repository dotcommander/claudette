package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyType(t *testing.T) {
	t.Parallel()

	// Build a temp source root: sourceDir/<dirName>/entry.md
	root := t.TempDir()

	cases := []struct {
		dirName string
		want    EntryType
	}{
		{"kb", TypeKB},
		{"skills", TypeSkill},
		{"agents", TypeAgent},
		{"commands", TypeCommand},
		{"plugin-commands", TypeCommand},
		{"dev-commands", TypeCommand},
		{"unknown", TypeKB},
		{"", TypeKB}, // file directly in sourceDir — base of sourceDir checked
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.dirName, func(t *testing.T) {
			t.Parallel()

			var sourceDir, filePath string
			if tc.dirName == "" {
				// File sits directly in sourceDir; sourceDir basename != any known type.
				sourceDir = filepath.Join(root, "direct-"+tc.dirName+"src")
				if err := os.MkdirAll(sourceDir, 0o755); err != nil {
					t.Fatalf("mkdir sourceDir: %v", err)
				}
				filePath = filepath.Join(sourceDir, "entry.md")
			} else {
				sourceDir = filepath.Join(root, "src-for-"+tc.dirName)
				subDir := filepath.Join(sourceDir, tc.dirName)
				if err := os.MkdirAll(subDir, 0o755); err != nil {
					t.Fatalf("mkdir subDir: %v", err)
				}
				filePath = filepath.Join(subDir, "entry.md")
			}

			if err := os.WriteFile(filePath, []byte("# Entry\n"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			got := classifyType(filePath, sourceDir)
			if got != tc.want {
				t.Errorf("classifyType(%q, %q) = %q; want %q", filePath, sourceDir, got, tc.want)
			}
		})
	}
}
