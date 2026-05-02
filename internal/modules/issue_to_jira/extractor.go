package issue_to_jira

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/api"
)

// DefaultExtractorModel is the model used if NewExtractor is given an
// empty string. Haiku is the right tradeoff for this task — small,
// structured-output, latency-sensitive.
const DefaultExtractorModel = "claude-haiku-4-5-20251001"

// ExtractedFields is the structured output Claude returns when asked to
// analyse a GitHub issue for Jira ingestion.
type ExtractedFields struct {
	// Summary is a concise one-line summary suitable for the Jira "summary"
	// field. <= 240 characters.
	Summary string `json:"summary"`
	// IssueType must be one of: "Bug", "Task", "Story", "Epic".
	IssueType string `json:"issue_type"`
	// Priority must be one of: "Highest", "High", "Medium", "Low", "Lowest".
	// Empty string means Claude couldn't infer one.
	Priority string `json:"priority"`
	// Component is an optional area/module tag (e.g. "auth", "billing").
	// Empty string is fine.
	Component string `json:"component"`
}

// Extractor uses Claude to derive structured Jira fields from a GitHub issue.
type Extractor struct {
	client api.Client
	model  string
}

// NewExtractor builds an Extractor. If model is "", DefaultExtractorModel
// is used.
func NewExtractor(client api.Client, model string) *Extractor {
	if model == "" {
		model = DefaultExtractorModel
	}
	return &Extractor{client: client, model: model}
}

// Extract sends the issue to Claude and returns the parsed fields.
// On any failure (network, invalid JSON) it returns the error so the
// caller can decide whether to fall back to mechanical mapping.
func (e *Extractor) Extract(ctx context.Context, issue *github.Issue) (*ExtractedFields, error) {
	userPrompt, err := buildExtractorPrompt(issue)
	if err != nil {
		return nil, err
	}
	contentJSON, _ := json.Marshal(userPrompt)

	req := &api.MessageRequest{
		Model:     e.model,
		MaxTokens: 1024,
		System:    extractorSystemPrompt,
		Messages: []api.MessageParam{
			{Role: "user", Content: contentJSON},
		},
		QuerySource: "background",
	}

	resp, err := e.client.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("claude complete: %w", err)
	}

	text := firstTextBlock(resp.Content)
	if text == "" {
		return nil, fmt.Errorf("claude returned no text content")
	}

	jsonBody := stripCodeFence(text)
	var fields ExtractedFields
	if err := json.Unmarshal([]byte(jsonBody), &fields); err != nil {
		return nil, fmt.Errorf("decode claude json: %w (raw: %q)", err, jsonBody)
	}
	return &fields, nil
}

// extractorSystemPrompt is the constant system message instructing Claude
// how to behave for this task.
const extractorSystemPrompt = `You are a Jira ticket triage assistant. Given a GitHub issue you must
extract concise structured fields suitable for filing a Jira ticket.

Reply with ONLY a JSON object — no commentary, no code fences. Schema:

{
  "summary":    "<= 240 chars, no trailing punctuation, no leading 'Bug:' / 'Feature:' prefixes",
  "issue_type": "Bug" | "Task" | "Story" | "Epic",
  "priority":   "Highest" | "High" | "Medium" | "Low" | "Lowest" | "",
  "component":  "<short module/area tag like auth, billing, ui — empty if unclear>"
}

Rules:
- "issue_type": pick "Bug" when the report describes broken or unexpected behaviour;
  "Story" for user-facing feature requests; "Epic" for large multi-step initiatives;
  default to "Task" otherwise.
- "priority": only emit a value if the issue text or labels clearly justify it
  (e.g. "production down" → Highest; "nice to have" → Low). When unsure, return "".
- "summary": rewrite the title for clarity if needed; do NOT copy the raw GitHub title verbatim if it's vague.
- "component": one word lowercase, derived from the body; "" if you can't tell.
- The output MUST be valid JSON parseable by Go's encoding/json.`

func buildExtractorPrompt(issue *github.Issue) (string, error) {
	if issue == nil {
		return "", fmt.Errorf("nil issue")
	}
	labels := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, l.Name)
	}
	body := strings.TrimSpace(issue.Body)
	if body == "" {
		body = "(no description)"
	}
	// Cap body at 8 KB to control token spend on pathological issues.
	const maxBody = 8 * 1024
	if len(body) > maxBody {
		body = body[:maxBody] + "\n…(truncated)"
	}

	return fmt.Sprintf(
		"GitHub issue to triage:\n\nTitle: %s\nLabels: %s\nReporter: @%s\n\nBody:\n%s",
		issue.Title,
		strings.Join(labels, ", "),
		issue.User.Login,
		body,
	), nil
}

// firstTextBlock returns the text of the first "text"-type content block,
// or "" if none.
func firstTextBlock(blocks []api.ContentBlock) string {
	for _, b := range blocks {
		if b.Type == "text" {
			return b.Text
		}
	}
	return ""
}

// stripCodeFence removes a leading ```json … ``` wrapper if Claude emits one
// despite instructions. Tolerant: returns input unchanged if no fence found.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line.
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	// Drop the trailing fence.
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
