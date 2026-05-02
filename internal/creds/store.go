// Package creds is Roster's credential store. Credentials live as JSON
// at $HOME/.roster/credentials.json with mode 0600 (owner-readable only),
// in the spirit of ~/.aws/credentials and ~/.netrc.
//
// This is intentionally simple — no encryption at rest, no OS keychain —
// to keep the implementation transparent and the failure modes obvious.
// Operators who need stronger storage can mount that directory from a
// secrets manager or wrap the binary in a launcher that injects env vars.
package creds

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Store holds all per-provider credentials.
type Store struct {
	GitHub *GitHubCreds `json:"github,omitempty"`
	Jira   *JiraCreds   `json:"jira,omitempty"`
	Slack  *SlackCreds  `json:"slack,omitempty"`
	Claude *ClaudeCreds `json:"claude,omitempty"`
}

// GitHubCreds is the GitHub PAT for the virtual employee account.
type GitHubCreds struct {
	Token string `json:"token"`
}

// JiraCreds covers the three fields needed for Jira basic-auth.
type JiraCreds struct {
	URL   string `json:"url"`
	Email string `json:"email"`
	Token string `json:"token"`
}

// SlackCreds is the Slack OAuth user token.
type SlackCreds struct {
	Token string `json:"token"`
}

// ClaudeCreds is the Anthropic API key (or compatible provider key).
type ClaudeCreds struct {
	APIKey string `json:"api_key"`
}

// Path returns the canonical credentials file path under baseDir.
// If baseDir is empty, falls back to $HOME/.roster.
func Path(baseDir string) string {
	if baseDir == "" {
		baseDir = defaultBaseDir()
	}
	return filepath.Join(baseDir, "credentials.json")
}

// Load reads the store from path. Returns an empty Store (no error) if the
// file doesn't exist; this lets callers treat "no credentials yet" as a
// soft-empty state.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Store{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("creds: read %s: %w", path, err)
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("creds: parse %s: %w", path, err)
	}
	return &s, nil
}

// Save writes the store to path with 0600 perms, using atomic temp+rename.
func (s *Store) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creds: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("creds: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("creds: rename: %w", err)
	}
	// Re-apply 0600 in case umask intervened.
	_ = os.Chmod(path, 0o600)
	return nil
}

// Has reports whether a provider has a usable record (token present).
func (s *Store) Has(provider string) bool {
	switch provider {
	case "github":
		return s.GitHub != nil && s.GitHub.Token != ""
	case "jira":
		return s.Jira != nil && s.Jira.Token != "" && s.Jira.URL != "" && s.Jira.Email != ""
	case "slack":
		return s.Slack != nil && s.Slack.Token != ""
	case "claude":
		return s.Claude != nil && s.Claude.APIKey != ""
	}
	return false
}

// Clear removes a provider's record. No-op if the provider isn't present.
func (s *Store) Clear(provider string) {
	switch provider {
	case "github":
		s.GitHub = nil
	case "jira":
		s.Jira = nil
	case "slack":
		s.Slack = nil
	case "claude":
		s.Claude = nil
	}
}

// defaultBaseDir mirrors audit.DefaultBaseDir without the import cycle.
func defaultBaseDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".roster")
	}
	return ".roster"
}
