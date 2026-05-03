package bootstrap

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/45online/roster/internal/creds"
)

// newLoginCmd builds the `roster login` command tree:
//
//	roster login github          # prompt for PAT
//	roster login jira            # prompt for url, email, token
//	roster login slack           # prompt for OAuth token
//	roster login claude          # prompt for API key
//	roster login status          # which providers are configured
//	roster login logout <prov>   # remove one provider
//
// Stored at ~/.roster/credentials.json (mode 0600).
func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save credentials for GitHub / Jira / Slack / Claude",
		Long: longDesc(`
Store provider credentials at ~/.roster/credentials.json (mode 0600) so
'roster takeover' and 'roster sync-issue' don't have to consume the
ROSTER_*_TOKEN environment variables every time.

Resolution order at runtime:
  1. ROSTER_*_TOKEN env vars (still honored, override the file)
  2. ~/.roster/credentials.json
  3. (none — error)

Use 'roster login status' to see what's configured, and
'roster login logout <provider>' to remove one.
`),
	}
	cmd.AddCommand(
		newLoginGitHubCmd(),
		newLoginJiraCmd(),
		newLoginSlackCmd(),
		newLoginClaudeCmd(),
		newLoginLLMCmd(),
		newLoginStatusCmd(),
		newLoginLogoutCmd(),
	)
	return cmd
}

func newLoginGitHubCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "github",
		Short: "Save the GitHub PAT for the virtual employee account",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := promptSecret("GitHub PAT")
			if err != nil {
				return err
			}
			return updateStore(func(s *creds.Store) {
				s.GitHub = &creds.GitHubCreds{Token: token}
			}, "github")
		},
	}
}

func newLoginJiraCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "jira",
		Short: "Save Jira URL, account email, and API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := promptLine("Jira site URL (e.g. https://acme.atlassian.net)")
			if err != nil {
				return err
			}
			email, err := promptLine("Jira account email")
			if err != nil {
				return err
			}
			token, err := promptSecret("Jira API token")
			if err != nil {
				return err
			}
			return updateStore(func(s *creds.Store) {
				s.Jira = &creds.JiraCreds{URL: url, Email: email, Token: token}
			}, "jira")
		},
	}
}

func newLoginSlackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "slack",
		Short: "Save the Slack OAuth user token",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := promptSecret("Slack OAuth token (xoxp-...)")
			if err != nil {
				return err
			}
			return updateStore(func(s *creds.Store) {
				s.Slack = &creds.SlackCreds{Token: token}
			}, "slack")
		},
	}
}

func newLoginClaudeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "claude",
		Short: "Save the Claude API key (legacy single-provider; prefer 'llm')",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := promptSecret("Claude API key (sk-ant-...)")
			if err != nil {
				return err
			}
			return updateStore(func(s *creds.Store) {
				s.Claude = &creds.ClaudeCreds{APIKey: key}
			}, "claude")
		},
	}
}

// newLoginLLMCmd is the multi-provider entry: prompts for provider /
// base URL / model / API key. Replaces 'login claude' for new setups.
func newLoginLLMCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "llm",
		Short: "Save LLM provider config (Anthropic / OpenAI-compatible)",
		Long: longDesc(`
Stores the multi-provider AI configuration: provider, base URL (for
OpenAI-compatible endpoints), default model, and the API key.

Examples of openai-compatible base URLs:
  https://api.deepseek.com                                       (DeepSeek)
  https://api.x.ai/v1                                            (xAI)
  https://generativelanguage.googleapis.com/v1beta/openai/        (Gemini)
  https://api.openai.com/v1                                      (OpenAI)
  https://api.together.xyz/v1                                    (Together)
  https://api.groq.com/openai/v1                                 (Groq)
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := promptLine("Provider [anthropic | openai-compatible]")
			if err != nil {
				return err
			}
			provider = strings.ToLower(strings.TrimSpace(provider))
			if provider == "" {
				provider = "anthropic"
			}
			if provider != "anthropic" && provider != "openai-compatible" {
				return fmt.Errorf("provider must be 'anthropic' or 'openai-compatible'")
			}

			var baseURL, model string
			if provider == "openai-compatible" {
				baseURL, err = promptLine("Base URL (e.g. https://api.deepseek.com)")
				if err != nil {
					return err
				}
				if baseURL == "" {
					return fmt.Errorf("base URL is required for openai-compatible")
				}
				model, err = promptLine("Default model (e.g. deepseek-chat)")
				if err != nil {
					return err
				}
				if model == "" {
					return fmt.Errorf("model is required for openai-compatible")
				}
			} else {
				// anthropic — model is optional (modules pick a sane default)
				model, err = promptLine("Default model (blank → module defaults Claude Sonnet/Haiku)")
				if err != nil {
					return err
				}
			}

			key, err := promptSecret("API key")
			if err != nil {
				return err
			}
			if key == "" {
				return fmt.Errorf("API key cannot be empty")
			}
			return updateStore(func(s *creds.Store) {
				s.LLM = &creds.LLMCreds{
					Provider: provider,
					BaseURL:  baseURL,
					Model:    model,
					APIKey:   key,
				}
			}, "llm")
		},
	}
}

func newLoginStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which providers have stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := creds.Path("")
			s, err := creds.Load(path)
			if err != nil {
				return err
			}
			fmt.Printf("Credentials file: %s\n", path)
			for _, p := range []string{"github", "jira", "slack", "claude", "llm"} {
				mark := "✗ not set"
				if s.Has(p) {
					mark = "✓ configured"
					if p == "llm" && s.LLM != nil {
						extras := s.LLM.Provider
						if s.LLM.Model != "" {
							extras += " / " + s.LLM.Model
						}
						mark += "  (" + extras + ")"
					}
				}
				fmt.Printf("  %-7s %s\n", p, mark)
			}
			return nil
		},
	}
}

func newLoginLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <provider>",
		Short: "Remove stored credentials for one provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := strings.ToLower(args[0])
			switch p {
			case "github", "jira", "slack", "claude", "llm":
				// fine
			default:
				return fmt.Errorf("unknown provider %q (want: github, jira, slack, claude, llm)", p)
			}
			path := creds.Path("")
			s, err := creds.Load(path)
			if err != nil {
				return err
			}
			s.Clear(p)
			if err := s.Save(path); err != nil {
				return err
			}
			fmt.Printf("✓ Cleared %s\n", p)
			return nil
		},
	}
}

// updateStore loads, mutates, and re-saves the credentials file.
func updateStore(mut func(*creds.Store), provider string) error {
	path := creds.Path("")
	s, err := creds.Load(path)
	if err != nil {
		return err
	}
	mut(s)
	if err := s.Save(path); err != nil {
		return err
	}
	fmt.Printf("✓ Saved %s credentials to %s\n", provider, path)
	return nil
}

// stdinReader is a single bufio.Reader shared across promptLine calls.
// Re-creating one each call would discard the bufio internal buffer
// (any byte read into bufio's window but not yet returned to the
// caller is lost when the wrapper goes away), which manifests as EOF on
// the second prompt of a multi-prompt subcommand like 'roster login llm'.
var stdinReader = bufio.NewReader(os.Stdin)

// promptLine reads a single line from stdin (echoed). Returns the line
// with trailing whitespace stripped.
func promptLine(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	line, err := stdinReader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n "), nil
}

// promptSecret reads a line without echoing (good for tokens).
func promptSecret(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// Fall back to plain read when stdin is piped (e.g. in scripts).
		s, err := promptLine("")
		fmt.Fprintln(os.Stderr)
		return s, err
	}
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
