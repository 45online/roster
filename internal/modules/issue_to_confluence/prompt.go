package issue_to_confluence

import (
	"fmt"
	"html"
	"strings"

	gh "github.com/45online/roster/internal/adapters/github"
)

// summarizeSystemPrompt instructs Claude to extract a structured archive
// document from a closed-issue thread.
const summarizeSystemPrompt = `You are a documentation engineer. Given a CLOSED
GitHub issue thread (description + comments), produce a structured JSON
archival summary suitable for Confluence. Reply with ONLY a JSON object,
no commentary, no code fences. Schema:

{
  "title":    "<short factual title — re-phrase the issue title if it's vague>",
  "summary":  "<1–2 paragraphs: what problem this issue described>",
  "decision": "<what was decided / shipped to resolve it — pull from the comments>",
  "details":  "<technical details, edge cases, follow-ups, gotchas — bullet form is fine in markdown>",
  "pr_links": ["https://github.com/owner/repo/pull/123", ...]
}

Guidance:
- Pull "decision" from comments by maintainers (later comments outweigh earlier ones).
- Include only PR URLs that are explicitly referenced in the body or comments.
- "details" is markdown — fine to use bullet points, fenced code, links.
- Be concrete. Skip "see attached" / "as discussed" filler.
- If the thread is genuinely sparse, set details to "" rather than fabricating.
- The output MUST be valid JSON parseable by Go's encoding/json.`

// buildSummarizePrompt composes the user message: issue header + body +
// each comment as a quoted block.
func buildSummarizePrompt(repo string, issue *gh.Issue, comments []gh.IssueComment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository: %s\n", repo)
	fmt.Fprintf(&b, "Issue #%d: %s\n", issue.Number, issue.Title)
	fmt.Fprintf(&b, "Reporter: @%s\n", issue.User.Login)
	if labels := labelNames(issue); labels != "" {
		fmt.Fprintf(&b, "Labels: %s\n", labels)
	}
	fmt.Fprintf(&b, "URL: %s\n\n", issue.HTMLURL)

	body := strings.TrimSpace(issue.Body)
	if body == "" {
		body = "(no description)"
	}
	b.WriteString("--- ISSUE BODY ---\n")
	b.WriteString(body)
	b.WriteString("\n\n")

	if len(comments) == 0 {
		b.WriteString("--- COMMENTS ---\n(no comments)\n")
		return b.String()
	}

	b.WriteString("--- COMMENTS ---\n")
	for _, c := range comments {
		fmt.Fprintf(&b, "\n@%s wrote:\n", c.User.Login)
		body := strings.TrimSpace(c.Body)
		if body == "" {
			body = "(empty)"
		}
		// Light prefixing to make the boundary clear to the model.
		for _, line := range strings.Split(body, "\n") {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func labelNames(issue *gh.Issue) string {
	if len(issue.Labels) == 0 {
		return ""
	}
	names := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		names = append(names, l.Name)
	}
	return strings.Join(names, ", ")
}

// renderStorage builds the Confluence "storage" body (a restricted XHTML
// dialect) for the draft page. Layout:
//
//	<info>Auto-generated draft from GH issue …</info>
//	<h2>Summary</h2>      <p>{{Summary}}</p>
//	<h2>Decision</h2>     <p>{{Decision}}</p>
//	<h2>Details</h2>      {{Details as <p>}}
//	<h2>References</h2>   <ul><li>GitHub issue, PRs…</li></ul>
//
// We don't try to translate Markdown perfectly — Confluence accepts plain
// XHTML well enough that most readers won't notice.
func renderStorage(doc *Document, issue *gh.Issue) string {
	var b strings.Builder

	b.WriteString(`<ac:structured-macro ac:name="info" ac:schema-version="1"><ac:rich-text-body>`)
	fmt.Fprintf(&b,
		`<p>This page is an auto-generated draft from GitHub issue <a href="%s">%s#%d</a>. Review before publishing.</p>`,
		html.EscapeString(issue.HTMLURL), html.EscapeString(repoFromURL(issue.HTMLURL)), issue.Number,
	)
	b.WriteString(`</ac:rich-text-body></ac:structured-macro>`)

	if s := strings.TrimSpace(doc.Summary); s != "" {
		b.WriteString("<h2>Summary</h2>")
		writeParagraphs(&b, s)
	}
	if s := strings.TrimSpace(doc.Decision); s != "" {
		b.WriteString("<h2>Decision</h2>")
		writeParagraphs(&b, s)
	}
	if s := strings.TrimSpace(doc.Details); s != "" {
		b.WriteString("<h2>Details</h2>")
		writeParagraphs(&b, s)
	}

	b.WriteString("<h2>References</h2><ul>")
	fmt.Fprintf(&b, `<li><a href="%s">GitHub issue %s#%d</a></li>`,
		html.EscapeString(issue.HTMLURL),
		html.EscapeString(repoFromURL(issue.HTMLURL)),
		issue.Number)
	for _, link := range doc.PRLinks {
		link = strings.TrimSpace(link)
		if link == "" {
			continue
		}
		fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, html.EscapeString(link), html.EscapeString(link))
	}
	b.WriteString("</ul>")

	return b.String()
}

func writeParagraphs(b *strings.Builder, s string) {
	for _, para := range strings.Split(s, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		// Treat any single newlines inside the paragraph as <br/>.
		fmt.Fprintf(b, "<p>%s</p>", strings.ReplaceAll(html.EscapeString(para), "\n", "<br/>"))
	}
}

// repoFromURL extracts "owner/name" from a GitHub URL like
// "https://github.com/owner/name/issues/42". Returns "" if not parseable.
func repoFromURL(u string) string {
	const prefix = "https://github.com/"
	if !strings.HasPrefix(u, prefix) {
		return ""
	}
	rest := u[len(prefix):]
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}
