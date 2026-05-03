// Package alert_aggregation implements Module D: when an external alert
// fires, post a Slack message that pairs the alert text with a list of
// recent commits and merged PRs in the repo.
//
// Module D is intentionally a "log platform"-style information aggregator.
// It does NOT:
//   - attribute the alert to any commit (correlation ≠ causation)
//   - @-mention anyone (no one wakes up because Roster guessed)
//   - file a Jira / GitHub issue
//
// The output is meant to land in oncall's Slack feed so they can scan
// recent activity at a glance, then make their own call.
package alert_aggregation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/adapters/slack"
	"github.com/45online/roster/internal/audit"
	"github.com/45online/roster/internal/undercover"
)

// Config configures Module D per project.
type Config struct {
	// SlackChannel is the destination channel ID or "#name". Required.
	SlackChannel string
	// Lookback is how far back to gather context. Default: 1h.
	Lookback time.Duration
	// MaxItems caps the total commits + PRs in the aggregated message.
	// Default: 10.
	MaxItems int
}

// Alert is the input to Aggregate — the external alert system supplies
// these fields.
type Alert struct {
	// Source is a human label for where the alert came from
	// (e.g. "CloudWatch", "Datadog", "PagerDuty"). Optional.
	Source string
	// Severity is "critical" / "warning" / etc. Optional, used as a header.
	Severity string
	// Title is the short alert title. Required.
	Title string
	// Body is the full alert payload. Optional, included verbatim.
	Body string
	// FiredAt is when the alert was raised; defaults to time.Now().
	FiredAt time.Time
	// Links is an optional set of URLs (logs, dashboards, runbooks) to
	// append to the message footer.
	Links []NamedLink
}

// NamedLink is a label + URL pair rendered as Slack markdown.
type NamedLink struct {
	Label string
	URL   string
}

// Module is the alert-aggregation module.
type Module struct {
	gh    *gh.Client
	slack *slack.Client
	cfg   Config
	audit *audit.Recorder
}

// New constructs Module D.
func New(github *gh.Client, slackCli *slack.Client, cfg Config) *Module {
	if cfg.Lookback == 0 {
		cfg.Lookback = time.Hour
	}
	if cfg.MaxItems == 0 {
		cfg.MaxItems = 10
	}
	return &Module{gh: github, slack: slackCli, cfg: cfg}
}

// WithAudit attaches an audit recorder.
func (m *Module) WithAudit(r *audit.Recorder) *Module {
	m.audit = r
	return m
}

// Result is what Aggregate returns on success.
type Result struct {
	SlackTs    string
	NumCommits int
	NumPulls   int
}

// Aggregate gathers recent commits + merged PRs in the repo, formats the
// alert + context as a Slack message, and posts it to the configured channel.
func (m *Module) Aggregate(ctx context.Context, repo string, alert Alert) (*Result, error) {
	started := time.Now()
	if alert.FiredAt.IsZero() {
		alert.FiredAt = time.Now().UTC()
	}
	entry := audit.Entry{
		Module: "alert_aggregation",
		Repo:   repo,
		Actor:  alert.Source,
	}

	if m.cfg.SlackChannel == "" {
		return m.fail(entry, started, "config", fmt.Errorf("SlackChannel is required"))
	}
	if m.slack == nil {
		return m.fail(entry, started, "config", fmt.Errorf("Slack client not configured"))
	}
	if strings.TrimSpace(alert.Title) == "" {
		return m.fail(entry, started, "input", fmt.Errorf("alert.Title is required"))
	}

	since := alert.FiredAt.Add(-m.cfg.Lookback)
	commits, err := m.gh.ListCommits(ctx, repo, since)
	if err != nil {
		// Soft-fail context fetching: still post the alert without context.
		commits = nil
		entry.Error = "list commits: " + err.Error()
	}
	pulls, err := m.gh.ListRecentMergedPulls(ctx, repo, since, 50)
	if err != nil {
		pulls = nil
		if entry.Error != "" {
			entry.Error += "; "
		}
		entry.Error += "list pulls: " + err.Error()
	}

	commits, pulls = trimToMax(commits, pulls, m.cfg.MaxItems)

	// Module D doesn't call Claude, but the rendered text quotes commit
	// messages and PR titles verbatim — both can carry secret-shaped or
	// vendor-name strings. Run the redactor as a final pass so the
	// Slack channel never carries leaked tokens or "AI Review" residue
	// from earlier Module B output that ended up in a commit.
	text := undercover.Redact(FormatMessage(repo, alert, commits, pulls, alert.FiredAt))

	resp, err := m.slack.PostMessage(ctx, slack.PostMessageRequest{
		Channel: m.cfg.SlackChannel,
		Text:    text,
	})
	if err != nil {
		return m.fail(entry, started, "slack post", err)
	}

	if entry.Error != "" {
		entry.Status = "partial"
	} else {
		entry.Status = "success"
	}
	entry.DurationMS = time.Since(started).Milliseconds()
	m.audit.Record(entry)

	return &Result{
		SlackTs:    resp.Ts,
		NumCommits: len(commits),
		NumPulls:   len(pulls),
	}, nil
}

