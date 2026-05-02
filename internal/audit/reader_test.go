package audit

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRecorder_ReadAllAndReadSince(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir)

	old := time.Now().UTC().Add(-2 * time.Hour)
	recent := time.Now().UTC().Add(-5 * time.Minute)

	r.Record(Entry{Repo: "foo/bar", Timestamp: old, Module: "issue_to_jira", Status: "success"})
	r.Record(Entry{Repo: "foo/bar", Timestamp: recent, Module: "pr_review", Status: "success"})
	r.Record(Entry{Repo: "foo/bar", Timestamp: recent, Module: "pr_review", Status: "error", Error: "boom"})

	all, err := r.ReadAll("foo/bar")
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ReadAll: got %d, want 3", len(all))
	}

	since := time.Now().UTC().Add(-1 * time.Hour)
	recentOnly, err := r.ReadSince("foo/bar", since)
	if err != nil {
		t.Fatalf("ReadSince: %v", err)
	}
	if len(recentOnly) != 2 {
		t.Errorf("ReadSince: got %d, want 2", len(recentOnly))
	}
}

func TestRecorder_ReadAll_MissingFile(t *testing.T) {
	r := NewRecorder(t.TempDir())
	got, err := r.ReadAll("nope/none")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing file, got %d entries", len(got))
	}
}

func TestRecorder_ListRepos(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir)
	r.Record(Entry{Repo: "foo/bar"})
	r.Record(Entry{Repo: "baz/qux"})

	repos, err := r.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repos, want 2: %v", len(repos), repos)
	}
	// Order is unspecified — check membership.
	seen := map[string]bool{}
	for _, r := range repos {
		seen[r] = true
	}
	if !seen["foo/bar"] || !seen["baz/qux"] {
		t.Errorf("repos missing: %v", repos)
	}
}

func TestSummarize_CountsByStatus(t *testing.T) {
	now := time.Now().UTC()
	entries := []Entry{
		{Module: "a", Status: "success", Timestamp: now.Add(-10 * time.Minute)},
		{Module: "a", Status: "success", Timestamp: now.Add(-5 * time.Minute)},
		{Module: "b", Status: "error", Error: "oops", Timestamp: now.Add(-3 * time.Minute)},
		{Module: "b", Status: "partial", Timestamp: now.Add(-2 * time.Minute)},
		{Module: "c", Status: "skipped", Timestamp: now.Add(-1 * time.Minute)},
	}
	s := Summarize("foo/bar", entries)

	if s.Total != 5 {
		t.Errorf("Total = %d, want 5", s.Total)
	}
	if s.Success != 2 || s.Errors != 1 || s.Partial != 1 || s.Skipped != 1 {
		t.Errorf("status breakdown wrong: %+v", s)
	}
	if s.ByModule["a"] != 2 || s.ByModule["b"] != 2 || s.ByModule["c"] != 1 {
		t.Errorf("ByModule wrong: %v", s.ByModule)
	}
	if s.LatestErrorMsg != "oops" {
		t.Errorf("LatestErrorMsg = %q, want oops", s.LatestErrorMsg)
	}
	// LatestEntry must be the most recent overall.
	if s.LatestEntry == nil || s.LatestEntry.Module != "c" {
		t.Errorf("LatestEntry = %+v, want module=c", s.LatestEntry)
	}
}

func TestSummarize_Empty(t *testing.T) {
	s := Summarize("x/y", nil)
	if s.Total != 0 || s.LatestEntry != nil {
		t.Errorf("empty summary should be zero-valued, got %+v", s)
	}
}

func TestPath_StaysUnderDir(t *testing.T) {
	r := NewRecorder("/base")
	want := filepath.Join("/base", "audit", "foo_bar.jsonl")
	if got := r.Path("foo/bar"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}
