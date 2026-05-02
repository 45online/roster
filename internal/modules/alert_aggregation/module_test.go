package alert_aggregation

import (
	"strings"
	"testing"
	"time"

	gh "github.com/45online/roster/internal/adapters/github"
)

func t(s string) time.Time {
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return tt
}

func TestAlertHeaderEmoji(t *testing.T) {
	cases := map[string]string{
		"critical": "🚨",
		"P0":       "🚨",
		"warning":  "⚠️",
		"info":     "ℹ️",
		"":         "🔔",
		"weird":    "🔔",
	}
	for in, want := range cases {
		if got := alertHeaderEmoji(in); got != want {
			t.Errorf("alertHeaderEmoji(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAgo(t_ *testing.T) {
	now := t("2026-05-03T14:00:00Z")
	cases := []struct {
		when string
		want string
	}{
		{"2026-05-03T13:59:59Z", "<1m"},
		{"2026-05-03T13:55:00Z", "5m"},
		{"2026-05-03T12:00:00Z", "2h"},
		{"2026-05-01T14:00:00Z", "2d"},
	}
	for _, c := range cases {
		if got := ago(now, t(c.when)); got != c.want {
			t_.Errorf("ago(%s) = %q, want %q", c.when, got, c.want)
		}
	}
}

func TestShortSHA(t *testing.T) {
	if got := shortSHA("a3f9c1d4e5"); got != "a3f9c1d" {
		t.Errorf("got %q", got)
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Errorf("short input passed through, got %q", got)
	}
}

func TestTrimToMax_DividesEvenly(t *testing.T) {
	commits := make([]gh.Commit, 8)
	pulls := make([]gh.MergedPR, 8)
	for i := range commits {
		commits[i].SHA = "c"
	}
	for i := range pulls {
		pulls[i].Number = i + 1
	}
	c, p := trimToMax(commits, pulls, 6)
	if len(c) != 3 || len(p) != 3 {
		t.Errorf("expected 3+3, got %d+%d", len(c), len(p))
	}
}

func TestTrimToMax_BelowCap_PassesThrough(t *testing.T) {
	commits := make([]gh.Commit, 2)
	pulls := make([]gh.MergedPR, 1)
	c, p := trimToMax(commits, pulls, 10)
	if len(c) != 2 || len(p) != 1 {
		t.Errorf("expected unchanged, got %d+%d", len(c), len(p))
	}
}

func TestFormatMessage_NoActivity(t_ *testing.T) {
	now := t("2026-05-03T14:00:00Z")
	a := Alert{
		Source:   "CloudWatch",
		Severity: "critical",
		Title:    "5xx error spike",
		Body:     "Error rate at 8.2%",
		FiredAt:  now,
	}
	got := FormatMessage("foo/bar", a, nil, nil, now)
	for _, want := range []string{
		"🚨 [CloudWatch] 5xx error spike",
		"> Error rate at 8.2%",
		"_Time:",
		"<https://github.com/foo/bar|foo/bar>",
		"No commits or merged PRs",
	} {
		if !strings.Contains(got, want) {
			t_.Errorf("missing %q\n--- got ---\n%s", want, got)
		}
	}
}

func TestFormatMessage_WithActivity_NewestFirst(t_ *testing.T) {
	now := t("2026-05-03T14:00:00Z")

	commits := []gh.Commit{
		{
			SHA:     "abcdef0123",
			HTMLURL: "https://github.com/foo/bar/commit/abcdef0",
			Commit: gh.CommitDetail{
				Message: "fix: rate limiter\n\nlong body",
				Author:  gh.CommitIdentity{Date: t("2026-05-03T13:55:00Z")},
			},
			Author: &gh.User{Login: "alice"},
		},
	}
	mergedAt := t("2026-05-03T13:30:00Z")
	pulls := []gh.MergedPR{
		{Number: 234, Title: "auth refactor", HTMLURL: "https://github.com/foo/bar/pull/234",
			User: gh.User{Login: "bob"}, MergedAt: &mergedAt},
	}

	got := FormatMessage("foo/bar", Alert{
		Source: "Datadog", Severity: "warning", Title: "latency p99 high", FiredAt: now,
	}, commits, pulls, now)

	// Recent activity section present.
	if !strings.Contains(got, "Recent activity") {
		t_.Errorf("missing recent activity header\n%s", got)
	}
	// Commit content
	if !strings.Contains(got, "abcdef0") || !strings.Contains(got, "@alice") || !strings.Contains(got, "rate limiter") {
		t_.Errorf("commit not rendered\n%s", got)
	}
	// PR content
	if !strings.Contains(got, "PR #234") || !strings.Contains(got, "@bob") {
		t_.Errorf("PR not rendered\n%s", got)
	}
	// Newest-first: commit @ 13:55 should appear before PR @ 13:30 (search positions).
	cIdx := strings.Index(got, "abcdef0")
	pIdx := strings.Index(got, "PR #234")
	if cIdx >= 0 && pIdx >= 0 && cIdx > pIdx {
		t_.Errorf("expected commit (newer) before PR (older); got commit at %d, PR at %d", cIdx, pIdx)
	}
}

func TestFormatMessage_DoesNotAtMention(t_ *testing.T) {
	// Crucial design property: messages must use "@username" as plain text,
	// NOT Slack's <@U-id> mention syntax. Module D is for awareness only —
	// nobody should get notified by Roster's guess at causation.
	now := t("2026-05-03T14:00:00Z")
	commits := []gh.Commit{
		{
			SHA: "x", Commit: gh.CommitDetail{Message: "msg", Author: gh.CommitIdentity{Date: now}},
			Author: &gh.User{Login: "alice"},
		},
	}
	got := FormatMessage("foo/bar", Alert{Source: "x", Title: "y", FiredAt: now}, commits, nil, now)
	if strings.Contains(got, "<@") {
		t_.Errorf("must not use Slack mention syntax; got: %s", got)
	}
}

func TestFormatMessage_WithLinks(t_ *testing.T) {
	now := t("2026-05-03T14:00:00Z")
	a := Alert{
		Source: "x", Title: "y", FiredAt: now,
		Links: []NamedLink{
			{Label: "Logs", URL: "https://logs.example.com"},
			{Label: "Runbook", URL: "https://wiki.example.com/runbook"},
			{Label: "skipped", URL: ""},
		},
	}
	got := FormatMessage("foo/bar", a, nil, nil, now)
	for _, want := range []string{
		"🔗",
		"<https://logs.example.com|Logs>",
		"<https://wiki.example.com/runbook|Runbook>",
	} {
		if !strings.Contains(got, want) {
			t_.Errorf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "|skipped>") {
		t_.Errorf("links with empty URL should be dropped\n%s", got)
	}
}
