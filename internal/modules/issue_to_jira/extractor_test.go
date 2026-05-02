package issue_to_jira

import (
	"strings"
	"testing"

	"github.com/45online/roster/internal/adapters/github"
)

func TestStripCodeFence_Plain(t *testing.T) {
	in := `{"summary":"x"}`
	if got := stripCodeFence(in); got != in {
		t.Errorf("plain JSON should pass through, got %q", got)
	}
}

func TestStripCodeFence_JSONFence(t *testing.T) {
	in := "```json\n{\"summary\":\"x\"}\n```"
	want := `{"summary":"x"}`
	if got := stripCodeFence(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCodeFence_BareFence(t *testing.T) {
	in := "```\n{\"x\":1}\n```"
	want := `{"x":1}`
	if got := stripCodeFence(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCodeFence_LeadingWhitespace(t *testing.T) {
	in := "   \n```json\n{\"x\":1}\n```\n  "
	want := `{"x":1}`
	if got := stripCodeFence(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildExtractorPrompt_Composes(t *testing.T) {
	issue := &github.Issue{
		Title:  "Login fails on Safari",
		Body:   "Steps:\n1. open https://app/login\n2. click Submit\n3. blank screen",
		User:   github.User{Login: "alice"},
		Labels: []github.Label{{Name: "bug"}, {Name: "P1"}},
	}
	got, err := buildExtractorPrompt(issue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range []string{
		"Login fails on Safari",
		"@alice",
		"bug, P1",
		"open https://app/login",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("prompt missing %q\n--- got ---\n%s", s, got)
		}
	}
}

func TestBuildExtractorPrompt_TruncatesLargeBody(t *testing.T) {
	huge := strings.Repeat("x", 20*1024)
	issue := &github.Issue{
		Title: "huge",
		Body:  huge,
		User:  github.User{Login: "bot"},
	}
	got, err := buildExtractorPrompt(issue)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "(truncated)") {
		t.Error("expected truncation marker in prompt")
	}
	if len(got) > 12*1024 {
		t.Errorf("prompt too long: %d bytes (expected ~8 KB body cap)", len(got))
	}
}

func TestBuildExtractorPrompt_NilIssue(t *testing.T) {
	if _, err := buildExtractorPrompt(nil); err == nil {
		t.Error("expected error for nil issue")
	}
}
