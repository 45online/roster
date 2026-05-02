package issue_to_jira

import (
	"strings"
	"testing"

	gh "github.com/45online/roster/internal/adapters/github"
)

func TestResolveIssueType_DefaultsToTask(t *testing.T) {
	m := New(nil, nil, Config{})
	got := m.resolveIssueType(&gh.Issue{})
	if got != "Task" {
		t.Errorf("expected default 'Task', got %q", got)
	}
}

func TestResolveIssueType_RespectsConfigDefault(t *testing.T) {
	m := New(nil, nil, Config{DefaultIssueType: "Story"})
	got := m.resolveIssueType(&gh.Issue{})
	if got != "Story" {
		t.Errorf("expected 'Story', got %q", got)
	}
}

func TestResolveIssueType_LabelOverridesDefault(t *testing.T) {
	m := New(nil, nil, Config{
		DefaultIssueType: "Story",
		LabelToIssueType: map[string]string{"bug": "Bug"},
	})
	issue := &gh.Issue{Labels: []gh.Label{{Name: "bug"}}}
	got := m.resolveIssueType(issue)
	if got != "Bug" {
		t.Errorf("expected 'Bug' from label override, got %q", got)
	}
}

func TestResolveIssueType_FirstMatchingLabelWins(t *testing.T) {
	m := New(nil, nil, Config{
		LabelToIssueType: map[string]string{
			"bug":     "Bug",
			"feature": "Story",
		},
	})
	issue := &gh.Issue{Labels: []gh.Label{
		{Name: "feature"},
		{Name: "bug"},
	}}
	got := m.resolveIssueType(issue)
	// "feature" appears first in issue.Labels and matches the mapping.
	if got != "Story" {
		t.Errorf("expected 'Story' (first matching label), got %q", got)
	}
}

func TestResolvePriority_NoMatchReturnsEmpty(t *testing.T) {
	m := New(nil, nil, Config{
		PriorityMapping: map[string]string{"P0": "Highest"},
	})
	issue := &gh.Issue{Labels: []gh.Label{{Name: "documentation"}}}
	if p := m.resolvePriority(issue); p != "" {
		t.Errorf("expected empty priority, got %q", p)
	}
}

func TestResolvePriority_MatchesByLabel(t *testing.T) {
	m := New(nil, nil, Config{
		PriorityMapping: map[string]string{
			"P0": "Highest",
			"P1": "High",
		},
	})
	issue := &gh.Issue{Labels: []gh.Label{{Name: "P0"}}}
	if p := m.resolvePriority(issue); p != "Highest" {
		t.Errorf("expected 'Highest', got %q", p)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 10, "this is t…"},
	}
	for _, tc := range cases {
		got := truncate(tc.in, tc.n)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestBuildDescription_IncludesIssueMetadata(t *testing.T) {
	issue := &gh.Issue{
		HTMLURL: "https://github.com/foo/bar/issues/42",
		User:    gh.User{Login: "alice"},
		Body:    "We have a problem.",
		Labels:  []gh.Label{{Name: "bug"}, {Name: "P0"}},
	}
	desc := buildDescription(issue, "foo/bar", "")

	for _, s := range []string{
		"https://github.com/foo/bar/issues/42",
		"@alice",
		"We have a problem.",
		"bug, P0",
	} {
		if !strings.Contains(desc, s) {
			t.Errorf("description missing %q\n--- got ---\n%s", s, desc)
		}
	}
}

func TestBuildDescription_HandlesEmptyBody(t *testing.T) {
	issue := &gh.Issue{
		HTMLURL: "https://github.com/foo/bar/issues/1",
		User:    gh.User{Login: "bob"},
		Body:    "",
	}
	desc := buildDescription(issue, "foo/bar", "")
	if !strings.Contains(desc, "(no description)") {
		t.Errorf("expected '(no description)' marker for empty body, got: %s", desc)
	}
}
