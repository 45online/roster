// Package poller implements GitHub event polling for Roster's modules.
// It keeps a per-repository cursor of the last processed event ID, plus
// the last ETag for conditional fetches.
package poller

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cursor records the last processed event for a repository.
// It is persisted as JSON at <baseDir>/cursors/<owner>_<repo>.json.
type Cursor struct {
	Repo         string    `json:"repo"`
	LastEventID  string    `json:"last_event_id"`
	LastETag     string    `json:"last_etag"`
	LastPolledAt time.Time `json:"last_polled_at"`

	path string `json:"-"`
}

// LoadCursor reads (or creates) the cursor file for the given repo, rooted
// at baseDir (typically ~/.roster).
func LoadCursor(baseDir, repo string) (*Cursor, error) {
	dir := filepath.Join(baseDir, "cursors")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("poller: mkdir cursors: %w", err)
	}
	fname := strings.ReplaceAll(repo, "/", "_") + ".json"
	path := filepath.Join(dir, fname)

	c := &Cursor{Repo: repo, path: path}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("poller: read cursor %s: %w", path, err)
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("poller: decode cursor %s: %w", path, err)
	}
	c.path = path
	return c, nil
}

// Save persists the cursor to disk atomically (write-temp + rename).
func (c *Cursor) Save() error {
	c.LastPolledAt = time.Now().UTC()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}
