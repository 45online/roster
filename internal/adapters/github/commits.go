package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Commit is the slice of GitHub's commit object Roster's modules use.
type Commit struct {
	SHA       string       `json:"sha"`
	HTMLURL   string       `json:"html_url"`
	Commit    CommitDetail `json:"commit"`
	Author    *User        `json:"author"`
	Committer *User        `json:"committer"`
}

// CommitDetail is the embedded commit metadata (message, author, date).
type CommitDetail struct {
	Message string         `json:"message"`
	Author  CommitIdentity `json:"author"`
}

// CommitIdentity is a name+email+date commit signature.
type CommitIdentity struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// ShortMessage returns the first line of the commit message — what people
// usually see in oneline log output.
func (c Commit) ShortMessage() string {
	if i := strings.IndexByte(c.Commit.Message, '\n'); i >= 0 {
		return c.Commit.Message[:i]
	}
	return c.Commit.Message
}

// AuthorLogin returns the GitHub login if the commit was made by a known
// user, falling back to the commit-author name (an email-derived string).
func (c Commit) AuthorLogin() string {
	if c.Author != nil && c.Author.Login != "" {
		return c.Author.Login
	}
	return c.Commit.Author.Name
}

// ListCommits fetches commits to the default branch since the given time.
// Pagination is intentionally capped at 100 — Module D's lookback windows
// (1h, 6h) never approach that for any healthy repo.
func (c *Client) ListCommits(ctx context.Context, repo string, since time.Time) ([]Commit, error) {
	q := url.Values{}
	q.Set("per_page", "100")
	if !since.IsZero() {
		q.Set("since", since.UTC().Format(time.RFC3339))
	}

	apiURL := fmt.Sprintf("%s/repos/%s/commits?%s", apiBase, repo, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
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
		return nil, fmt.Errorf("github: ListCommits %s: %s: %s", repo, resp.Status, strings.TrimSpace(string(body)))
	}

	var out []Commit
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("github: decode commits: %w", err)
	}
	return out, nil
}

// MergedPR is the slice of a PR list response we use when filtering for
// recently-merged PRs.
type MergedPR struct {
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	HTMLURL  string     `json:"html_url"`
	User     User       `json:"user"`
	MergedAt *time.Time `json:"merged_at"`
}

// ListRecentMergedPulls fetches the most recently-updated closed PRs and
// filters down to those whose merged_at is after `since`. (GitHub's PR list
// endpoint sorts by update_at, not merge time, so we over-fetch and filter
// in code.)
func (c *Client) ListRecentMergedPulls(ctx context.Context, repo string, since time.Time, limit int) ([]MergedPR, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := url.Values{}
	q.Set("state", "closed")
	q.Set("sort", "updated")
	q.Set("direction", "desc")
	q.Set("per_page", fmt.Sprintf("%d", limit))

	apiURL := fmt.Sprintf("%s/repos/%s/pulls?%s", apiBase, repo, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
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
		return nil, fmt.Errorf("github: ListRecentMergedPulls %s: %s: %s", repo, resp.Status, strings.TrimSpace(string(body)))
	}

	var raw []MergedPR
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("github: decode PRs: %w", err)
	}

	out := make([]MergedPR, 0, len(raw))
	for _, p := range raw {
		if p.MergedAt == nil {
			continue // closed but not merged
		}
		if p.MergedAt.Before(since) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
