package audit

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecorder_AppendsLine(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir)
	r.Record(Entry{
		Module:  "issue_to_jira",
		Repo:    "foo/bar",
		Issue:   42,
		Actor:   "alice",
		JiraKey: "ABC-1",
		Status:  "success",
	})

	data, err := os.ReadFile(r.Path("foo/bar"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	line := strings.TrimSpace(string(data))
	if !strings.Contains(line, `"jira_key":"ABC-1"`) {
		t.Errorf("missing jira_key in: %s", line)
	}
	if !strings.Contains(line, `"status":"success"`) {
		t.Errorf("missing status in: %s", line)
	}
	// Must be valid JSON.
	var got Entry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got.Timestamp.IsZero() {
		t.Error("expected timestamp to be auto-filled")
	}
}

func TestRecorder_MultipleEntries_AreAllAppended(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir)
	for i := 1; i <= 5; i++ {
		r.Record(Entry{Repo: "foo/bar", Issue: i, Status: "success"})
	}

	data, _ := os.ReadFile(r.Path("foo/bar"))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
}

func TestRecorder_DefaultStatusFilledIn(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir)
	r.Record(Entry{Repo: "x/y"})

	data, _ := os.ReadFile(r.Path("x/y"))
	if !strings.Contains(string(data), `"status":"success"`) {
		t.Errorf("expected status filled to 'success', got: %s", data)
	}
}

func TestRecorder_NilRecorderIsNoOp(t *testing.T) {
	var r *Recorder
	// must not panic
	r.Record(Entry{Repo: "z/q"})
}

func TestRecorder_IsConcurrencySafe(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir)
	const n = 50

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			r.Record(Entry{
				Repo:      "foo/bar",
				Issue:     i,
				Timestamp: time.Now(),
			})
		}(i)
	}
	wg.Wait()

	data, _ := os.ReadFile(r.Path("foo/bar"))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != n {
		t.Errorf("expected %d lines, got %d (concurrent writes were lost or interleaved)", n, len(lines))
	}
	// Every line must be valid JSON — no torn lines.
	for i, ln := range lines {
		var e Entry
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Errorf("line %d not valid JSON: %v\n%s", i, err, ln)
		}
	}
}

func TestRepoFilename(t *testing.T) {
	cases := map[string]string{
		"foo/bar":     "foo_bar.jsonl",
		"":            "_unknown.jsonl",
		"complex/r-1": "complex_r-1.jsonl",
	}
	for in, want := range cases {
		if got := repoFilename(in); got != want {
			t.Errorf("repoFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
