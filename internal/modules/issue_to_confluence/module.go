// Package issue_to_confluence implements Module C: when a GitHub issue is
// closed (and tagged "completed"), Roster asks Claude to summarise the
// issue thread into a structured page, files it in Confluence as a DRAFT
// (not published), and pings the issue owner on Slack with the link.
//
// The "draft" status is deliberate: AI-generated summaries deserve a human
// pass before going to the documentation space, and Confluence's draft
// state is the natural mechanism for that.
package issue_to_confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/45online/roster/internal/adapters/confluence"
	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/adapters/slack"
	"github.com/45online/roster/internal/api"
	"github.com/45online/roster/internal/audit"
	"github.com/45online/roster/internal/budget"
	"github.com/45online/roster/internal/undercover"
)

// DefaultModel is Sonnet — Module C produces longer-form prose than
// Module A's field extractor, and quality matters more than latency.
const DefaultModel = "claude-sonnet-4-6-20250514"

// Config configures Module C per project.
type Config struct {
	// SpaceID is the Confluence numeric space ID where drafts are filed.
	SpaceID string
	// ParentPageID is optional: the parent page under which drafts are
	// nested in the space tree.
	ParentPageID string
	// CompletedLabel is the GitHub label that gates archival. Defaults to
	// "completed".
	CompletedLabel string
	// SlackChannel is where the back-link is posted (channel ID or "#name").
	// Empty means: no Slack notification.
	SlackChannel string
}

// Module is the Issue → Confluence archival module.
type Module struct {
	gh         *gh.Client
	confluence *confluence.Client
	slack      *slack.Client // optional
	api        api.Client
	cfg        Config
	model      string
	audit      *audit.Recorder
}

// New constructs Module C. slackCli may be nil — Slack notification is
// then skipped silently.
func New(github *gh.Client, conf *confluence.Client, slackCli *slack.Client, claude api.Client, model string, cfg Config) *Module {
	if model == "" {
		model = DefaultModel
	}
	if cfg.CompletedLabel == "" {
		cfg.CompletedLabel = "completed"
	}
	return &Module{
		gh: github, confluence: conf, slack: slackCli, api: claude,
		cfg: cfg, model: model,
	}
}

// WithAudit attaches an audit recorder.
func (m *Module) WithAudit(r *audit.Recorder) *Module {
	m.audit = r
	return m
}

// Result is what ArchiveIssue returns on success.
type Result struct {
	PageID  string
	PageURL string
	Skipped bool
	Reason  string
}

// ArchiveIssue runs the full pipeline. It expects the caller to have
// already gated on issue closed-state; the only gate inside is the
// completed label (so manual invocations work too).
func (m *Module) ArchiveIssue(ctx context.Context, repo string, number int) (*Result, error) {
	started := time.Now()
	entry := audit.Entry{
		Module: "issue_to_confluence",
		Repo:   repo,
		Issue:  number,
		Model:  m.model,
	}

	issue, err := m.gh.GetIssue(ctx, repo, number)
	if err != nil {
		return m.fail(entry, started, "fetch issue", err)
	}
	entry.Actor = issue.User.Login

	if !hasLabel(issue, m.cfg.CompletedLabel) {
		entry.Status = "skipped"
		entry.Error = fmt.Sprintf("missing %q label", m.cfg.CompletedLabel)
		entry.DurationMS = time.Since(started).Milliseconds()
		m.audit.Record(entry)
		return &Result{Skipped: true, Reason: entry.Error}, nil
	}

	comments, err := m.gh.ListIssueComments(ctx, repo, number)
	if err != nil {
		return m.fail(entry, started, "list comments", err)
	}

	doc, usage, err := m.askClaude(ctx, repo, issue, comments)
	if err != nil {
		return m.fail(entry, started, "claude summarize", err)
	}
	entry.AIExtracted = true
	entry.InputTokens = usage.InputTokens
	entry.OutputTokens = usage.OutputTokens
	entry.CacheCreateTokens = usage.CacheCreationInputTokens
	entry.CacheReadTokens = usage.CacheReadInputTokens
	entry.CostUSD = budget.CostForUsage(m.model, usage)

	// Undercover scrub on every AI-authored field before it gets baked
	// into the Confluence page body. Title and the four prose sections
	// all flow into renderStorage's HTML.
	doc.Title = undercover.Redact(doc.Title)
	doc.Summary = undercover.Redact(doc.Summary)
	doc.Decision = undercover.Redact(doc.Decision)
	doc.Details = undercover.Redact(doc.Details)
	page, err := m.confluence.CreateDraft(ctx, confluence.CreateDraftRequest{
		SpaceID:     m.cfg.SpaceID,
		ParentID:    m.cfg.ParentPageID,
		Title:       doc.Title,
		BodyStorage: renderStorage(doc, issue),
	})
	if err != nil {
		return m.fail(entry, started, "create confluence draft", err)
	}
	entry.JiraKey = page.ID // we reuse JiraKey as the generic "external ref"
	entry.JiraURL = page.URL()

	if m.slack != nil && m.cfg.SlackChannel != "" {
		msg := fmt.Sprintf(
			"📄 New Confluence draft for issue %s#%d (closed by @%s)\nTitle: *%s*\nReview & publish: %s",
			repo, number, issue.User.Login, doc.Title, page.URL(),
		)
		if _, err := m.slack.PostMessage(ctx, slack.PostMessageRequest{
			Channel: m.cfg.SlackChannel,
			Text:    msg,
		}); err != nil {
			// Page exists; treat Slack failure as partial.
			entry.Status = "partial"
			entry.Error = fmt.Sprintf("slack notify: %v", err)
			entry.DurationMS = time.Since(started).Milliseconds()
			m.audit.Record(entry)
			return &Result{PageID: page.ID, PageURL: page.URL()},
				fmt.Errorf("slack notify (page already created): %w", err)
		}
	}

	entry.Status = "success"
	entry.DurationMS = time.Since(started).Milliseconds()
	m.audit.Record(entry)
	return &Result{PageID: page.ID, PageURL: page.URL()}, nil
}

