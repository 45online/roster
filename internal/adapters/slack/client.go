// Package slack is a minimal Slack Web API client, covering the slice
// Roster's modules need: posting messages to a channel.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBase = "https://slack.com/api"

// Client is a Slack Web API client authenticated with a user OAuth token
// (xoxp-...) or a bot token (xoxb-...).
type Client struct {
	token string
	http  *http.Client
}

// NewClient creates a Slack client backed by the given token.
func NewClient(token string) *Client {
	return &Client{
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

// PostMessageRequest configures a single chat.postMessage call.
type PostMessageRequest struct {
	// Channel is a channel ID (preferred, e.g. "C012ABCD") or name with a
	// leading # ("#alerts").
	Channel string
	// Text is the message body. Required even when blocks are provided
	// (used as the notification fallback).
	Text string
}

// PostMessageResponse is the slice of the chat.postMessage response we use.
type PostMessageResponse struct {
	OK      bool   `json:"ok"`
	Channel string `json:"channel"`
	Ts      string `json:"ts"`
	Error   string `json:"error,omitempty"`
}

// PostMessage posts a message to a Slack channel.
func (c *Client) PostMessage(ctx context.Context, req PostMessageRequest) (*PostMessageResponse, error) {
	if req.Channel == "" {
		return nil, fmt.Errorf("slack: Channel is required")
	}
	if req.Text == "" {
		return nil, fmt.Errorf("slack: Text is required")
	}

	payload, _ := json.Marshal(map[string]string{
		"channel": req.Channel,
		"text":    req.Text,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/chat.postMessage", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var out PostMessageResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("slack: decode response: %w (raw: %s)", err, string(body))
	}
	if !out.OK {
		return &out, fmt.Errorf("slack: chat.postMessage failed: %s", out.Error)
	}
	return &out, nil
}
