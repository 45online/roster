package bootstrap

import (
	"fmt"
	"os"

	"github.com/45online/roster/internal/creds"
)

// resolveCreds resolves credentials from env vars first, then from
// ~/.roster/credentials.json, then errors. The store is loaded lazily on
// first env-var miss.
type credsResolver struct {
	cached *creds.Store
	loaded bool
}

func (r *credsResolver) load() *creds.Store {
	if r.loaded {
		return r.cached
	}
	s, _ := creds.Load(creds.Path(""))
	if s == nil {
		s = &creds.Store{}
	}
	r.cached = s
	r.loaded = true
	return s
}

// gh returns the GitHub PAT, env-first.
func (r *credsResolver) gh() (string, error) {
	if v := os.Getenv("ROSTER_GITHUB_TOKEN"); v != "" {
		return v, nil
	}
	if s := r.load(); s.Has("github") {
		return s.GitHub.Token, nil
	}
	return "", fmt.Errorf("no GitHub token: set ROSTER_GITHUB_TOKEN or run 'roster login github'")
}

// jira returns URL, email, token in that order.
func (r *credsResolver) jira(urlOverride, emailOverride string) (string, string, string, error) {
	url := urlOverride
	if url == "" {
		url = os.Getenv("ROSTER_JIRA_URL")
	}
	email := emailOverride
	if email == "" {
		email = os.Getenv("ROSTER_JIRA_EMAIL")
	}
	token := os.Getenv("ROSTER_JIRA_TOKEN")

	if url == "" || email == "" || token == "" {
		s := r.load()
		if s.Has("jira") {
			if url == "" {
				url = s.Jira.URL
			}
			if email == "" {
				email = s.Jira.Email
			}
			if token == "" {
				token = s.Jira.Token
			}
		}
	}
	if url == "" {
		return "", "", "", fmt.Errorf("no Jira URL: set ROSTER_JIRA_URL, --jira-url, or run 'roster login jira'")
	}
	if email == "" {
		return "", "", "", fmt.Errorf("no Jira email: set ROSTER_JIRA_EMAIL, --jira-email, or run 'roster login jira'")
	}
	if token == "" {
		return "", "", "", fmt.Errorf("no Jira token: set ROSTER_JIRA_TOKEN or run 'roster login jira'")
	}
	return url, email, token, nil
}

// claude returns the Claude API key (or "" if none — caller decides whether
// that's an error).
func (r *credsResolver) claude() string {
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		return v
	}
	if s := r.load(); s.Has("claude") {
		return s.Claude.APIKey
	}
	return ""
}
