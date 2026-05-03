package webhookreceiver

import (
	"encoding/json"
	"fmt"
	"time"

	gh "github.com/45online/roster/internal/adapters/github"
)

// MapEventType translates the GitHub webhook X-GitHub-Event header value
// (e.g. "issues", "pull_request") to the equivalent type string the
// events API uses (e.g. "IssuesEvent", "PullRequestEvent"). This lets us
// reuse the existing poller dispatcher unchanged.
//
// Only the subset Roster cares about is supported; unknown types return
// the empty string and the caller should drop the event.
func MapEventType(headerValue string) string {
	switch headerValue {
	case "issues":
		return "IssuesEvent"
	case "pull_request":
		return "PullRequestEvent"
	}
	return ""
}

// senderActor pulls "sender.login" out of a GitHub webhook payload. It
// avoids unmarshalling the entire body into a strongly-typed struct
// (each event type is shaped differently) by going through map[string]any.
func senderActor(body []byte) (gh.User, error) {
	var raw struct {
		Sender struct {
			Login string `json:"login"`
		} `json:"sender"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return gh.User{}, fmt.Errorf("decode sender: %w", err)
	}
	return gh.User{Login: raw.Sender.Login}, nil
}

// BuildEvent constructs a gh.Event from a webhook delivery so it can be
// passed to the existing poller Handler signature unchanged.
//
// The webhook body IS the payload (no envelope), so we just adopt it
// verbatim as event.Payload. The poller-style envelope's ID / Type /
// Actor / CreatedAt fields are filled from the headers and the body.
func BuildEvent(deliveryID, ghEventType string, body []byte) (gh.Event, bool) {
	mapped := MapEventType(ghEventType)
	if mapped == "" {
		return gh.Event{}, false
	}
	actor, _ := senderActor(body) // a missing sender field just yields empty login
	return gh.Event{
		ID:        deliveryID,
		Type:      mapped,
		Actor:     actor,
		CreatedAt: time.Now().UTC(),
		Payload:   body,
	}, true
}
