package webhookreceiver

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	gh "github.com/45online/roster/internal/adapters/github"
)

// Handler matches internal/poller.Handler so the dispatcher in takeover
// can be wired to either source unchanged.
type Handler func(ctx context.Context, ev gh.Event) error

// Config configures the embedded HTTP server.
type Config struct {
	// Listen is the addr passed to net/http (e.g. ":8080", "127.0.0.1:9090").
	Listen string
	// Path is the URL path that receives POSTs from GitHub. Default
	// "/webhook/github".
	Path string
	// Secret is the shared HMAC secret configured on the GitHub repo
	// webhook. Required — empty rejects every incoming request.
	Secret string
	// Handler is the dispatch callback (same signature poller uses).
	Handler Handler
	// Logger is optional.
	Logger *log.Logger
	// SelfLogin is the virtual employee account login. Events whose
	// sender matches this login are dropped (anti-loop), mirroring
	// the poller behaviour.
	SelfLogin string
	// ExtraRoutes lets the caller attach side-channel handlers on the
	// same listener — e.g. a Slack slash-command receiver. Keys are URL
	// paths (e.g. "/slack/command"). Nil values are ignored.
	ExtraRoutes map[string]http.Handler
}

// Server wraps net/http. Use NewServer + Run.
type Server struct {
	cfg    Config
	logger *log.Logger
	srv    *http.Server
}

// NewServer validates config and returns a ready-to-Run server.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.Path == "" {
		cfg.Path = "/webhook/github"
	}
	if cfg.Secret == "" {
		return nil, errors.New("webhookreceiver: Secret is required")
	}
	if cfg.Handler == nil {
		return nil, errors.New("webhookreceiver: Handler is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	mux := http.NewServeMux()
	s := &Server{cfg: cfg, logger: cfg.Logger}
	mux.HandleFunc(cfg.Path, s.handleWebhook)
	mux.HandleFunc("/healthz", s.handleHealth)
	for path, h := range cfg.ExtraRoutes {
		if h == nil || path == "" {
			continue
		}
		mux.Handle(path, h)
	}
	s.srv = &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
	}
	return s, nil
}

// Run blocks listening for requests until ctx is cancelled or the
// server errors. ctx.Cancel triggers a graceful shutdown.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Printf("[webhook] listening on %s%s", s.cfg.Listen, s.cfg.Path)

	errCh := make(chan error, 1)
	go func() {
		err := s.srv.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
		s.logger.Printf("[webhook] shutting down (%v)", ctx.Err())
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	// Cap the request body — a runaway POST shouldn't OOM the daemon.
	const maxBody = 5 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if len(body) > maxBody {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if !VerifySignature(body, s.cfg.Secret, r.Header.Get("X-Hub-Signature-256")) {
		s.logger.Printf("[webhook] rejected: bad/missing signature from %s", r.RemoteAddr)
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	ghType := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if ghType == "ping" {
		// GitHub sends ping on webhook setup — return 200 so the UI
		// shows green.
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "pong\n")
		return
	}

	ev, ok := BuildEvent(deliveryID, ghType, body)
	if !ok {
		// Subscribed event type we don't handle (e.g. "push"). 200 to
		// stop GitHub retrying; we just don't act on it.
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ignored\n")
		return
	}

	// Anti-loop — same rule the poller applies.
	if s.cfg.SelfLogin != "" && ev.Actor.Login == s.cfg.SelfLogin {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "self-event\n")
		return
	}

	// Dispatch synchronously so a 200 means "handler ran". Heavy work
	// inside the handler should still come back fast — GitHub will
	// retry on a slow response.
	if err := s.cfg.Handler(r.Context(), ev); err != nil {
		s.logger.Printf("[webhook] handler error on %s/%s: %v", ev.Type, deliveryID, err)
		// Still return 200 — the handler logs to audit, retries from
		// GitHub typically don't help fix a downstream API failure.
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}
