// Package projcfg loads and validates per-project configuration that
// lives at <repo>/.roster/config.yml. This is distinct from internal/config,
// which handles the inherited settings.json layering.
package projcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level shape of .roster/config.yml.
type Config struct {
	ProjectName string  `yaml:"project_name"`
	Identity    string  `yaml:"identity"`
	LLM         LLM     `yaml:"llm"`
	Modules     Modules `yaml:"modules"`
	Budget      Budget  `yaml:"budget"`
	Webhook     Webhook `yaml:"webhook"`
	DryRun      bool    `yaml:"dry_run"`
}

// LLM selects the AI provider Roster uses for Modules A / B / C.
// Provider:
//   - "" or "anthropic"        — Claude via api.anthropic.com (default)
//   - "openai-compatible"      — any endpoint speaking OpenAI Chat
//                                 Completions (DeepSeek, xAI, Gemini
//                                 OpenAI-compat, Together, Groq, ...)
//
// API key never lives in the YAML — use env (ROSTER_LLM_API_KEY /
// ANTHROPIC_API_KEY) or the credential store ('roster login llm' /
// 'roster login claude').
type LLM struct {
	Provider string `yaml:"provider"`
	BaseURL  string `yaml:"base_url"` // required when provider=openai-compatible
	Model    string `yaml:"model"`    // required when provider=openai-compatible
}

// Webhook configures the embedded GitHub webhook receiver. When enabled,
// takeover replaces polling with HTTP push.
type Webhook struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"` // e.g. ":8080" or "127.0.0.1:9090". Default ":8080".
	Path    string `yaml:"path"`   // default "/webhook/github"
	// Secret is the HMAC shared secret you set on the GitHub repo's
	// webhook. Empty here is allowed only if env ROSTER_WEBHOOK_SECRET
	// is set; takeover refuses to start without one.
	Secret string `yaml:"secret"`
}

// Modules toggles and configures each Roster module.
type Modules struct {
	IssueToJira       IssueToJira       `yaml:"issue_to_jira"`
	PRReview          PRReview          `yaml:"pr_review"`
	IssueToConfluence IssueToConfluence `yaml:"issue_to_confluence"`
	AlertAggregation  AlertAggregation  `yaml:"alert_aggregation"`
}

// ModuleToggle is a simple enabled-flag for modules not yet implemented.
type ModuleToggle struct {
	Enabled bool `yaml:"enabled"`
}

// IssueToJira configures Module A.
type IssueToJira struct {
	Enabled          bool              `yaml:"enabled"`
	JiraProject      string            `yaml:"jira_project"`
	DefaultIssueType string            `yaml:"default_issue_type"`
	PriorityMapping  map[string]string `yaml:"priority_mapping"`
	LabelToIssueType map[string]string `yaml:"label_to_issue_type"`
}

// IssueToConfluence configures Module C.
type IssueToConfluence struct {
	Enabled bool `yaml:"enabled"`
	// SpaceID is the numeric Confluence space ID where drafts are filed.
	SpaceID string `yaml:"space_id"`
	// ParentPageID nests drafts under a parent page (optional).
	ParentPageID string `yaml:"parent_page_id"`
	// CompletedLabel gates archival to issues that carry it. Default
	// "completed".
	CompletedLabel string `yaml:"completed_label"`
	// SlackChannel receives a notification with the draft URL. Empty →
	// no Slack notification.
	SlackChannel string `yaml:"slack_channel"`
}

// PRReview configures Module B.
type PRReview struct {
	Enabled bool `yaml:"enabled"`
	// SkipPaths short-circuits the review if every changed file matches.
	// Forms: "docs/", "docs/**", "*.md".
	SkipPaths []string `yaml:"skip_paths"`
	// MaxDiffBytes truncates large diffs before sending to Claude.
	// 0 → use module default (64 KB).
	MaxDiffBytes int `yaml:"max_diff_bytes"`
	// CanApprove gates the APPROVE verdict. false (default) → submitted
	// as plain COMMENT, real approval still requires a human.
	CanApprove bool `yaml:"can_approve"`
	// CanRequestChanges gates REQUEST_CHANGES (a blocking review).
	CanRequestChanges bool `yaml:"can_request_changes"`
}

// AlertAggregation configures Module D.
type AlertAggregation struct {
	Enabled      bool   `yaml:"enabled"`
	SlackChannel string `yaml:"slack_channel"`
	Lookback     string `yaml:"lookback"`
}

// Budget caps Roster's spend on a project.
type Budget struct {
	MonthlyUSD float64 `yaml:"monthly_usd"`
	OnExceed   string  `yaml:"on_exceed"` // "downgrade" | "stop"
}

// Default returns a Config populated with sensible defaults. Useful as the
// base before merging user values.
func Default() *Config {
	return &Config{
		Modules: Modules{
			IssueToJira: IssueToJira{
				Enabled:          true,
				DefaultIssueType: "Task",
				PriorityMapping: map[string]string{
					"P0": "Highest",
					"P1": "High",
					"P2": "Medium",
				},
				LabelToIssueType: map[string]string{
					"bug": "Bug",
				},
			},
		},
		Budget: Budget{
			MonthlyUSD: 50,
			OnExceed:   "downgrade",
		},
	}
}

// ConfigPath returns the canonical config path for a repo root.
func ConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".roster", "config.yml")
}

// Load reads and validates the config at path. If the file doesn't exist,
// returns (nil, os.ErrNotExist) so callers can fall back to defaults.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("projcfg: parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("projcfg: %s: %w", path, err)
	}
	return cfg, nil
}

// Validate checks that required fields for enabled modules are present and
// budget settings make sense.
func (c *Config) Validate() error {
	if c.Modules.IssueToJira.Enabled {
		if strings.TrimSpace(c.Modules.IssueToJira.JiraProject) == "" {
			return fmt.Errorf("modules.issue_to_jira.enabled=true but jira_project is empty")
		}
	}
	if c.Budget.OnExceed != "" &&
		c.Budget.OnExceed != "downgrade" &&
		c.Budget.OnExceed != "stop" {
		return fmt.Errorf("budget.on_exceed must be 'downgrade' or 'stop' (got %q)", c.Budget.OnExceed)
	}
	switch c.LLM.Provider {
	case "", "anthropic":
		// fine
	case "openai-compatible":
		if c.LLM.BaseURL == "" {
			return fmt.Errorf("llm.provider=openai-compatible but llm.base_url is empty")
		}
		if c.LLM.Model == "" {
			return fmt.Errorf("llm.provider=openai-compatible but llm.model is empty")
		}
	default:
		return fmt.Errorf("llm.provider must be 'anthropic' or 'openai-compatible' (got %q)", c.LLM.Provider)
	}
	return nil
}

// LoadOrDefault loads the config from path; returns defaults if the file
// is missing. Any other error (parse / validate) is propagated.
func LoadOrDefault(path string) (*Config, bool, error) {
	cfg, err := Load(path)
	if os.IsNotExist(err) {
		return Default(), false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return cfg, true, nil
}
