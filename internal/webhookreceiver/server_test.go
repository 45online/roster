package webhookreceiver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gh "github.com/45online/roster/internal/adapters/github"
)

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature_Match(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	if !VerifySignature(body, "shh", sign(body, "shh")) {
		t.Error("expected match")
	}
}

func TestVerifySignature_WrongSecret(t *testing.T) {
	body := []byte(`{}`)
	if VerifySignature(body, "right", sign(body, "wrong")) {
		t.Error("expected mismatch")
	}
}

func TestVerifySignature_TamperedBody(t *testing.T) {
	body := []byte(`original`)
	hdr := sign(body, "k")
	if VerifySignature([]byte(`tampered`), "k", hdr) {
		t.Error("tampered body must not verify")
	}
}

func TestVerifySignature_EmptyOrMalformed(t *testing.T) {
	body := []byte(`x`)
	cases := []struct {
		secret, header string
	}{
		{"", sign(body, "k")},     // empty secret
		{"k", ""},                 // empty header
		{"k", "md5=abcd"},         // wrong scheme
		{"k", "sha256=not-hex"},   // bad hex
	}
	for _, tc := range cases {
		if VerifySignature(body, tc.secret, tc.header) {
			t.Errorf("expected reject for secret=%q header=%q", tc.secret, tc.header)
		}
	}
}

func TestMapEventType(t *testing.T) {
	cases := map[string]string{
		"issues":       "IssuesEvent",
		"pull_request": "PullRequestEvent",
		"push":         "",
		"":             "",
	}
	for in, want := range cases {
		if got := MapEventType(in); got != want {
			t.Errorf("MapEventType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildEvent_PopulatesFields(t *testing.T) {
	body := []byte(`{"action":"opened","sender":{"login":"alice"},"issue":{"number":42}}`)
	ev, ok := BuildEvent("delivery-uuid-1", "issues", body)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.ID != "delivery-uuid-1" {
		t.Errorf("ID = %q", ev.ID)
	}
	if ev.Type != "IssuesEvent" {
		t.Errorf("Type = %q", ev.Type)
	}
	if ev.Actor.Login != "alice" {
		t.Errorf("Actor.Login = %q", ev.Actor.Login)
	}
	if !bytes.Equal(ev.Payload, body) {
		t.Errorf("Payload not preserved verbatim")
	}
	if ev.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestBuildEvent_RejectsUnknownType(t *testing.T) {
	if _, ok := BuildEvent("x", "push", []byte(`{}`)); ok {
		t.Error("push should not be supported")
	}
}

// --- HTTP handler integration ----------------------------------------

func newTestServer(t *testing.T, secret string, handler Handler) *httptest.Server {
	t.Helper()
	s, err := NewServer(Config{
		Listen:  "127.0.0.1:0",
		Secret:  secret,
		Handler: handler,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/github", s.handleWebhook)
	mux.HandleFunc("/healthz", s.handleHealth)
	return httptest.NewServer(mux)
}

func TestHandleWebhook_RejectsBadSignature(t *testing.T) {
	called := false
	srv := newTestServer(t, "secret", func(ctx context.Context, ev gh.Event) error {
		called = true
		return nil
	})
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/webhook/github", strings.NewReader(`{}`))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	req.Header.Set("X-GitHub-Event", "issues")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
	if called {
		t.Error("handler should not run on bad signature")
	}
}

func TestHandleWebhook_DispatchesIssuesEvent(t *testing.T) {
	body := []byte(`{"action":"opened","sender":{"login":"alice"},"issue":{"number":7,"title":"t"}}`)
	var seen gh.Event
	srv := newTestServer(t, "secret", func(ctx context.Context, ev gh.Event) error {
		seen = ev
		return nil
	})
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, "secret"))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "uuid-7")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if seen.Type != "IssuesEvent" {
		t.Errorf("Type = %q", seen.Type)
	}
	if seen.Actor.Login != "alice" {
		t.Errorf("Actor.Login = %q", seen.Actor.Login)
	}
	if seen.ID != "uuid-7" {
		t.Errorf("ID = %q", seen.ID)
	}
}

func TestHandleWebhook_PingReturnsPong(t *testing.T) {
	body := []byte(`{"zen":"hello"}`)
	srv := newTestServer(t, "secret", func(ctx context.Context, ev gh.Event) error {
		t.Fatal("ping should not dispatch")
		return nil
	})
	defer srv.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, "secret"))
	req.Header.Set("X-GitHub-Event", "ping")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestHandleWebhook_AntiLoop_DropsSelfEvents(t *testing.T) {
	body := []byte(`{"action":"opened","sender":{"login":"airouter-bot"},"issue":{"number":1}}`)
	called := false
	s, _ := NewServer(Config{
		Listen:    "127.0.0.1:0",
		Secret:    "secret",
		SelfLogin: "airouter-bot",
		Handler:   func(ctx context.Context, ev gh.Event) error { called = true; return nil },
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/github", s.handleWebhook)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, "secret"))
	req.Header.Set("X-GitHub-Event", "issues")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 even when dropping, got %d", resp.StatusCode)
	}
	if called {
		t.Error("self-event should not reach the dispatcher")
	}
}

func TestHandleWebhook_RejectsGet(t *testing.T) {
	srv := newTestServer(t, "secret", func(context.Context, gh.Event) error { return nil })
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/webhook/github")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestNewServer_ValidatesConfig(t *testing.T) {
	cases := map[string]Config{
		"missing secret":  {Listen: ":0", Handler: func(context.Context, gh.Event) error { return nil }},
		"missing handler": {Listen: ":0", Secret: "x"},
	}
	for name, c := range cases {
		if _, err := NewServer(c); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
