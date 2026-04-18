package fileutil_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotcommander/claudette/internal/fileutil"
)

func TestAtomicWriteFile_WritesContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	want := []byte("hello, world")

	if err := fileutil.AtomicWriteFile(path, want, 0o644); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestAtomicWriteFile_Overwrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	if err := fileutil.AtomicWriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatalf("first write: %v", err)
	}

	want := []byte("second")
	if err := fileutil.AtomicWriteFile(path, want, 0o644); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestAtomicWriteFile_NoTempLeftover(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	if err := fileutil.AtomicWriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "out.txt" {
			t.Errorf("unexpected file left in dir: %s", e.Name())
		}
	}
}

func TestWriteJSONFile_RoundTrip(t *testing.T) {
	t.Parallel()

	type payload struct {
		A int    `json:"a"`
		B string `json:"b"`
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "data.json")

	in := payload{A: 42, B: "hello"}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	if err := fileutil.WriteJSONFile(path, data); err != nil {
		t.Fatalf("WriteJSONFile: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var out payload
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if out != in {
		t.Errorf("round-trip = %+v, want %+v", out, in)
	}
}

func TestWriteJSONFile_CreatesParentDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// nested path that does not exist yet
	path := filepath.Join(dir, "a", "b", "c", "data.json")

	if err := fileutil.WriteJSONFile(path, []byte(`{}`)); err != nil {
		t.Fatalf("WriteJSONFile: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist after WriteJSONFile: %v", err)
	}
}
