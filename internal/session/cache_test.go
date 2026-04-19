package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionCache_PutGet(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	ts := time.Date(2026, 4, 19, 8, 0, 0, 0, time.UTC)
	meta := SessionMeta{
		TranscriptPath: "/fake/path.jsonl",
		ModTime:        ts,
		Size:           1234,
		MessageCount:   10,
	}
	turns := []Turn{
		{Timestamp: ts, UserContent: "hello", AssistantSummary: "hi"},
	}

	c.Put("proj1", "/original/path", "sess-uuid", meta, turns)

	got, ok := c.Get("proj1", "sess-uuid")
	if !ok {
		t.Fatal("Get returned ok=false after Put")
	}
	if len(got) != 1 {
		t.Fatalf("Get returned %d turns, want 1", len(got))
	}
	if got[0].UserContent != "hello" {
		t.Errorf("UserContent = %q, want 'hello'", got[0].UserContent)
	}
}

func TestSessionCache_GetMiss(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	_, ok := c.Get("no-such-proj", "no-such-uuid")
	if ok {
		t.Error("Get on empty cache returned ok=true, want ok=false")
	}
}

func TestSessionCache_GetMissSession(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	ts := time.Now()
	c.Put("proj1", "/path", "uuid1", SessionMeta{ModTime: ts, Size: 100}, nil)

	_, ok := c.Get("proj1", "different-uuid")
	if ok {
		t.Error("Get for missing session UUID returned ok=true, want ok=false")
	}
}

func TestSessionCache_IsStale_FreshEntry(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	ts := time.Date(2026, 4, 19, 8, 0, 0, 0, time.UTC)
	meta := SessionMeta{ModTime: ts, Size: 5000, MessageCount: 20}
	c.Put("proj1", "/path", "uuid1", meta, nil)

	if c.IsStale("proj1", "uuid1", meta) {
		t.Error("IsStale = true for entry with identical mtime+size, want false")
	}
}

func TestSessionCache_IsStale_MtimeChanged(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	ts := time.Date(2026, 4, 19, 8, 0, 0, 0, time.UTC)
	meta := SessionMeta{ModTime: ts, Size: 5000}
	c.Put("proj1", "/path", "uuid1", meta, nil)

	newer := SessionMeta{ModTime: ts.Add(time.Second), Size: 5000}
	if !c.IsStale("proj1", "uuid1", newer) {
		t.Error("IsStale = false after mtime change, want true")
	}
}

func TestSessionCache_IsStale_SizeChanged(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	ts := time.Date(2026, 4, 19, 8, 0, 0, 0, time.UTC)
	meta := SessionMeta{ModTime: ts, Size: 5000}
	c.Put("proj1", "/path", "uuid1", meta, nil)

	grown := SessionMeta{ModTime: ts, Size: 6000}
	if !c.IsStale("proj1", "uuid1", grown) {
		t.Error("IsStale = false after size change, want true")
	}
}

func TestSessionCache_IsStale_MissingProject(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	meta := SessionMeta{ModTime: time.Now(), Size: 100}
	if !c.IsStale("no-proj", "no-uuid", meta) {
		t.Error("IsStale = false for missing project, want true")
	}
}

func TestSessionCache_IsStale_MissingSession(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	ts := time.Now()
	c.Put("proj1", "/path", "uuid1", SessionMeta{ModTime: ts, Size: 100}, nil)

	if !c.IsStale("proj1", "uuid-missing", SessionMeta{ModTime: ts, Size: 100}) {
		t.Error("IsStale = false for missing session UUID, want true")
	}
}

func TestSessionCache_PutOverwrite(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	ts := time.Date(2026, 4, 19, 8, 0, 0, 0, time.UTC)
	meta := SessionMeta{ModTime: ts, Size: 100}

	c.Put("proj1", "/path", "uuid1", meta, []Turn{{UserContent: "first"}})
	c.Put("proj1", "/path", "uuid1", meta, []Turn{{UserContent: "second"}})

	got, ok := c.Get("proj1", "uuid1")
	if !ok {
		t.Fatal("Get returned ok=false")
	}
	if len(got) != 1 || got[0].UserContent != "second" {
		t.Errorf("after overwrite, UserContent = %q, want 'second'", got[0].UserContent)
	}
}

func TestEmptyCache(t *testing.T) {
	t.Parallel()

	c := emptyCache()
	if c.Version != cacheVersion {
		t.Errorf("Version = %d, want %d", c.Version, cacheVersion)
	}
	if c.Projects == nil {
		t.Error("Projects is nil, want initialized map")
	}
}

func TestLoadSaveCache_Override(t *testing.T) {
	// Mutates package-level cachePath — must not run in parallel.
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	orig := cachePath
	cachePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { cachePath = orig })

	ctx := context.Background()

	// File absent — should return empty cache without error.
	c, err := LoadCache(ctx)
	if err != nil {
		t.Fatalf("LoadCache on missing file: %v", err)
	}
	if c.Version != cacheVersion {
		t.Errorf("Version = %d, want %d", c.Version, cacheVersion)
	}

	// Round-trip: put a session, save, reload, and verify.
	ts := time.Date(2026, 4, 19, 8, 0, 0, 0, time.UTC)
	meta := SessionMeta{ModTime: ts, Size: 42}
	c.Put("proj1", "/original", "uuid1", meta, []Turn{{UserContent: "hello"}})

	if err := SaveCache(ctx, c); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not written: %v", err)
	}

	c2, err := LoadCache(ctx)
	if err != nil {
		t.Fatalf("LoadCache after save: %v", err)
	}
	got, ok := c2.Get("proj1", "uuid1")
	if !ok {
		t.Fatal("Get returned ok=false after round-trip")
	}
	if len(got) != 1 || got[0].UserContent != "hello" {
		t.Errorf("round-trip UserContent = %q, want 'hello'", got[0].UserContent)
	}
}

func TestLoadCache_CorruptFile_Override(t *testing.T) {
	// Mutates package-level cachePath — must not run in parallel.
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(path, []byte("not valid json {{{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	orig := cachePath
	cachePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { cachePath = orig })

	c, err := LoadCache(context.Background())
	if err != nil {
		t.Fatalf("LoadCache on corrupt file should not error: %v", err)
	}
	// Corrupt input returns empty cache.
	if c.Version != cacheVersion {
		t.Errorf("Version = %d, want %d after corrupt load", c.Version, cacheVersion)
	}
	if len(c.Projects) != 0 {
		t.Errorf("Projects count = %d, want 0 after corrupt load", len(c.Projects))
	}
}

func TestLoadCache_WrongVersion_Override(t *testing.T) {
	// Mutates package-level cachePath — must not run in parallel.
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	// Write a cache with a version number that doesn't match cacheVersion.
	if err := os.WriteFile(path, []byte(`{"version":9999,"projects":{}}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	orig := cachePath
	cachePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { cachePath = orig })

	c, err := LoadCache(context.Background())
	if err != nil {
		t.Fatalf("LoadCache on wrong version should not error: %v", err)
	}
	if c.Version != cacheVersion {
		t.Errorf("Version = %d, want %d (fresh empty cache)", c.Version, cacheVersion)
	}
}