func (m *Module) fail(entry audit.Entry, start time.Time, stage string, err error) (*Result, error) {
	entry.Status = "error"
	entry.Error = fmt.Sprintf("%s: %v", stage, err)
	entry.DurationMS = time.Since(start).Milliseconds()
	m.audit.Record(entry)
	return nil, fmt.Errorf("%s: %w", stage, err)
}

// trimToMax keeps the most recent commits + PRs combined under maxTotal,
// dividing the cap roughly evenly. Returns slices in oldest-first order.
func trimToMax(commits []gh.Commit, pulls []gh.MergedPR, maxTotal int) ([]gh.Commit, []gh.MergedPR) {
	if maxTotal <= 0 {
		return commits, pulls
	}
	// Both inputs are newest-first from GitHub; keep that until rendering.
	cBudget := maxTotal / 2
	pBudget := maxTotal - cBudget
	if len(commits) > cBudget {
		commits = commits[:cBudget]
	}
	if len(pulls) > pBudget {
		pulls = pulls[:pBudget]
	}
	return commits, pulls
}

// FormatMessage renders the Slack text. Exported for testability — Module D
// is largely string formatting, and tests run cleanly against this without
// any HTTP mocks.
func FormatMessage(repo string, a Alert, commits []gh.Commit, pulls []gh.MergedPR, now time.Time) string {
	var b strings.Builder

	header := alertHeaderEmoji(a.Severity)
	if a.Source != "" {
		fmt.Fprintf(&b, "%s [%s] %s\n", header, a.Source, a.Title)
	} else {
		fmt.Fprintf(&b, "%s %s\n", header, a.Title)
	}
	if body := strings.TrimSpace(a.Body); body != "" {
		// Slack honors leading "> " as quoted text.
		for _, line := range strings.Split(body, "\n") {
			b.WriteString("> ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "_Time: %s_\n", a.FiredAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "_Repo: <https://github.com/%s|%s>_\n\n", repo, repo)

	if len(commits) == 0 && len(pulls) == 0 {
		b.WriteString("_No commits or merged PRs in the lookback window._")
	} else {
		b.WriteString("📋 *Recent activity:*\n")
		// Merge by time and render as one chronological list (newest first).
		items := mergeForDisplay(commits, pulls, now)
		for _, line := range items {
			b.WriteString("• ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	if len(a.Links) > 0 {
		b.WriteString("\n🔗 ")
		parts := make([]string, 0, len(a.Links))
		for _, l := range a.Links {
			if l.URL == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("<%s|%s>", l.URL, l.Label))
		}
		b.WriteString(strings.Join(parts, "  ·  "))
	}

	return b.String()
}

// mergeForDisplay sorts commits and PRs together by time (newest first)
// and renders each as a one-line Slack-formatted entry.
func mergeForDisplay(commits []gh.Commit, pulls []gh.MergedPR, now time.Time) []string {
	type item struct {
		when time.Time
		text string
	}
	items := make([]item, 0, len(commits)+len(pulls))

	for _, c := range commits {
		when := c.Commit.Author.Date
		short := truncate(c.ShortMessage(), 80)
		items = append(items, item{
			when: when,
			text: fmt.Sprintf("`%s` <%s|commit> by @%s — %q  _%s ago_",
				shortSHA(c.SHA), c.HTMLURL, c.AuthorLogin(), short, ago(now, when)),
		})
	}
	for _, p := range pulls {
		when := time.Time{}
		if p.MergedAt != nil {
			when = *p.MergedAt
		}
		items = append(items, item{
			when: when,
			text: fmt.Sprintf("<%s|PR #%d> merged by @%s — %q  _%s ago_",
				p.HTMLURL, p.Number, p.User.Login, truncate(p.Title, 80), ago(now, when)),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].when.After(items[j].when)
	})

	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.text
	}
	return out
}

// alertHeaderEmoji picks an emoji from a free-form severity string.
func alertHeaderEmoji(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical", "fatal", "p0", "p1":
		return "🚨"
	case "warning", "warn", "p2":
		return "⚠️"
	case "info", "notice":
		return "ℹ️"
	}
	return "🔔"
}

func ago(now, t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := now.Sub(t)
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func shortSHA(s string) string {
	if len(s) <= 7 {
		return s
	}
	return s[:7]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
