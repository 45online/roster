// Package github is a minimal REST client for the slice of the GitHub API
// that Roster's modules need: reading issues, posting comments, listing
// repository events. It intentionally avoids depending on a full SDK so
// the surface stays auditable.
package github

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

const apiBase = "https://api.github.com"

// Client is a GitHub REST client authenticated with a personal access token.
// It is intended to authenticate as a "virtual employee" account.
type Client struct {
	token string
	http  *http.Client
}

// NewClient returns a Client backed by the given PAT.
func NewClient(token string) *Client {
	return &Client{
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Issue is the subset of the GitHub Issue object Roster uses.
type Issue struct {
	Number  int     `json:"number"`
	Title   string  `json:"title"`
	Body    string  `json:"body"`
	State   string  `json:"state"`
	Labels  []Label `json:"labels"`
	User    User    `json:"user"`
	HTMLURL string  `json:"html_url"`
}

// Label is a GitHub issue label.
type Label struct {
	Name string `json:"name"`
}

// User is a GitHub user reference.
type User struct {
	Login string `json:"login"`
}

// GetIssue fetches a single issue by repo (owner/name) and number.
func (c *Client) GetIssue(ctx context.Context, repo string, number int) (*Issue, error) {
	url := fmt.Sprintf("%s/repos/%s/issues/%d", apiBase, repo, number)
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
		return nil, fmt.Errorf("github: GetIssue %s#%d: %s: %s", repo, number, resp.Status, strings.TrimSpace(string(body)))
	}

	var out Issue
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("github: decode issue: %w", err)
	}
	return &out, nil
}

// CreateComment posts a comment on an issue or pull request.
func (c *Client) CreateComment(ctx context.Context, repo string, number int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/issues/%d/comments", apiBase, repo, number)
	payload, _ := json.Marshal(map[string]string{"body": body})

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

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github: CreateComment %s#%d: %s: %s", repo, number, resp.Status, strings.TrimSpace(string(respBody)))
	}
	return nil
}

// AuthUser returns the authenticated user's login. Used for actor filtering
// (anti-loop): events whose actor matches this login are dropped before
// being routed to a module.
func (c *Client) AuthUser(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/user", nil)
	if err != nil {
		return "", err
	}
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github: AuthUser: %s", resp.Status)
	}

	var u User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return "", err
	}
	return u.Login, nil
}

func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}
