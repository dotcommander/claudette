package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/dotcommander/claudette/internal/config"
	"github.com/dotcommander/claudette/internal/fileutil"
)

// cacheVersion is bumped whenever the on-disk cache shape changes incompatibly.
const cacheVersion = 1

// sessionCache is the on-disk representation of ~/.config/claudette/sessions.json.
type sessionCache struct {
	Version  int                      `json:"version"`
	Projects map[string]cachedProject `json:"projects"`
}

// cachedProject is one project's worth of cached session data.
type cachedProject struct {
	OriginalPath string                   `json:"original_path"`
	Sessions     map[string]cachedSession `json:"sessions"`
}

// cachedSession is the persisted data for a single session UUID.
// The cache key is the session UUID string (directory name).
type cachedSession struct {
	MTime        time.Time `json:"mtime"`
	Size         int64     `json:"size"`
	MessageCount int       `json:"message_count"`
	TurnCount    int       `json:"turn_count"`
	Turns        []Turn    `json:"turns"`
}

// cachePath returns ~/.config/claudette/sessions.json.
// Declared as a var so tests can override it with t.Cleanup to restore.
var cachePath = func() (string, error) {
	return config.ConfigFilePath("sessions.json")
}

// LoadCache reads the session cache from disk.
// Returns an empty cache (no error) when the file is absent or has an incompatible version.
func LoadCache(ctx context.Context) (sessionCache, error) {
	_ = ctx // reserved for future timeout propagation
	path, err := cachePath()
	if err != nil {
		return emptyCache(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return emptyCache(), nil
		}
		return emptyCache(), fmt.Errorf("read session cache: %w", err)
	}
	var c sessionCache
	if err := json.Unmarshal(data, &c); err != nil {
		return emptyCache(), nil // corrupt — start fresh
	}
	if c.Version != cacheVersion {
		return emptyCache(), nil
	}
	return c, nil
}

// SaveCache persists the session cache atomically via fileutil.WriteJSONFile.
func SaveCache(ctx context.Context, c sessionCache) error {
	_ = ctx
	path, err := cachePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session cache: %w", err)
	}
	return fileutil.WriteJSONFile(path, data)
}

// emptyCache returns a zero-value cache ready for population.
func emptyCache() sessionCache {
	return sessionCache{
		Version:  cacheVersion,
		Projects: make(map[string]cachedProject),
	}
}

// IsStale reports whether the cached entry for uuid is stale relative to meta.
// A session is stale when mtime or size differs from the cached values.
func (c sessionCache) IsStale(encodedProject, uuid string, meta SessionMeta) bool {
	proj, ok := c.Projects[encodedProject]
	if !ok {
		return true
	}
	entry, ok := proj.Sessions[uuid]
	if !ok {
		return true
	}
	return entry.MTime != meta.ModTime || entry.Size != meta.Size
}

// Put stores turns for a session into the cache. It creates the project bucket
// if it doesn't exist.
func (c *sessionCache) Put(encodedProject, originalPath, uuid string, meta SessionMeta, turns []Turn) {
	if c.Projects == nil {
		c.Projects = make(map[string]cachedProject)
	}
	proj, ok := c.Projects[encodedProject]
	if !ok {
		proj = cachedProject{
			OriginalPath: originalPath,
			Sessions:     make(map[string]cachedSession),
		}
	}
	if proj.Sessions == nil {
		proj.Sessions = make(map[string]cachedSession)
	}
	proj.Sessions[uuid] = cachedSession{
		MTime:        meta.ModTime,
		Size:         meta.Size,
		MessageCount: meta.MessageCount,
		TurnCount:    len(turns),
		Turns:        turns,
	}
	c.Projects[encodedProject] = proj
}

// Get retrieves cached turns for a session. Returns (turns, true) on a hit.
func (c sessionCache) Get(encodedProject, uuid string) ([]Turn, bool) {
	proj, ok := c.Projects[encodedProject]
	if !ok {
		return nil, false
	}
	entry, ok := proj.Sessions[uuid]
	if !ok {
		return nil, false
	}
	return entry.Turns, true
}
