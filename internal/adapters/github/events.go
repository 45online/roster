package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Event is the slice of a GitHub event Roster cares about. The API returns
// many event types; we only deserialize fields common to all and pull
// type-specific bits via Payload.
type Event struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Actor     User            `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

// IssuesEventPayload is the payload of an "IssuesEvent".
type IssuesEventPayload struct {
	Action string `json:"action"`
	Issue  Issue  `json:"issue"`
}

// DecodeIssuesPayload extracts an IssuesEvent payload from the raw event.
// Returns an error if the event is not an IssuesEvent.
func (e *Event) DecodeIssuesPayload() (*IssuesEventPayload, error) {
	if e.Type != "IssuesEvent" {
		return nil, fmt.Errorf("not an IssuesEvent (got %q)", e.Type)
	}
	var p IssuesEventPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return nil, fmt.Errorf("decode IssuesEvent payload: %w", err)
	}
	return &p, nil
}

// ListEventsResponse pairs the event slice with conditional-fetch metadata.
type ListEventsResponse struct {
	Events       []Event
	ETag         string        // pass back as IfNoneMatch on the next call
	PollInterval time.Duration // GitHub-suggested minimum poll interval
	NotModified  bool          // true if the server returned 304
}

// ListEvents fetches recent events for a repository, optionally short-
// circuiting with an ETag to save rate-limit quota. When the ETag matches,
// the response NotModified flag is true and Events is empty.
//
// The GitHub events endpoint returns events in reverse chronological order
// (newest first) and only the last ~90 events / 300 events depending on
// repo size; the caller is responsible for not falling behind.
func (c *Client) ListEvents(ctx context.Context, repo, ifNoneMatch string) (*ListEventsResponse, error) {
	url := fmt.Sprintf("%s/repos/%s/events", apiBase, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	out := &ListEventsResponse{
		ETag:         resp.Header.Get("ETag"),
		PollInterval: parsePollInterval(resp.Header.Get("X-Poll-Interval")),
	}

	switch resp.StatusCode {
	case http.StatusNotModified:
		out.NotModified = true
		return out, nil
	case http.StatusOK:
		// proceed
	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github: ListEvents %s: %s: %s", repo, resp.Status, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(&out.Events); err != nil {
		return nil, fmt.Errorf("github: decode events: %w", err)
	}
	return out, nil
}

func parsePollInterval(v string) time.Duration {
	if v == "" {
		return 0
	}
	var seconds int
	if _, err := fmt.Sscanf(v, "%d", &seconds); err != nil {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
