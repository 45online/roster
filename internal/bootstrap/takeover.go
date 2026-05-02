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
	"github.com/45online/roster/internal/modules/issue_to_jira"
	"github.com/45online/roster/internal/poller"
)

// newTakeoverCmd builds `roster takeover`: foreground-running poller
// that watches a repo's GitHub events and dispatches them to modules.
//
// Currently only Module A (Issue → Jira) is wired in. Other modules will
// be plugged into the same handler in later phases.
func newTakeoverCmd() *cobra.Command {
	var (
		repo, jiraProject, jiraURL, jiraEmail string
		interval                              time.Duration
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
			ghToken := os.Getenv("ROSTER_GITHUB_TOKEN")
			if ghToken == "" {
				return fmt.Errorf("ROSTER_GITHUB_TOKEN is not set")
			}
			jiraToken := os.Getenv("ROSTER_JIRA_TOKEN")
			if jiraToken == "" {
				return fmt.Errorf("ROSTER_JIRA_TOKEN is not set")
			}
			if jiraURL == "" {
				jiraURL = os.Getenv("ROSTER_JIRA_URL")
			}
			if jiraURL == "" {
				return fmt.Errorf("--jira-url or ROSTER_JIRA_URL is required")
			}
			if jiraEmail == "" {
				jiraEmail = os.Getenv("ROSTER_JIRA_EMAIL")
			}
			if jiraEmail == "" {
				return fmt.Errorf("--jira-email or ROSTER_JIRA_EMAIL is required")
			}

			ghClient := gh.NewClient(ghToken)
			jiraClient := jira.NewClient(jiraURL, jiraEmail, jiraToken)
			modA := issue_to_jira.New(ghClient, jiraClient, issue_to_jira.Config{
				JiraProject:      jiraProject,
				DefaultIssueType: "Task",
				LabelToIssueType: map[string]string{"bug": "Bug"},
				PriorityMapping: map[string]string{
					"P0": "Highest",
					"P1": "High",
					"P2": "Medium",
				},
			})

			ctx, cancel := signalContext()
			defer cancel()

			selfLogin, err := ghClient.AuthUser(ctx)
			if err != nil {
				return fmt.Errorf("identify virtual employee account: %w", err)
			}
			fmt.Printf("✓ Authenticated as @%s (anti-loop filter armed)\n", selfLogin)

			handler := func(ctx context.Context, ev gh.Event) error {
				if ev.Type != "IssuesEvent" {
					return nil
				}
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
				log.Printf("[mod-a] ✓ %s → %s", res.JiraURL, res.JiraURL)
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
	cmd.Flags().StringVar(&jiraProject, "jira-project", "", "Jira project key — required")
	cmd.Flags().StringVar(&jiraURL, "jira-url", "", "Jira site URL (or set ROSTER_JIRA_URL)")
	cmd.Flags().StringVar(&jiraEmail, "jira-email", "", "Jira account email (or set ROSTER_JIRA_EMAIL)")
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Second, "Poll cadence")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("jira-project")
	return cmd
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
