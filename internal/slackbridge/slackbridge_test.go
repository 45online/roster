package slackbridge

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func sign(body []byte, ts, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "v0:%s:%s", ts, body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// ── signature.go ─────────────────────────────────────────────────────

func TestVerifySignature_Match(t *testing.T) {
	body := []byte(`text=hello`)
	ts := "1700000000"
	now := time.Unix(1700000005, 0)
	if err := VerifySignature(body, "shh", sign(body, ts, "shh"), ts, now, 0); err != nil {
		t.Errorf("expected match, got %v", err)
	}
}

func TestVerifySignature_WrongSecret(t *testing.T) {
	body := []byte(`x`)
	ts := "1700000000"
	now := time.Unix(1700000000, 0)
	err := VerifySignature(body, "right", sign(body, ts, "wrong"), ts, now, 0)
	if err == nil {
		t.Error("expected mismatch")
	}
}

func TestVerifySignature_StaleTimestamp(t *testing.T) {
	body := []byte(`x`)
	ts := "1700000000"
	now := time.Unix(1700000000+10*60, 0) // 10 min later, default skew is 5
	err := VerifySignature(body, "k", sign(body, ts, "k"), ts, now, 0)
	if err == nil || !strings.Contains(err.Error(), "replay window") {
		t.Errorf("expected replay-window error, got %v", err)
	}
}

func TestVerifySignature_BadScheme(t *testing.T) {
	body := []byte(`x`)
	ts := "1700000000"
	now := time.Unix(1700000000, 0)
	err := VerifySignature(body, "k", "v1=abcd", ts, now, 0)
	if err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Errorf("expected scheme error, got %v", err)
	}
}

func TestVerifySignature_EmptyInputs(t *testing.T) {
	body := []byte(`x`)
	ts := "1700000000"
	now := time.Unix(1700000000, 0)
	cases := []struct {
		secret, sig, ts string
	}{
		{"", sign(body, ts, "k"), ts},
		{"k", "", ts},
		{"k", sign(body, ts, "k"), ""},
	}
	for _, tc := range cases {
		if err := VerifySignature(body, tc.secret, tc.sig, tc.ts, now, 0); err == nil {
			t.Errorf("expected reject for empty input %+v", tc)
		}
	}
}

// ── parser.go ────────────────────────────────────────────────────────

func TestParse_HappyPaths(t *testing.T) {
	cases := []struct {
		text     string
		wantVerb string
		wantRepo string
		wantNum  int
	}{
		{"help", "help", "", 0},
		{"", "help", "", 0}, // empty text → help
		{"status", "status", "", 0},
		{"sync-issue acme/backend#42", "sync-issue", "acme/backend", 42},
		{"sync-issue acme/backend 42", "sync-issue", "acme/backend", 42},
		{"sync-issue acme/backend  #42", "sync-issue", "acme/backend", 42},
		{"review-pr foo/bar#7", "review-pr", "foo/bar", 7},
		{"archive-issue x/y#1", "archive-issue", "x/y", 1},
		{"SYNC-ISSUE acme/x#5", "sync-issue", "acme/x", 5}, // case-insensitive verb
	}
	for _, tc := range cases {
		got, err := Parse(tc.text, "U1", "alice", "C1")
		if err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", tc.text, err)
			continue
		}
		if got.Verb != tc.wantVerb || got.Repo != tc.wantRepo || got.Number != tc.wantNum {
			t.Errorf("Parse(%q): got %+v want verb=%q repo=%q num=%d",
				tc.text, got, tc.wantVerb, tc.wantRepo, tc.wantNum)
		}
	}
}

func TestParse_Errors(t *testing.T) {
	cases := []string{
		"sync-issue",                // missing target
		"review-pr foo/bar",         // missing number
		"sync-issue notarepo#42",    // repo without slash
		"sync-issue foo/bar#abc",    // non-int number
		"sync-issue foo/bar#0",      // zero
		"sync-issue foo/bar#-1",     // negative
		"unknown-verb foo/bar#1",
	}
	for _, in := range cases {
		if _, err := Parse(in, "U", "alice", "C"); err == nil {
			t.Errorf("Parse(%q): expected error", in)
		}
	}
}

// ── handler.go ────────────────────────────────────────────────────────

type fakeDispatcher struct {
	syncCalls    int
	reviewCalls  int
	archiveCalls int
	statusCalls  int
}

