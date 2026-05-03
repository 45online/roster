// Package webhookreceiver receives GitHub webhooks, verifies their HMAC
// signatures, maps them onto the same gh.Event shape that the poller
// already produces, and dispatches them through the same Handler.
//
// Webhook mode and polling mode are mutually exclusive — running both
// against the same repo would dispatch each event twice (the poller
// reads from /events and the webhook reads delivery push, with
// independent IDs that don't dedupe). takeover picks one based on
// config.
package webhookreceiver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// VerifySignature returns true when the X-Hub-Signature-256 header value
// matches HMAC-SHA256(secret, body). Empty secret + empty header is
// rejected (someone forgot to configure the secret on either side).
//
// The header format is "sha256=<hex>". Comparison is constant-time.
func VerifySignature(body []byte, secret, headerValue string) bool {
	if secret == "" || headerValue == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(headerValue, prefix) {
		return false
	}
	gotHex := headerValue[len(prefix):]
	gotBytes, err := hex.DecodeString(gotHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := mac.Sum(nil)
	return hmac.Equal(gotBytes, want)
}
