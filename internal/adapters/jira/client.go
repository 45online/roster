// Package jira is a minimal REST client for Jira Cloud (REST API v3),
// covering the subset Roster's modules need: creating issues. Authentication
// is HTTP basic with email + API token.
package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a Jira REST API v3 client.
type Client struct {
	baseURL string
	email   string
	token   string
	http    *http.Client
}

// NewClient creates a Jira client. baseURL is the site root, e.g.
// "https://yourdomain.atlassian.net".
func NewClient(baseURL, email, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		email:   email,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// CreateIssueRequest is the input to CreateIssue.
type CreateIssueRequest struct {
	Project     string // project key, e.g. "ROSTER"
	Summary     string
	Description string
	IssueType   string // e.g. "Task", "Bug", "Story"
	Priority    string // e.g. "Highest", "High" — empty to skip
}

// CreateIssueResponse is the slice of the Jira create response we use.
type CreateIssueResponse struct {
	ID  string `json:"id"`
	Key string `json:"key"`
	URL string `json:"-"`
}

// CreateIssue creates a new Jira issue and returns its key (e.g. "ROSTER-42").
func (c *Client) CreateIssue(ctx context.Context, req CreateIssueRequest) (*CreateIssueResponse, error) {
	fields := map[string]interface{}{
		"project":     map[string]string{"key": req.Project},
		"summary":     req.Summary,
		"issuetype":   map[string]string{"name": req.IssueType},
		"description": adfText(req.Description),
	}
	if req.Priority != "" {
		fields["priority"] = map[string]string{"name": req.Priority}
	}
	payload := map[string]interface{}{"fields": fields}

	data, _ := json.Marshal(payload)
	url := c.baseURL + "/rest/api/3/issue"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.SetBasicAuth(c.email, c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jira: CreateIssue: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out CreateIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("jira: decode response: %w", err)
	}
	out.URL = fmt.Sprintf("%s/browse/%s", c.baseURL, out.Key)
	return &out, nil
}

// adfText wraps a plain string into an Atlassian Document Format paragraph,
// which Jira REST v3 requires for the description field.
func adfText(s string) map[string]interface{} {
	if s == "" {
		s = "(no description)"
	}
	return map[string]interface{}{
		"type":    "doc",
		"version": 1,
		"content": []interface{}{
			map[string]interface{}{
				"type": "paragraph",
				"content": []interface{}{
					map[string]string{"type": "text", "text": s},
				},
			},
		},
	}
}
