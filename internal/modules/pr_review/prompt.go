package pr_review

import (
	"fmt"
	"strings"

	gh "github.com/45online/roster/internal/adapters/github"
)

// reviewSystemPrompt instructs Claude to produce a JSON-structured code review.
const reviewSystemPrompt = `You are a senior code reviewer. Read the unified diff
and produce a structured JSON review. Reply with ONLY a JSON object — no
commentary, no code fences. Schema:

{
  "summary":  "<one paragraph: what this PR changes and your overall take>",
  "verdict":  "approve" | "request_changes" | "comment",
  "comments": [ { "path": "<file>", "line": <int>, "body": "<feedback>" } ]
}

Guidance for verdict:
- "request_changes": only for clear blockers — bugs, security issues
  (SQL injection, hardcoded secrets, missing auth checks), data loss, race
  conditions, broken error handling on a critical path.
- "approve": the change is sound and the inline comments (if any) are
  optional polish, not blockers.
- "comment": minor suggestions or questions only; no strong recommendation
  either way.

Guidance for comments[]:
- Each comment MUST point at a line that appears in the diff (added "+"
  line or context). The "line" is the 1-based line number in the file's
  AFTER state (right side of the diff).
- Focus on substantive issues: clear bugs, security risks, missed nil
  checks, broken concurrency, leaked resources, wrong error wrapping.
- AVOID: bikeshedding, formatting, naming preferences, "consider extracting
  this", or any comment that begins with "nit:".
- AVOID: praise comments. The summary is where you note what's good.
- Prefer 0-3 high-signal comments over 10 low-signal ones.

If the diff is too small/trivial to review meaningfully, return:
  {"summary":"...", "verdict":"approve", "comments":[]}

Do NOT cross-reference design docs you cannot see. Do NOT speculate about
intent — only flag what's clearly wrong from the diff itself.

The output MUST be valid JSON parseable by Go's encoding/json.`

// buildReviewPrompt composes the user message: PR metadata + the diff
// (truncated to maxDiffBytes if needed).
func buildReviewPrompt(pr *gh.PullRequest, diff string, maxDiffBytes int) string {
	body := strings.TrimSpace(pr.Body)
	if body == "" {
		body = "(no description)"
	}

	truncated := false
	if maxDiffBytes > 0 && len(diff) > maxDiffBytes {
		diff = diff[:maxDiffBytes] + "\n\n…(diff truncated; the rest is omitted)"
		truncated = true
	}

	tail := ""
	if truncated {
		tail = "\n\nNote: the diff was larger than the configured limit and was truncated. Review only the visible portion."
	}

	return fmt.Sprintf(
		"PR #%d: %s\nAuthor: @%s\nBase: %s ← Head: %s\n\nDescription:\n%s\n\n--- DIFF ---\n%s%s",
		pr.Number,
		pr.Title,
		pr.User.Login,
		pr.Base.Ref,
		pr.Head.Ref,
		body,
		diff,
		tail,
	)
}

// changedFiles extracts the set of file paths from a unified diff. Used by
// the SkipPaths short-circuit. Looks for "diff --git a/<path> b/<path>"
// headers, which appear once per changed file.
func changedFiles(diff string) []string {
	var out []string
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		// Format: "diff --git a/<path-a> b/<path-b>"
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// Use the b/ side (the new path); strip the leading "b/".
		path := strings.TrimPrefix(fields[3], "b/")
		out = append(out, path)
	}
	return out
}

// matchesPathPrefix returns true if path is "under" the given prefix.
// Supports two simple forms:
//
//	"docs/"      → matches paths starting with "docs/"
//	"docs/**"    → equivalent to "docs/" (** is treated as recursive match)
//	"*.md"       → matches paths whose basename ends with ".md"
//
// More elaborate globs are out of scope; this is meant for the common
// "skip docs / vendor / generated" case.
func matchesPathPrefix(path, prefix string) bool {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return false
	}
	// Strip the trailing /** marker.
	prefix = strings.TrimSuffix(prefix, "/**")
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(path, prefix)
	}
	if strings.HasPrefix(prefix, "*.") {
		ext := prefix[1:] // ".md"
		return strings.HasSuffix(path, ext)
	}
	if strings.HasPrefix(path, prefix+"/") || path == prefix {
		return true
	}
	return false
}
