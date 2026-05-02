// Package pr_review implements Module B: AI code review of GitHub pull
// requests. Given a (repo, PR number) it fetches PR metadata + unified
// diff, asks Claude to produce a structured review, then posts the review
// (line comments + verdict) back to GitHub.
//
// Module B is intentionally code-only — it does NOT cross-check against
// design docs in /docs, because docs are usually stale. The review focuses
// on clear bugs, security issues, missing error handling, and obvious
// code-quality problems.
package pr_review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/api"
	"github.com/45online/roster/internal/audit"
)

// DefaultModel is the Claude model used when none is configured. Sonnet
// balances depth (real bug-spotting beats nits) with cost (PRs aren't free).
const DefaultModel = "claude-sonnet-4-6-20250514"

// Config controls Module B's per-project behavior.
type Config struct {
	// MaxDiffBytes caps how much of the diff is sent to Claude. Diffs
	// larger than this are summarised: Claude only gets the first N bytes
	// plus a "(diff truncated)" marker. Default: 64 KB.
	MaxDiffBytes int
	// SkipPaths is a list of glob-ish path prefixes that, if matched by
	// every changed file, skip the review entirely (e.g. doc-only PRs).
	// Empty list disables this short-circuit.
	SkipPaths []string
	// CanApprove gates the APPROVE verdict. When false (default for
	// safety), even a clean review is downgraded to COMMENT — the actual
	// approval still requires a human reviewer.
	CanApprove bool
	// CanRequestChanges gates the REQUEST_CHANGES verdict. When false,
	// downgrades to COMMENT (so AI doesn't block merges by itself).
	CanRequestChanges bool
}

// Verdict is Claude's recommendation.
type Verdict string

const (
	VerdictApprove        Verdict = "approve"
	VerdictRequestChanges Verdict = "request_changes"
	VerdictComment        Verdict = "comment"
)

// Review is the structured output the prompt asks Claude to return.
type Review struct {
	Summary  string        `json:"summary"`
	Verdict  Verdict       `json:"verdict"`
	Comments []LineComment `json:"comments"`
}

// LineComment is a single inline comment on a diff line.
type LineComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Body string `json:"body"`
}

// Module is the PR review module.
type Module struct {
	gh    *gh.Client
	api   api.Client
	cfg   Config
	model string
	audit *audit.Recorder
}

// New constructs a Module. Caller supplies a Claude client and optional config.
// model="" picks DefaultModel.
func New(github *gh.Client, claude api.Client, model string, cfg Config) *Module {
	if model == "" {
		model = DefaultModel
	}
	if cfg.MaxDiffBytes == 0 {
		cfg.MaxDiffBytes = 64 * 1024
	}
	return &Module{gh: github, api: claude, model: model, cfg: cfg}
}

// WithAudit attaches an audit recorder.
func (m *Module) WithAudit(r *audit.Recorder) *Module {
	m.audit = r
	return m
}

// Result is what ReviewPR returns.
type Result struct {
	Verdict      Verdict
	CommentCount int
	Skipped      bool   // true if the diff was empty / above SkipPaths only
	SkipReason   string // populated when Skipped
}

// ReviewPR fetches the PR + diff, asks Claude to review it, and posts the
// result back to GitHub.
func (m *Module) ReviewPR(ctx context.Context, repo string, number int) (*Result, error) {
	started := time.Now()
	entry := audit.Entry{
		Module: "pr_review",
		Repo:   repo,
		Issue:  number, // PR # uses same field; GH numbers are unified
		Model:  m.model,
	}

	pr, err := m.gh.GetPullRequest(ctx, repo, number)
	if err != nil {
		return m.fail(entry, started, "fetch PR", err)
	}
	entry.Actor = pr.User.Login

	diff, err := m.gh.GetPullRequestDiff(ctx, repo, number)
	if err != nil {
		return m.fail(entry, started, "fetch diff", err)
	}

	if reason := m.shouldSkip(diff); reason != "" {
		entry.Status = "skipped"
		entry.Error = reason
		entry.DurationMS = time.Since(started).Milliseconds()
		m.audit.Record(entry)
		return &Result{Skipped: true, SkipReason: reason}, nil
	}

	review, err := m.askClaude(ctx, pr, diff)
	if err != nil {
		return m.fail(entry, started, "claude review", err)
	}
	entry.AIExtracted = true

	gate := m.gateVerdict(review.Verdict)
	body := buildReviewBody(review, m.model, gate != mapEvent(review.Verdict))

	if err := m.gh.CreateReview(ctx, repo, number, gh.CreateReviewRequest{
		Body:     body,
		Event:    gate,
		Comments: toGHComments(review.Comments),
	}); err != nil {
		return m.fail(entry, started, "submit review", err)
	}

	entry.Status = "success"
	entry.JiraKey = "" // not applicable
	entry.DurationMS = time.Since(started).Milliseconds()
	m.audit.Record(entry)

	return &Result{
		Verdict:      review.Verdict,
		CommentCount: len(review.Comments),
	}, nil
}

