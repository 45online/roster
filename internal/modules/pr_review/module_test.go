package pr_review

import (
	"strings"
	"testing"

	gh "github.com/45online/roster/internal/adapters/github"
)

func TestStripCodeFence(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                 `{"a":1}`,
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"```\n{\"a\":1}\n```":     `{"a":1}`,
		"  ```\n{\"x\":2}\n```  ": `{"x":2}`,
	}
	for in, want := range cases {
		if got := stripCodeFence(in); got != want {
			t.Errorf("stripCodeFence(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidVerdict(t *testing.T) {
	for _, v := range []Verdict{VerdictApprove, VerdictRequestChanges, VerdictComment} {
		if !validVerdict(v) {
			t.Errorf("%q should be valid", v)
		}
	}
	for _, v := range []Verdict{"", "yes", "lgtm", "approveplz"} {
		if validVerdict(v) {
			t.Errorf("%q should be invalid", v)
		}
	}
}

func TestGateVerdict_Defaults_DowngradeBoth(t *testing.T) {
	m := New(nil, nil, "", Config{})
	cases := map[Verdict]gh.ReviewEvent{
		VerdictApprove:        gh.ReviewComment, // CanApprove=false → COMMENT
		VerdictRequestChanges: gh.ReviewComment, // CanRequestChanges=false → COMMENT
		VerdictComment:        gh.ReviewComment,
	}
	for v, want := range cases {
		if got := m.gateVerdict(v); got != want {
			t.Errorf("gate(%s) = %s, want %s", v, got, want)
		}
	}
}

func TestGateVerdict_AllowApprove(t *testing.T) {
	m := New(nil, nil, "", Config{CanApprove: true})
	if got := m.gateVerdict(VerdictApprove); got != gh.ReviewApprove {
		t.Errorf("expected APPROVE, got %s", got)
	}
	if got := m.gateVerdict(VerdictRequestChanges); got != gh.ReviewComment {
		t.Errorf("expected COMMENT (gate), got %s", got)
	}
}

func TestToGHComments_DropsInvalid(t *testing.T) {
	in := []LineComment{
		{Path: "a.go", Line: 10, Body: "ok"},
		{Path: "", Line: 1, Body: "no path"},
		{Path: "b.go", Line: 0, Body: "no line"},
		{Path: "c.go", Line: 1, Body: "  "},
		{Path: "d.go", Line: 5, Body: "ok2"},
	}
	out := toGHComments(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 valid comments, got %d", len(out))
	}
	if out[0].Path != "a.go" || out[1].Path != "d.go" {
		t.Errorf("wrong filtering: %+v", out)
	}
}

func TestChangedFiles(t *testing.T) {
	diff := `diff --git a/foo/bar.go b/foo/bar.go
index abc..def 100644
--- a/foo/bar.go
+++ b/foo/bar.go
@@ -1 +1 @@
-old
+new
diff --git a/docs/x.md b/docs/x.md
--- a/docs/x.md
+++ b/docs/x.md
@@ -1 +1,2 @@
 first
+second
`
	got := changedFiles(diff)
	want := []string{"foo/bar.go", "docs/x.md"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMatchesPathPrefix(t *testing.T) {
	cases := []struct {
		path, prefix string
		want         bool
	}{
		{"docs/intro.md", "docs/", true},
		{"docs/intro.md", "docs/**", true},
		{"docs/intro.md", "docs", true},
		{"src/foo.go", "docs/", false},
		{"README.md", "*.md", true},
		{"vendor/x.go", "vendor", true},
		{"vendored/x.go", "vendor", false}, // not-prefixed-by-/
		{"x.md", "*.go", false},
	}
	for _, tc := range cases {
		got := matchesPathPrefix(tc.path, tc.prefix)
		if got != tc.want {
			t.Errorf("matchesPathPrefix(%q, %q) = %v, want %v", tc.path, tc.prefix, got, tc.want)
		}
	}
}

func TestShouldSkip_EmptyDiff(t *testing.T) {
	m := New(nil, nil, "", Config{})
	if r := m.shouldSkip("   \n   "); r == "" {
		t.Error("expected to skip empty diff")
	}
}

func TestShouldSkip_AllPathsMatchSkipPrefixes(t *testing.T) {
	m := New(nil, nil, "", Config{
		SkipPaths: []string{"docs/", "*.md"},
	})
	diff := `diff --git a/docs/a.md b/docs/a.md
diff --git a/docs/sub/b.md b/docs/sub/b.md
diff --git a/CHANGELOG.md b/CHANGELOG.md
`
	if r := m.shouldSkip(diff); r == "" {
		t.Error("expected to skip docs-only diff")
	}
}

func TestShouldSkip_OneCodeFile_PreventsSkip(t *testing.T) {
	m := New(nil, nil, "", Config{
		SkipPaths: []string{"docs/"},
	})
	diff := `diff --git a/docs/a.md b/docs/a.md
diff --git a/src/foo.go b/src/foo.go
`
	if r := m.shouldSkip(diff); r != "" {
		t.Errorf("should NOT skip when a code file is touched: got %q", r)
	}
}

func TestBuildReviewPrompt_Truncates(t *testing.T) {
	pr := &gh.PullRequest{
		Number: 1, Title: "x", Body: "y",
		User: gh.User{Login: "u"},
		Head: gh.Ref{Ref: "feature"}, Base: gh.Ref{Ref: "main"},
	}
	huge := strings.Repeat("Z", 200)
	got := buildReviewPrompt(pr, huge, 50)
	if !strings.Contains(got, "(diff truncated") {
		t.Errorf("expected truncation marker, got: %s", got[:200])
	}
}

func TestBuildReviewBody_NotesPolicyDowngrade(t *testing.T) {
	r := &Review{Summary: "looks ok", Verdict: VerdictApprove}
	body := buildReviewBody(r, "claude-x", true)
	if !strings.Contains(body, "policy gate") {
		t.Errorf("expected policy gate note, got: %s", body)
	}
}
