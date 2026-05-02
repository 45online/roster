package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/45online/roster/internal/adapters/confluence"
	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/adapters/slack"
	"github.com/45online/roster/internal/api"
	"github.com/45online/roster/internal/audit"
	"github.com/45online/roster/internal/modules/issue_to_confluence"
)

// newArchiveIssueCmd builds `roster archive-issue`: Module C's manual
// one-shot. Useful for testing the Confluence + Slack pipeline before the
// takeover handler dispatches automatically on issues.closed events.
func newArchiveIssueCmd() *cobra.Command {
	var (
		repo, spaceID, parentID, completedLabel, slackChannel string
		issueNum                                              int
	)
	cmd := &cobra.Command{
		Use:   "archive-issue",
		Short: "Archive a closed GitHub issue to Confluence (Module C, manual trigger)",
		Long: longDesc(`
Run Module C (Issue close → Confluence draft) once for a single issue.

The issue MUST be closed and carry the configured 'completed' label
(default "completed"); otherwise the call is skipped silently.

Confluence credentials reuse the Jira ones (Atlassian site/email/token);
make sure 'roster login jira' has been run.

A Slack notification is sent if --slack-channel is given (or the project
config defines one) and 'roster login slack' has been run.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := &credsResolver{}
			ghToken, err := r.gh()
			if err != nil {
				return err
			}
			confURL, confEmail, confToken, err := r.jira("", "")
			if err != nil {
				return fmt.Errorf("Confluence reuses Jira credentials: %w", err)
			}
			claudeKey := r.claude()
			if claudeKey == "" {
				return fmt.Errorf("Claude API key required (set ANTHROPIC_API_KEY or run 'roster login claude')")
			}

			ghClient := gh.NewClient(ghToken)
			confClient := confluence.NewClient(confURL, confEmail, confToken)
			apiClient, err := api.NewClient(api.ClientConfig{
				Provider: api.ProviderDirect,
				APIKey:   claudeKey,
			}, nil)
			if err != nil {
				return fmt.Errorf("init claude client: %w", err)
			}

			var slackClient *slack.Client
			if slackChannel != "" {
				if s := r.load(); s.Has("slack") {
					slackClient = slack.NewClient(s.Slack.Token)
				} else if v := os.Getenv("ROSTER_SLACK_TOKEN"); v != "" {
					slackClient = slack.NewClient(v)
				} else {
					fmt.Fprintln(os.Stderr, "⚠ --slack-channel given but no Slack token (run 'roster login slack'); skipping notification")
				}
			}

			recorder := audit.NewRecorder(audit.DefaultBaseDir())
			mod := issue_to_confluence.New(ghClient, confClient, slackClient, apiClient, "", issue_to_confluence.Config{
				SpaceID:        spaceID,
				ParentPageID:   parentID,
				CompletedLabel: completedLabel,
				SlackChannel:   slackChannel,
			}).WithAudit(recorder)

			ctx := context.Background()
			res, err := mod.ArchiveIssue(ctx, repo, issueNum)
			if err != nil && res == nil {
				return err
			}
			if res.Skipped {
				fmt.Fprintf(os.Stderr, "⏭  Skipped: %s\n", res.Reason)
				return nil
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠ partial: %v\n", err)
			}
			fmt.Printf("✓ Draft created (id=%s)\n  %s\n", res.PageID, res.PageURL)
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (owner/name) — required")
	cmd.Flags().IntVar(&issueNum, "issue", 0, "GitHub issue number — required")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "Confluence space ID — required")
	cmd.Flags().StringVar(&parentID, "parent-id", "", "Optional Confluence parent page ID")
	cmd.Flags().StringVar(&completedLabel, "completed-label", "completed", "Label that gates archival")
	cmd.Flags().StringVar(&slackChannel, "slack-channel", "", "Optional Slack channel for the draft URL")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("issue")
	_ = cmd.MarkFlagRequired("space-id")
	return cmd
}