func (m *Module) fail(entry audit.Entry, start time.Time, stage string, err error) (*Result, error) {
	entry.Status = "error"
	entry.Error = fmt.Sprintf("%s: %v", stage, err)
	entry.DurationMS = time.Since(start).Milliseconds()
	m.audit.Record(entry)
	return nil, fmt.Errorf("%s: %w", stage, err)
}

// Document is the structured output the prompt asks Claude to return.
type Document struct {
	Title    string `json:"title"`
	Summary  string `json:"summary"`  // 1-2 paragraph problem statement
	Decision string `json:"decision"` // what was decided / done
	Details  string `json:"details"`  // technical details, edge cases
	PRLinks  []string `json:"pr_links"`
}

// askClaude builds the prompt, calls Claude, decodes the JSON. Returns
// the Document plus the response Usage for audit.
func (m *Module) askClaude(ctx context.Context, repo string, issue *gh.Issue, comments []gh.IssueComment) (*Document, api.Usage, error) {
	user := buildSummarizePrompt(repo, issue, comments)
	contentJSON, _ := json.Marshal(user)

	req := &api.MessageRequest{
		Model:       m.model,
		MaxTokens:   4096,
		System:      summarizeSystemPrompt + undercover.SystemSuffix,
		Messages:    []api.MessageParam{{Role: "user", Content: contentJSON}},
		QuerySource: "background",
	}
	resp, err := m.api.Complete(ctx, req)
	if err != nil {
		return nil, api.Usage{}, fmt.Errorf("claude complete: %w", err)
	}
	text := firstTextBlock(resp.Content)
	if text == "" {
		return nil, resp.Usage, fmt.Errorf("claude returned no text")
	}

	jsonBody := stripCodeFence(text)
	var d Document
	if err := json.Unmarshal([]byte(jsonBody), &d); err != nil {
		return nil, resp.Usage, fmt.Errorf("decode summary json: %w (raw: %q)", err, jsonBody)
	}
	if strings.TrimSpace(d.Title) == "" {
		d.Title = strings.TrimSpace(issue.Title)
	}
	return &d, resp.Usage, nil
}

// hasLabel reports whether the issue carries the named label (case-insensitive).
func hasLabel(issue *gh.Issue, name string) bool {
	for _, l := range issue.Labels {
		if strings.EqualFold(l.Name, name) {
			return true
		}
	}
	return false
}

func firstTextBlock(blocks []api.ContentBlock) string {
	for _, b := range blocks {
		if b.Type == "text" {
			return b.Text
		}
	}
	return ""
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
