package issue_to_confluence

import (
	"strings"
	"testing"

	gh "github.com/45online/roster/internal/adapters/github"
)

func TestHasLabel_CaseInsensitive(t *testing.T) {
	issue := &gh.Issue{Labels: []gh.Label{{Name: "Completed"}, {Name: "x"}}}
	if !hasLabel(issue, "completed") {
		t.Error("should match case-insensitively")
	}
	if hasLabel(issue, "wip") {
		t.Error("should not match a label that is not present")
	}
}

func TestRepoFromURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/foo/bar/issues/42": "foo/bar",
		"https://github.com/a/b":               "a/b",
		"https://example.com/foo/bar":          "",
		"":                                     "",
	}
	for in, want := range cases {
		if got := repoFromURL(in); got != want {
			t.Errorf("repoFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderStorage_HasAllSections(t *testing.T) {
	doc := &Document{
		Title:    "Login fails on Safari",
		Summary:  "Users on Safari cannot log in.",
		Decision: "Replaced cookie SameSite=None with proper SameSite=Lax for first-party.",
		Details:  "Affected versions: Safari 17. Workaround was a hard refresh.",
		PRLinks:  []string{"https://github.com/foo/bar/pull/123"},
	}
	issue := &gh.Issue{
		Number:  42,
		HTMLURL: "https://github.com/foo/bar/issues/42",
	}
	out := renderStorage(doc, issue)

	for _, want := range []string{
		"foo/bar#42",
		"<h2>Summary</h2>",
		"<h2>Decision</h2>",
		"<h2>Details</h2>",
		"<h2>References</h2>",
		"https://github.com/foo/bar/issues/42",
		"https://github.com/foo/bar/pull/123",
		`ac:name="info"`, // info macro
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestRenderStorage_OmitsEmptySections(t *testing.T) {
	doc := &Document{
		Title:   "x",
		Summary: "only summary",
	}
	issue := &gh.Issue{Number: 1, HTMLURL: "https://github.com/o/r/issues/1"}
	out := renderStorage(doc, issue)
	if strings.Contains(out, "<h2>Decision</h2>") {
		t.Error("Decision section should be omitted when empty")
	}
	if strings.Contains(out, "<h2>Details</h2>") {
		t.Error("Details section should be omitted when empty")
	}
	if !strings.Contains(out, "<h2>Summary</h2>") {
		t.Error("Summary section should be present")
	}
}

func TestRenderStorage_EscapesHTML(t *testing.T) {
	doc := &Document{
		Title:   "x",
		Summary: "<script>alert(1)</script>",
	}
	issue := &gh.Issue{Number: 1, HTMLURL: "https://github.com/o/r/issues/1"}
	out := renderStorage(doc, issue)
	if strings.Contains(out, "<script>") {
		t.Errorf("user-provided HTML must be escaped, got: %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected escaped <script>")
	}
}

func TestBuildSummarizePrompt_IncludesEverything(t *testing.T) {
	issue := &gh.Issue{
		Number:  7,
		Title:   "Memory leak in worker",
		Body:    "After 24h workers OOM.",
		HTMLURL: "https://github.com/foo/bar/issues/7",
		User:    gh.User{Login: "alice"},
		Labels:  []gh.Label{{Name: "bug"}, {Name: "completed"}},
	}
	comments := []gh.IssueComment{
		{User: gh.User{Login: "bob"}, Body: "Heap profile shows growing slice cap."},
		{User: gh.User{Login: "alice"}, Body: "Fixed in #99."},
	}
	got := buildSummarizePrompt("foo/bar", issue, comments)
	for _, want := range []string{
		"Memory leak in worker",
		"After 24h workers OOM.",
		"@alice",
		"@bob",
		"Heap profile shows",
		"Fixed in #99",
		"bug, completed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\n--- got ---\n%s", want, got)
		}
	}
}
