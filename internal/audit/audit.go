// Package audit records every action a Roster module takes against an
// external system. Records are appended to JSONL files under
// $HOME/.roster/audit/<repo>.jsonl, one line per action.
//
// The format is intentionally simple — line-oriented JSON, monotonically
// growing — so operators can `tail -f` it during incidents and reasonable
// archival tools (rsync, log shippers, S3 sync) handle it without ceremony.
//
// Audit is for *debugging by the operator*, not for compliance attestation.
// Don't put secrets in the entries.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry is a single audit record. Fields are intentionally optional so a
// partial-success path (e.g. "Jira created but GitHub comment failed") can
// still produce a meaningful record.
type Entry struct {
	Timestamp time.Time `json:"ts"`
	Module    string    `json:"module"`           // e.g. "issue_to_jira"
	Repo      string    `json:"repo"`             // owner/name
	Issue     int       `json:"issue,omitempty"`  // GitHub issue number
	Actor     string    `json:"actor,omitempty"`  // GitHub user that triggered the event
	EventID   string    `json:"event_id,omitempty"`
	EventType string    `json:"event_type,omitempty"`

	// AI usage flag — true if Claude produced the fields used.
	AIExtracted bool   `json:"ai_extracted,omitempty"`
	Model       string `json:"model,omitempty"`

	// Token usage from the Claude API response. Filled by modules that
	// invoke Claude. Omitted when no AI call was made.
	InputTokens       int     `json:"in_tok,omitempty"`
	OutputTokens      int     `json:"out_tok,omitempty"`
	CacheCreateTokens int     `json:"cache_create_tok,omitempty"`
	CacheReadTokens   int     `json:"cache_read_tok,omitempty"`
	CostUSD           float64 `json:"cost_usd,omitempty"`

	// Outcome.
	JiraKey string `json:"jira_key,omitempty"`
	JiraURL string `json:"jira_url,omitempty"`

	// Status: "success" / "partial" / "error".
	Status string `json:"status"`
	// Error message if Status != "success".
	Error string `json:"error,omitempty"`

	// Duration of the whole module invocation.
	DurationMS int64 `json:"duration_ms,omitempty"`
}

// Recorder appends Entry records to a JSONL file. It is safe for concurrent use.
type Recorder struct {
	dir string
	mu  sync.Mutex
}

// NewRecorder creates a Recorder rooted at dir/audit. The directory is
// created lazily on first write.
func NewRecorder(baseDir string) *Recorder {
	return &Recorder{dir: filepath.Join(baseDir, "audit")}
}

// Record appends one entry. It never returns an error to the caller — audit
// must not block business actions; failures are written to stderr.
func (r *Recorder) Record(e Entry) {
	if r == nil {
		return
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.Status == "" {
		e.Status = "success"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[audit] mkdir: %v\n", err)
		return
	}
	path := filepath.Join(r.dir, repoFilename(e.Repo))

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[audit] open %s: %v\n", path, err)
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(e); err != nil {
		fmt.Fprintf(os.Stderr, "[audit] encode: %v\n", err)
	}
}

// Path returns the path of the audit file for a given repo. Useful in tests.
func (r *Recorder) Path(repo string) string {
	return filepath.Join(r.dir, repoFilename(repo))
}

// repoFilename turns "owner/name" into "owner_name.jsonl". Falls back to
// "_unknown.jsonl" if repo is empty.
func repoFilename(repo string) string {
	if repo == "" {
		return "_unknown.jsonl"
	}
	return strings.ReplaceAll(repo, "/", "_") + ".jsonl"
}
