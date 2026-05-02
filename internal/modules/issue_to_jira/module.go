// Package issue_to_jira implements Module A: a one-way sync from a GitHub
// issue to a Jira ticket. The module is event-shaped — given a (repo, issue
// number) it does the full round-trip: fetch issue, build Jira fields,
// create the ticket, post a back-link comment on the GitHub issue.
package issue_to_jira

import (
	"context"
	"fmt"
	"strings"
	"time"

	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/adapters/jira"
	"github.com/45online/roster/internal/audit"
)

// Config holds Module A's per-project configuration.
type Config struct {
	// JiraProject is the destination Jira project key (e.g. "ROSTER").
	JiraProject string
	// DefaultIssueType is used when no label-based override matches.
	// Defaults to "Task" if empty.
	DefaultIssueType string
	// PriorityMapping maps a GitHub label name to a Jira priority name.
	// Example: {"P0": "Highest", "P1": "High", "bug": "High"}.
	PriorityMapping map[string]string
	// LabelToIssueType maps a GitHub label to a Jira issue type override.
	// Example: {"bug": "Bug", "feature": "Story"}.
	LabelToIssueType map[string]string
}

// Module is the Issue → Jira sync module.
type Module struct {
	gh        *gh.Client
	jira      *jira.Client
	cfg       Config
	extractor *Extractor      // optional Claude-powered field extractor
	audit     *audit.Recorder // optional audit recorder
}

// New constructs a Module. The caller supplies pre-configured adapters.
func New(github *gh.Client, j *jira.Client, cfg Config) *Module {
	return &Module{gh: github, jira: j, cfg: cfg}
}

// WithExtractor enables Claude-powered field extraction. When set, the
// module asks Claude to derive summary / issue_type / priority / component
// from the issue text; mechanical label-based mapping remains the fallback
// path on extraction failure.
func (m *Module) WithExtractor(e *Extractor) *Module {
	m.extractor = e
	return m
}

// WithAudit attaches an audit recorder. Each SyncIssue invocation will
// append one entry capturing inputs, AI usage, outcome, and duration.
func (m *Module) WithAudit(r *audit.Recorder) *Module {
	m.audit = r
	return m
}

// Result is what SyncIssue returns on success.
type Result struct {
	JiraKey string
	JiraURL string
	// AIExtracted is true when Claude's extractor produced the fields used.
	AIExtracted bool
}

// SyncIssue fetches the given GitHub issue, creates a corresponding Jira
// ticket, and posts a back-link comment on the issue. It is idempotency-naive
// — calling it twice on the same issue will create two Jira tickets — so the
// caller is responsible for filtering events appropriately (e.g. only on
// issues.opened).
func (m *Module) SyncIssue(ctx context.Context, repo string, number int) (*Result, error) {
	started := time.Now()
	entry := audit.Entry{
		Module: "issue_to_jira",
		Repo:   repo,
		Issue:  number,
	}

	issue, err := m.gh.GetIssue(ctx, repo, number)
	if err != nil {
		entry.Status = "error"
		entry.Error = fmt.Sprintf("fetch github issue: %v", err)
		entry.DurationMS = time.Since(started).Milliseconds()
		m.audit.Record(entry)
		return nil, fmt.Errorf("fetch github issue: %w", err)
	}
	entry.Actor = issue.User.Login

	summary, issueType, priority, component, aiUsed := m.deriveFields(ctx, issue)
	entry.AIExtracted = aiUsed
	if aiUsed && m.extractor != nil {
		entry.Model = m.extractor.model
	}

	createReq := jira.CreateIssueRequest{
		Project:     m.cfg.JiraProject,
		Summary:     summary,
		Description: buildDescription(issue, repo, component),
		IssueType:   issueType,
		Priority:    priority,
	}

	created, err := m.jira.CreateIssue(ctx, createReq)
	if err != nil {
		entry.Status = "error"
		entry.Error = fmt.Sprintf("create jira issue: %v", err)
		entry.DurationMS = time.Since(started).Milliseconds()
		m.audit.Record(entry)
		return nil, fmt.Errorf("create jira issue: %w", err)
	}
	entry.JiraKey = created.Key
	entry.JiraURL = created.URL

	comment := fmt.Sprintf("📋 Tracking in Jira: **[%s](%s)**", created.Key, created.URL)
	if err := m.gh.CreateComment(ctx, repo, number, comment); err != nil {
		// The Jira ticket exists; we report partial success.
		entry.Status = "partial"
		entry.Error = fmt.Sprintf("post github back-link comment: %v", err)
		entry.DurationMS = time.Since(started).Milliseconds()
		m.audit.Record(entry)
		return &Result{JiraKey: created.Key, JiraURL: created.URL, AIExtracted: aiUsed},
			fmt.Errorf("post github back-link comment: %w", err)
	}

	entry.Status = "success"
	entry.DurationMS = time.Since(started).Milliseconds()
	m.audit.Record(entry)
	return &Result{JiraKey: created.Key, JiraURL: created.URL, AIExtracted: aiUsed}, nil
}

// deriveFields chooses the values for summary / issue type / priority /
// component, preferring the AI extractor when available and falling back
// to mechanical mapping for any field the extractor omits or fails on.
func (m *Module) deriveFields(ctx context.Context, issue *gh.Issue) (
	summary, issueType, priority, component string, aiUsed bool,
) {
	if m.extractor != nil {
		if f, err := m.extractor.Extract(ctx, issue); err == nil && f != nil {
			summary = strings.TrimSpace(f.Summary)
			issueType = f.IssueType
			priority = f.Priority
			component = f.Component
			aiUsed = true
		}
	}
	if summary == "" {
		summary = truncate(issue.Title, 240)
	}
	if issueType == "" {
		issueType = m.resolveIssueType(issue)
	}
	if priority == "" {
		priority = m.resolvePriority(issue)
	}
	return
}

// resolveIssueType picks a Jira issue type by:
//  1. first label that matches Config.LabelToIssueType, else
//  2. Config.DefaultIssueType, else
//  3. "Task".
func (m *Module) resolveIssueType(issue *gh.Issue) string {
	for _, l := range issue.Labels {
		if t, ok := m.cfg.LabelToIssueType[l.Name]; ok {
			return t
		}
	}
	if m.cfg.DefaultIssueType != "" {
		return m.cfg.DefaultIssueType
	}
	return "Task"
}

// resolvePriority returns the Jira priority for the first matching label, or
// "" if no label matches.
func (m *Module) resolvePriority(issue *gh.Issue) string {
	for _, l := range issue.Labels {
		if p, ok := m.cfg.PriorityMapping[l.Name]; ok {
			return p
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// buildDescription renders a Jira description body that links back to the
// GitHub issue and inlines its body. The optional component is rendered as
// a header line when present.
func buildDescription(issue *gh.Issue, repo, component string) string {
	body := strings.TrimSpace(issue.Body)
	if body == "" {
		body = "(no description)"
	}
	labels := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, l.Name)
	}
	labelLine := ""
	if len(labels) > 0 {
		labelLine = "Labels: " + strings.Join(labels, ", ") + "\n"
	}
	componentLine := ""
	if component != "" {
		componentLine = "Component: " + component + "\n"
	}

	return fmt.Sprintf(
		"GitHub: %s\nReporter: @%s\n%s%s\n---\n\n%s",
		issue.HTMLURL, issue.User.Login, labelLine, componentLine, body,
	)
}