func (f *fakeDispatcher) SyncIssue(ctx context.Context, repo string, n int) (string, error) {
	f.syncCalls++
	return fmt.Sprintf("ABC-%d", n), nil
}
func (f *fakeDispatcher) ReviewPR(ctx context.Context, repo string, n int) (string, error) {
	f.reviewCalls++
	return "comment", nil
}
func (f *fakeDispatcher) ArchiveIssue(ctx context.Context, repo string, n int) (string, error) {
	f.archiveCalls++
	return "page-id", nil
}
func (f *fakeDispatcher) Status(ctx context.Context) (string, error) {
	f.statusCalls++
	return "ok", nil
}

func newSlackRequest(t *testing.T, secret, body string) *http.Request {
	t.Helper()
	// Use the current epoch so handler's time.Now() comparison stays
	// within the default replay window without needing a huge Skew.
	ts := fmt.Sprintf("%d", time.Now().Unix())
	req := httptest.NewRequest("POST", "/slack/command", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sign([]byte(body), ts, secret))
	return req
}

func TestHandler_StatusInline(t *testing.T) {
	d := &fakeDispatcher{}
	h := &Handler{
		Secret:     "shh",
		Skew:       1000 * time.Hour, // make the test forever-fresh
		Dispatcher: d,
	}
	form := url.Values{
		"text":       {"status"},
		"user_id":    {"U1"},
		"user_name":  {"alice"},
		"channel_id": {"C1"},
	}.Encode()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newSlackRequest(t, "shh", form))

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if d.statusCalls != 1 {
		t.Errorf("expected 1 Status call, got %d", d.statusCalls)
	}
	if !strings.Contains(w.Body.String(), `"text"`) {
		t.Errorf("body should be Slack JSON: %s", w.Body.String())
	}
}

func TestHandler_SyncIssueIsAsync(t *testing.T) {
	d := &fakeDispatcher{}
	h := &Handler{
		Secret:     "shh",
		Skew:       1000 * time.Hour,
		Dispatcher: d,
	}
	form := url.Values{
		"text":       {"sync-issue acme/x#42"},
		"user_id":    {"U1"},
		"user_name":  {"alice"},
		"channel_id": {"C1"},
	}.Encode()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newSlackRequest(t, "shh", form))

	// Immediate response (handler doesn't wait on goroutine).
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "queued") {
		t.Errorf("expected 'queued' ack, got: %s", w.Body.String())
	}
	// The async work eventually runs; small wait + assert.
	time.Sleep(50 * time.Millisecond)
	if d.syncCalls != 1 {
		t.Errorf("expected 1 SyncIssue call, got %d", d.syncCalls)
	}
}

func TestHandler_BadSignatureRejected(t *testing.T) {
	d := &fakeDispatcher{}
	h := &Handler{Secret: "right", Dispatcher: d}
	body := "text=help"
	req := httptest.NewRequest("POST", "/slack/command", strings.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", "1700000000")
	req.Header.Set("X-Slack-Signature", "v0=deadbeef")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if d.syncCalls+d.statusCalls+d.reviewCalls+d.archiveCalls != 0 {
		t.Error("dispatcher must not run on bad signature")
	}
}

func TestHandler_RejectsGet(t *testing.T) {
	d := &fakeDispatcher{}
	h := &Handler{Secret: "x", Dispatcher: d}
	req := httptest.NewRequest("GET", "/slack/command", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandler_HelpForEmptyText(t *testing.T) {
	d := &fakeDispatcher{}
	h := &Handler{Secret: "shh", Skew: 1000 * time.Hour, Dispatcher: d}
	form := url.Values{"text": {""}}.Encode()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newSlackRequest(t, "shh", form))
	if !strings.Contains(w.Body.String(), "/roster status") {
		t.Errorf("expected help text, got: %s", w.Body.String())
	}
}

func TestHandler_ParseErrorIsReturnedToUser(t *testing.T) {
	d := &fakeDispatcher{}
	h := &Handler{Secret: "shh", Skew: 1000 * time.Hour, Dispatcher: d}
	form := url.Values{"text": {"sync-issue notarepo"}}.Encode()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, newSlackRequest(t, "shh", form))
	if w.Code != http.StatusOK {
		t.Errorf("parse errors should still be 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "owner/name") {
		t.Errorf("expected helpful error message, got: %s", w.Body.String())
	}
}
