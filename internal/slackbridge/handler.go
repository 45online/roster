package slackbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Dispatcher is implemented by code that knows how to actually run the
// modules. takeover constructs one closing over already-wired
// modA / modB / modC instances. Slack expects a 3-second response, so
// each method returns a short status string and starts the real work
// in a goroutine.
type Dispatcher interface {
	SyncIssue(ctx context.Context, repo string, number int) (string, error)
	ReviewPR(ctx context.Context, repo string, number int) (string, error)
	ArchiveIssue(ctx context.Context, repo string, number int) (string, error)
	Status(ctx context.Context) (string, error)
}

// Handler is the http.Handler that verifies the signature, parses, and
// dispatches.
type Handler struct {
	Secret     string        // Slack signing secret (workspace-level)
	Skew       time.Duration // 0 → 5 min default
	Dispatcher Dispatcher
	Logger     *log.Logger
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Logger == nil {
		h.Logger = log.Default()
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	// Cap the body — slash-command payloads are tiny.
	const maxBody = 64 * 1024
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if len(body) > maxBody {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err := VerifySignature(body,
		h.Secret,
		r.Header.Get("X-Slack-Signature"),
		r.Header.Get("X-Slack-Request-Timestamp"),
		time.Now(),
		h.Skew,
	); err != nil {
		h.Logger.Printf("[slack] reject: %v (from %s)", err, r.RemoteAddr)
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	form, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	cmd, err := Parse(
		form.Get("text"),
		form.Get("user_id"),
		form.Get("user_name"),
		form.Get("channel_id"),
	)
	if err != nil {
		respond(w, ":warning: "+err.Error())
		return
	}

	switch cmd.Verb {
	case "help":
		respond(w, HelpText)
		return
	case "status":
		// Status is fast (file scan), so do it inline.
		out, err := h.Dispatcher.Status(r.Context())
		if err != nil {
			respond(w, ":warning: status: "+err.Error())
			return
		}
		respond(w, out)
		return
	case "sync-issue", "review-pr", "archive-issue":
		// Real work; defer to a goroutine and return an immediate ack.
		go h.runAsync(*cmd)
		respond(w, fmt.Sprintf(":hourglass_flowing_sand: queued `%s %s#%d` — results will appear in GitHub / Jira / Confluence directly.",
			cmd.Verb, cmd.Repo, cmd.Number))
		return
	}
	respond(w, ":warning: nothing handled this command — that's a bug")
}

// runAsync is the goroutine that handles a long-running module call. We
// detach from the request context (Slack has already gone) but cap the
// total time so a stuck handler can't leak a goroutine forever.
func (h *Handler) runAsync(cmd Command) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var (
		out string
		err error
	)
	switch cmd.Verb {
	case "sync-issue":
		out, err = h.Dispatcher.SyncIssue(ctx, cmd.Repo, cmd.Number)
	case "review-pr":
		out, err = h.Dispatcher.ReviewPR(ctx, cmd.Repo, cmd.Number)
	case "archive-issue":
		out, err = h.Dispatcher.ArchiveIssue(ctx, cmd.Repo, cmd.Number)
	}
	if err != nil {
		h.Logger.Printf("[slack-async] %s %s#%d → error: %v (by @%s)",
			cmd.Verb, cmd.Repo, cmd.Number, err, cmd.UserName)
		return
	}
	h.Logger.Printf("[slack-async] %s %s#%d → %s (by @%s)",
		cmd.Verb, cmd.Repo, cmd.Number, out, cmd.UserName)
}

// respond writes a Slack-shaped JSON acknowledgement. Slack will surface
// it inline in the channel where the slash command was invoked
// ("response_type" defaults to "ephemeral" — only the issuer sees it).
func respond(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	body, _ := json.Marshal(map[string]string{"text": strings.TrimSpace(text)})
	_, _ = w.Write(body)
}
