package bootstrap

import (
	"fmt"
	"os"

	"github.com/45online/roster/internal/api"
	"github.com/45online/roster/internal/creds"
	"github.com/45online/roster/internal/projcfg"
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
// that's an error). Legacy single-provider helper; prefer llm() for new code.
func (r *credsResolver) claude() string {
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		return v
	}
	if s := r.load(); s.Has("claude") {
		return s.Claude.APIKey
	}
	return ""
}

// LLMConfig is the resolved provider configuration for AI-backed modules.
type LLMConfig struct {
	Provider string // "anthropic" | "openai-compatible"
	BaseURL  string // optional for anthropic; required for openai-compatible
	Model    string // default model id
	APIKey   string
}

// llm resolves the LLM credentials + provider info from the layered
// sources. Resolution order, first non-empty wins:
//
//  1. ROSTER_LLM_* env vars                     (provider/base_url/model/api_key)
//  2. ~/.roster/credentials.json `llm`          (multi-provider record)
//  3. project config (passed in; provider/base_url/model only — no key)
//  4. ROSTER_LLM_API_KEY / ANTHROPIC_API_KEY    (env-only key)
//  5. ~/.roster/credentials.json `claude`       (legacy single-provider key)
//
// Returns ok=false when no API key surfaces. Defaults provider to
// "anthropic" when nothing else specifies.
func (r *credsResolver) llm(cfg projcfg.LLM) (LLMConfig, bool) {
	out := LLMConfig{
		Provider: os.Getenv("ROSTER_LLM_PROVIDER"),
		BaseURL:  os.Getenv("ROSTER_LLM_BASE_URL"),
		Model:    os.Getenv("ROSTER_LLM_MODEL"),
		APIKey:   os.Getenv("ROSTER_LLM_API_KEY"),
	}

	// Layer 2: store-level llm record fills in any blanks.
	if out.Provider == "" || out.BaseURL == "" || out.Model == "" || out.APIKey == "" {
		s := r.load()
		if s.Has("llm") {
			if out.Provider == "" {
				out.Provider = s.LLM.Provider
			}
			if out.BaseURL == "" {
				out.BaseURL = s.LLM.BaseURL
			}
			if out.Model == "" {
				out.Model = s.LLM.Model
			}
			if out.APIKey == "" {
				out.APIKey = s.LLM.APIKey
			}
		}
	}

	// Layer 3: project config overrides. Cfg has no key.
	if out.Provider == "" {
		out.Provider = cfg.Provider
	}
	if out.BaseURL == "" {
		out.BaseURL = cfg.BaseURL
	}
	if out.Model == "" {
		out.Model = cfg.Model
	}

	// Layer 4 + 5: legacy ANTHROPIC_API_KEY / claude record. These are
	// pure-key sources and imply provider=anthropic.
	if out.APIKey == "" {
		if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
			out.APIKey = v
			if out.Provider == "" {
				out.Provider = "anthropic"
			}
		} else if s := r.load(); s.Has("claude") {
			out.APIKey = s.Claude.APIKey
			if out.Provider == "" {
				out.Provider = "anthropic"
			}
		}
	}

	if out.APIKey == "" {
		return LLMConfig{}, false
	}
	if out.Provider == "" {
		out.Provider = "anthropic"
	}
	return out, true
}

// NewClient constructs the appropriate api.Client for the resolved
// provider. openai-compatible providers require BaseURL.
func (l LLMConfig) NewClient() (api.Client, error) {
	switch l.Provider {
	case "", "anthropic":
		cfg := api.ClientConfig{
			Provider: api.ProviderDirect,
			APIKey:   l.APIKey,
		}
		if l.BaseURL != "" {
			cfg.BaseURL = l.BaseURL
		}
		return api.NewClient(cfg, nil)
	case "openai-compatible", "openai":
		if l.BaseURL == "" {
			return nil, fmt.Errorf("LLM provider %q requires base_url", l.Provider)
		}
		return api.NewClient(api.ClientConfig{
			Provider: api.ProviderOpenAI,
			APIKey:   l.APIKey,
			BaseURL:  l.BaseURL,
		}, nil)
	default:
		return nil, fmt.Errorf("unknown LLM provider %q (want 'anthropic' or 'openai-compatible')", l.Provider)
	}
}
