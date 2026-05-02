package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/adapters/jira"
	"github.com/45online/roster/internal/api"
	"github.com/45online/roster/internal/audit"
	"github.com/45online/roster/internal/modules/issue_to_jira"
	"github.com/45online/roster/internal/modules/pr_review"
	"github.com/45online/roster/internal/poller"
	"github.com/45online/roster/internal/projcfg"
)

// newTakeoverCmd builds `roster takeover`: foreground-running poller
// that watches a repo's GitHub events and dispatches them to modules.
//
// Currently only Module A (Issue → Jira) is wired in. Other modules will
// be plugged into the same handler in later phases.
func newTakeoverCmd() *cobra.Command {
	var (
		repo, jiraProject, jiraURL, jiraEmail, configPath string
		interval                                          time.Duration
	)
	cmd := &cobra.Command{
		Use:   "takeover",
		Short: "Run Roster's poller in the foreground for a repo",
		Long: longDesc(`
Start the GitHub events poller for a repository and dispatch new events
to the configured modules. The poller advances a per-repo cursor in
~/.roster/cursors/<owner>_<repo>.json and uses ETag conditional fetches
to stay within GitHub's rate limit.

Anti-loop: events authored by the virtual employee account itself are
dropped before reaching any module. The login is determined automatically
via GET /user.

Currently dispatches only Module A (Issue → Jira) on issues.opened events.
Run in the foreground; press Ctrl+C to stop.

Credentials (env vars):
  ROSTER_GITHUB_TOKEN, ROSTER_JIRA_TOKEN, ROSTER_JIRA_URL, ROSTER_JIRA_EMAIL
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := &credsResolver{}
			ghToken, err := r.gh()
			if err != nil {
				return err
			}
			resolvedURL, resolvedEmail, jiraToken, err := r.jira(jiraURL, jiraEmail)
			if err != nil {
				return err
			}
			jiraURL, jiraEmail = resolvedURL, resolvedEmail

			// Project config (optional). Flags override config values.
			cfgFile := configPath
			if cfgFile == "" {
				cfgFile = ".roster/config.yml"
			}
			cfg, found, err := projcfg.LoadOrDefault(cfgFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if found {
				fmt.Printf("✓ Loaded config from %s\n", cfgFile)
			}
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}
			if jiraProject == "" {
				jiraProject = cfg.Modules.IssueToJira.JiraProject
			}
			if jiraProject == "" {
				return fmt.Errorf("jira project is required (pass --jira-project or set modules.issue_to_jira.jira_project)")
			}

			ghClient := gh.NewClient(ghToken)
			jiraClient := jira.NewClient(jiraURL, jiraEmail, jiraToken)
			recorder := audit.NewRecorder(audit.DefaultBaseDir())
			modA := issue_to_jira.New(ghClient, jiraClient, issue_to_jira.Config{
				JiraProject:      jiraProject,
				DefaultIssueType: orDefault(cfg.Modules.IssueToJira.DefaultIssueType, "Task"),
				LabelToIssueType: orMap(cfg.Modules.IssueToJira.LabelToIssueType, map[string]string{"bug": "Bug"}),
				PriorityMapping: orMap(cfg.Modules.IssueToJira.PriorityMapping, map[string]string{
					"P0": "Highest",
					"P1": "High",
					"P2": "Medium",
				}),
			}).WithAudit(recorder)

			// Optional Claude extractor for Module A and Module B.
			var apiClient api.Client
			if claudeKey := r.claude(); claudeKey != "" {
				if c, apiErr := api.NewClient(api.ClientConfig{
					Provider: api.ProviderDirect,
					APIKey:   claudeKey,
				}, nil); apiErr == nil {
					apiClient = c
					modA = modA.WithExtractor(issue_to_jira.NewExtractor(apiClient, ""))
					fmt.Println("✓ Claude extractor enabled")
				}
			}

			// Module B (PR AI Review): enabled by config + Claude availability.
			var modB *pr_review.Module
			if cfg.Modules.PRReview.Enabled {
				if apiClient == nil {
					fmt.Fprintln(os.Stderr, "⚠ pr_review.enabled=true but no Claude API key — Module B disabled")
				} else {
					modB = pr_review.New(ghClient, apiClient, "", pr_review.Config{
						SkipPaths:         cfg.Modules.PRReview.SkipPaths,
						MaxDiffBytes:      cfg.Modules.PRReview.MaxDiffBytes,
						CanApprove:        cfg.Modules.PRReview.CanApprove,
						CanRequestChanges: cfg.Modules.PRReview.CanRequestChanges,
					}).WithAudit(recorder)
					fmt.Println("✓ Module B (PR review) armed")
				}
			}

			ctx, cancel := signalContext()
			defer cancel()

			selfLogin, err := ghClient.AuthUser(ctx)
			if err != nil {
				return fmt.Errorf("identify virtual employee account: %w", err)
			}
			fmt.Printf("✓ Authenticated as @%s (anti-loop filter armed)\n", selfLogin)

			handler := func(ctx context.Context, ev gh.Event) error {
				switch ev.Type {
				case "IssuesEvent":
					p, err := ev.DecodeIssuesPayload()
					if err != nil {
						return err
					}
					if p.Action != "opened" {
						return nil
					}
					log.Printf("[mod-a] dispatching: %s#%d %q (by @%s)",
						repo, p.Issue.Number, p.Issue.Title, ev.Actor.Login)

					res, err := modA.SyncIssue(ctx, repo, p.Issue.Number)
					if err != nil && res == nil {
						return err
					}
					if err != nil {
						log.Printf("[mod-a] partial: %s created, comment failed: %v", res.JiraKey, err)
						return nil
					}
					marker := ""
					if res.AIExtracted {
						marker = " (AI-extracted)"
					}
					log.Printf("[mod-a] ✓ %s%s → %s", res.JiraKey, marker, res.JiraURL)
					return nil

				case "PullRequestEvent":
					if modB == nil {
						return nil
					}
					p, err := ev.DecodePullRequestPayload()
					if err != nil {
						return err
					}
					if p.Action != "opened" && p.Action != "synchronize" {
						return nil
					}
					if p.PullRequest.Draft {
						return nil
					}
					log.Printf("[mod-b] dispatching: %s#%d %q (by @%s, action=%s)",
						repo, p.Number, p.PullRequest.Title, ev.Actor.Login, p.Action)

					res, err := modB.ReviewPR(ctx, repo, p.Number)
					if err != nil {
						return err
					}
					if res.Skipped {
						log.Printf("[mod-b] ⏭  skipped: %s", res.SkipReason)
						return nil
					}
					log.Printf("[mod-b] ✓ %s (%d inline comments)", res.Verdict, res.CommentCount)
					return nil
				}
				return nil
			}

			p, err := poller.New(poller.Config{
				GH:        ghClient,
				Repo:      repo,
				Interval:  interval,
				SelfLogin: selfLogin,
				Handler:   handler,
			})
			if err != nil {
				return err
			}

			err = p.Run(ctx)
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (owner/name) — required")
	cmd.Flags().StringVar(&jiraProject, "jira-project", "", "Jira project key (or read from .roster/config.yml)")
	cmd.Flags().StringVar(&jiraURL, "jira-url", "", "Jira site URL (or set ROSTER_JIRA_URL)")
	cmd.Flags().StringVar(&jiraEmail, "jira-email", "", "Jira account email (or set ROSTER_JIRA_EMAIL)")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to project config (default: .roster/config.yml)")
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Second, "Poll cadence")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

// orDefault returns s when non-empty, else def.
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// orMap returns m when non-empty, else def.
func orMap(m, def map[string]string) map[string]string {
	if len(m) > 0 {
		return m
	}
	return def
}

// signalContext returns a Context that's cancelled on SIGINT or SIGTERM.
func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()
	return ctx, cancel
}
