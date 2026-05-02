// Package confluence is a minimal Confluence Cloud REST API v2 client,
// covering the slice Roster's modules need: creating draft pages.
// Authentication is HTTP basic with email + API token (same scheme as Jira).
package confluence

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

// Client is a Confluence v2 REST client.
type Client struct {
	baseURL string // site root, e.g. https://yourdomain.atlassian.net
	email   string
	token   string
	http    *http.Client
}

// NewClient creates a Confluence client. baseURL is the Atlassian site root,
// not the /wiki endpoint — the client appends "/wiki" itself.
func NewClient(baseURL, email, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		email:   email,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// CreateDraftRequest holds fields for creating a draft page.
type CreateDraftRequest struct {
	// SpaceID is the numeric ID of the space the page lives in.
	// (Get it from /wiki/api/v2/spaces — space *keys* aren't accepted by v2.)
	SpaceID string
	// ParentID is optional: the numeric ID of the parent page.
	ParentID string
	Title    string
	// BodyStorage is the page body in Confluence "storage" representation
	// (a restricted XHTML dialect; plain HTML mostly works).
	BodyStorage string
}

// CreateDraftResponse is the slice of the create response we use.
type CreateDraftResponse struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Links  struct {
		WebUI string `json:"webui"`
		Base  string `json:"base"`
	} `json:"_links"`
}

// URL returns a direct browser URL to the page.
func (r *CreateDraftResponse) URL() string {
	if r.Links.Base != "" && r.Links.WebUI != "" {
		return r.Links.Base + r.Links.WebUI
	}
	return ""
}

// CreateDraft creates a page with status="draft" — it won't be visible in
// search or to non-owners until someone publishes it via the UI.
func (c *Client) CreateDraft(ctx context.Context, req CreateDraftRequest) (*CreateDraftResponse, error) {
	if req.SpaceID == "" {
		return nil, fmt.Errorf("confluence: SpaceID is required")
	}
	if req.Title == "" {
		return nil, fmt.Errorf("confluence: Title is required")
	}

	payload := map[string]interface{}{
		"spaceId": req.SpaceID,
		"status":  "draft",
		"title":   req.Title,
		"body": map[string]string{
			"representation": "storage",
			"value":          req.BodyStorage,
		},
	}
	if req.ParentID != "" {
		payload["parentId"] = req.ParentID
	}

	data, _ := json.Marshal(payload)
	url := c.baseURL + "/wiki/api/v2/pages"

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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("confluence: CreateDraft: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out CreateDraftResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("confluence: decode response: %w", err)
	}
	return &out, nil
}
