package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// PullRequest is the slice of GitHub's PR object Roster's modules need.
type PullRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	Draft   bool   `json:"draft"`
	HTMLURL string `json:"html_url"`
	User    User   `json:"user"`
	Head    Ref    `json:"head"`
	Base    Ref    `json:"base"`
}

// Ref is a branch ref (PR head or base).
type Ref struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo Repo   `json:"repo"`
}

// Repo is the minimal repo identifier on a Ref.
type Repo struct {
	FullName string `json:"full_name"`
}

// PullRequestEventPayload is the payload of a "PullRequestEvent".
type PullRequestEventPayload struct {
	Action      string      `json:"action"`
	Number      int         `json:"number"`
	PullRequest PullRequest `json:"pull_request"`
}

// DecodePullRequestPayload extracts a PullRequestEvent payload.
func (e *Event) DecodePullRequestPayload() (*PullRequestEventPayload, error) {
	if e.Type != "PullRequestEvent" {
		return nil, fmt.Errorf("not a PullRequestEvent (got %q)", e.Type)
	}
	var p PullRequestEventPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return nil, fmt.Errorf("decode PullRequestEvent payload: %w", err)
	}
	return &p, nil
}

// GetPullRequest fetches PR metadata.
func (c *Client) GetPullRequest(ctx context.Context, repo string, number int) (*PullRequest, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d", apiBase, repo, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github: GetPullRequest %s#%d: %s: %s", repo, number, resp.Status, strings.TrimSpace(string(body)))
	}

	var out PullRequest
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("github: decode PR: %w", err)
	}
	return &out, nil
}

// GetPullRequestDiff fetches the unified diff for a PR. Returns the raw
// diff string (may be large for big PRs).
func (c *Client) GetPullRequestDiff(ctx context.Context, repo string, number int) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d", apiBase, repo, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	c.setAuth(req)
	// Override the default Accept set by setAuth.
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github: GetPullRequestDiff %s#%d: %s: %s", repo, number, resp.Status, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("github: read diff: %w", err)
	}
	return string(body), nil
}

// ReviewEvent enumerates the three submission states GitHub accepts.
type ReviewEvent string

const (
	ReviewComment        ReviewEvent = "COMMENT"
	ReviewApprove        ReviewEvent = "APPROVE"
	ReviewRequestChanges ReviewEvent = "REQUEST_CHANGES"
)

// ReviewLineComment is a single line comment on a file in a PR review.
type ReviewLineComment struct {
	Path string `json:"path"`
	// Line is the line number in the file's RIGHT side of the diff (the
	// "after" version). Use Position for left-side comments.
	Line int    `json:"line,omitempty"`
	Body string `json:"body"`
}

// CreateReviewRequest is the input to CreateReview.
type CreateReviewRequest struct {
	Body     string              `json:"body"`
	Event    ReviewEvent         `json:"event"`
	Comments []ReviewLineComment `json:"comments,omitempty"`
}

// CreateReview posts a review on a PR.
func (c *Client) CreateReview(ctx context.Context, repo string, number int, r CreateReviewRequest) error {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d/reviews", apiBase, repo, number)
	payload, _ := json.Marshal(r)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github: CreateReview %s#%d: %s: %s", repo, number, resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}
