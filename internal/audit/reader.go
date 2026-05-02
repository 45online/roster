package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReadAll returns every Entry in the audit file for repo, in append order.
// Returns (nil, nil) if the file doesn't exist.
func (r *Recorder) ReadAll(repo string) ([]Entry, error) {
	return readEntries(r.Path(repo), nil)
}

// ReadSince returns entries written at or after `since`. Cheaper than
// ReadAll for status displays — JSONL is parsed line by line and lines
// older than `since` are skipped without allocating an Entry struct.
func (r *Recorder) ReadSince(repo string, since time.Time) ([]Entry, error) {
	return readEntries(r.Path(repo), &since)
}

// ListRepos scans the audit dir and returns the repo names (owner/name)
// for which a JSONL file exists. Order is unspecified.
func (r *Recorder) ListRepos() ([]string, error) {
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".jsonl")
		// Inverse of repoFilename: first underscore separates owner from name.
		i := strings.IndexByte(base, '_')
		if i <= 0 {
			out = append(out, base) // best-effort
			continue
		}
		out = append(out, base[:i]+"/"+base[i+1:])
	}
	return out, nil
}

// Summary aggregates audit entries for status display.
type Summary struct {
	Repo            string
	Total           int
	Success         int
	Partial         int
	Errors          int
	Skipped         int
	ByModule        map[string]int
	LatestEntry     *Entry
	LatestErrorMsg  string
	LatestErrorTime time.Time
}

// Summarize collapses a slice of entries into a Summary.
func Summarize(repo string, entries []Entry) Summary {
	s := Summary{Repo: repo, ByModule: map[string]int{}}
	for i := range entries {
		e := entries[i]
		s.Total++
		switch e.Status {
		case "success":
			s.Success++
		case "partial":
			s.Partial++
		case "error":
			s.Errors++
			if e.Timestamp.After(s.LatestErrorTime) {
				s.LatestErrorTime = e.Timestamp
				s.LatestErrorMsg = e.Error
			}
		case "skipped":
			s.Skipped++
		}
		s.ByModule[e.Module]++
		if s.LatestEntry == nil || e.Timestamp.After(s.LatestEntry.Timestamp) {
			cp := e
			s.LatestEntry = &cp
		}
	}
	return s
}

// readEntries decodes a JSONL file. If sinceFilter is non-nil, entries
// strictly older than it are dropped before being returned (cheap by-ts
// filtering for "last 24h" displays).
func readEntries(path string, sinceFilter *time.Time) ([]Entry, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	defer f.Close()

	out := make([]Entry, 0, 64)
	sc := bufio.NewScanner(f)
	// Audit lines may contain arbitrary user text — bump the buffer.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			// Tolerate one corrupt line — keep reading rather than aborting
			// the whole status display.
			continue
		}
		if sinceFilter != nil && e.Timestamp.Before(*sinceFilter) {
			continue
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("audit: read %s: %w", path, err)
	}
	return out, nil
}

// EnsureDir is a small helper so callers can pre-create the audit dir
// (e.g. before listing it). Useful to keep the empty-state behaviour
// consistent in the status command.
func (r *Recorder) EnsureDir() error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	return nil
}

// Dir returns the audit directory path; useful for `roster logs` resolving
// the file path independently.
func (r *Recorder) Dir() string {
	return r.dir
}

// PathForRepo is an exported alias of Path for callers outside the
// package who don't want to deal with the unexported repoFilename.
func (r *Recorder) PathForRepo(repo string) string {
	return filepath.Join(r.dir, repoFilename(repo))
}