func (m *Module) fail(entry audit.Entry, start time.Time, stage string, err error) (*Result, error) {
	entry.Status = "error"
	entry.Error = fmt.Sprintf("%s: %v", stage, err)
	entry.DurationMS = time.Since(start).Milliseconds()
	m.audit.Record(entry)
	return nil, fmt.Errorf("%s: %w", stage, err)
}

// shouldSkip returns a non-empty reason string to skip the review.
func (m *Module) shouldSkip(diff string) string {
	if strings.TrimSpace(diff) == "" {
		return "empty diff"
	}
	if len(m.cfg.SkipPaths) == 0 {
		return ""
	}
	files := changedFiles(diff)
	if len(files) == 0 {
		return ""
	}
	for _, f := range files {
		matched := false
		for _, prefix := range m.cfg.SkipPaths {
			if matchesPathPrefix(f, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			return ""
		}
	}
	return "all changed paths matched skip_paths"
}

// gateVerdict applies CanApprove / CanRequestChanges policy and translates
// to the GitHub review event string. Downgraded verdicts become COMMENT.
func (m *Module) gateVerdict(v Verdict) gh.ReviewEvent {
	switch v {
	case VerdictApprove:
		if m.cfg.CanApprove {
			return gh.ReviewApprove
		}
		return gh.ReviewComment
	case VerdictRequestChanges:
		if m.cfg.CanRequestChanges {
			return gh.ReviewRequestChanges
		}
		return gh.ReviewComment
	default:
		return gh.ReviewComment
	}
}

// mapEvent translates Verdict to ReviewEvent (no policy applied).
func mapEvent(v Verdict) gh.ReviewEvent {
	switch v {
	case VerdictApprove:
		return gh.ReviewApprove
	case VerdictRequestChanges:
		return gh.ReviewRequestChanges
	default:
		return gh.ReviewComment
	}
}

// toGHComments converts Module B's LineComment to the GitHub adapter type.
func toGHComments(in []LineComment) []gh.ReviewLineComment {
	out := make([]gh.ReviewLineComment, 0, len(in))
	for _, c := range in {
		if c.Path == "" || c.Line <= 0 || strings.TrimSpace(c.Body) == "" {
			continue
		}
		out = append(out, gh.ReviewLineComment{
			Path: c.Path,
			Line: c.Line,
			Body: c.Body,
		})
	}
	return out
}

// buildReviewBody composes the top-level review body (the "summary"
// section). When the verdict was downgraded by policy, that's noted.
func buildReviewBody(r *Review, model string, downgraded bool) string {
	var b strings.Builder
	b.WriteString("🤖 **AI Review**")
	b.WriteString(" (")
	b.WriteString(model)
	b.WriteString(")\n\n")
	b.WriteString(strings.TrimSpace(r.Summary))
	b.WriteString("\n\n")

	verdictLabel := strings.ToUpper(string(r.Verdict))
	b.WriteString("**Recommendation: ")
	b.WriteString(verdictLabel)
	b.WriteString("**")
	if downgraded {
		b.WriteString(" — submitted as COMMENT only (policy gate)")
	}
	b.WriteString("\n")
	return b.String()
}

// askClaude builds the messages, calls the API, decodes the JSON.
func (m *Module) askClaude(ctx context.Context, pr *gh.PullRequest, diff string) (*Review, error) {
	prompt := buildReviewPrompt(pr, diff, m.cfg.MaxDiffBytes)
	contentJSON, _ := json.Marshal(prompt)

	req := &api.MessageRequest{
		Model:       m.model,
		MaxTokens:   4096,
		System:      reviewSystemPrompt,
		Messages:    []api.MessageParam{{Role: "user", Content: contentJSON}},
		QuerySource: "background",
	}

	resp, err := m.api.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("claude complete: %w", err)
	}
	text := firstTextBlock(resp.Content)
	if text == "" {
		return nil, fmt.Errorf("claude returned no text")
	}

	jsonBody := stripCodeFence(text)
	var r Review
	if err := json.Unmarshal([]byte(jsonBody), &r); err != nil {
		return nil, fmt.Errorf("decode review json: %w (raw: %q)", err, jsonBody)
	}
	if !validVerdict(r.Verdict) {
		// Default to COMMENT on bogus verdict — never block on AI hallucination.
		r.Verdict = VerdictComment
	}
	return &r, nil
}

func validVerdict(v Verdict) bool {
	switch v {
	case VerdictApprove, VerdictRequestChanges, VerdictComment:
		return true
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
