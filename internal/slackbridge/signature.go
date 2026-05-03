// Package slackbridge accepts Slack slash commands and routes them to
// Roster modules. The transport contract follows Slack's standard:
//
//   - HMAC-SHA256 signature in X-Slack-Signature ("v0=<hex>")
//   - Timestamp in X-Slack-Request-Timestamp (replay window: 5 min)
//   - Body: application/x-www-form-urlencoded
//   - Slack expects 200 within 3s; long work runs as a goroutine and
//     surfaces back through GitHub / Jira / Confluence rather than
//     using response_url callbacks (keeps the path simple).
package slackbridge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VerifySignature reproduces Slack's v0 signing algorithm.
//
//	sig_basestring = "v0:" + ts + ":" + body
//	expected       = "v0=" + hex(HMAC-SHA256(secret, sig_basestring))
//	→ constant-time compare against X-Slack-Signature
//
// `now` is parameterised for tests; pass time.Now() in production.
//
// Rejects:
//   - empty secret / empty headers
//   - timestamp older than skew (5 min)
//   - signature with the wrong scheme prefix
//   - any HMAC mismatch
func VerifySignature(body []byte, secret, sigHeader, tsHeader string, now time.Time, skew time.Duration) error {
	if secret == "" {
		return fmt.Errorf("empty signing secret")
	}
	if sigHeader == "" || tsHeader == "" {
		return fmt.Errorf("missing X-Slack-Signature or X-Slack-Request-Timestamp")
	}

	tsInt, err := strconv.ParseInt(tsHeader, 10, 64)
	if err != nil {
		return fmt.Errorf("malformed timestamp: %w", err)
	}
	ts := time.Unix(tsInt, 0)
	delta := now.Sub(ts)
	if delta < 0 {
		delta = -delta
	}
	if skew == 0 {
		skew = 5 * time.Minute
	}
	if delta > skew {
		return fmt.Errorf("timestamp outside replay window (%s)", delta)
	}

	const prefix = "v0="
	if !strings.HasPrefix(sigHeader, prefix) {
		return fmt.Errorf("unexpected signature scheme")
	}
	gotBytes, err := hex.DecodeString(sigHeader[len(prefix):])
	if err != nil {
		return fmt.Errorf("malformed hex in signature")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "v0:%s:%s", tsHeader, body)
	want := mac.Sum(nil)
	if !hmac.Equal(gotBytes, want) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}
