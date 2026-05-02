package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/adapters/jira"
	"github.com/45online/roster/internal/api"
	"github.com/45online/roster/internal/audit"
	"github.com/45online/roster/internal/modules/issue_to_jira"
)

// newSyncIssueCmd builds the `roster sync-issue` command — Module A's
// manual one-shot trigger. Useful for testing the full pipeline before the
// background poller is wired up.
func newSyncIssueCmd() *cobra.Command {
	var (
		repo, jiraProject, jiraURL, jiraEmail, defaultType string
		issueNum                                           int
	)
	cmd := &cobra.Command{
		Use:   "sync-issue",
		Short: "Sync a single GitHub issue to Jira (Module A, manual trigger)",
		Long: longDesc(`
Run Module A (Issue → Jira) once for a single GitHub issue.

This is a manual one-shot for verifying the GitHub + Jira credential pipeline
end-to-end before the background poller is wired up. The fully automated
flow is reached by 'roster takeover' once that command lands.

Credentials are read from environment variables:
  ROSTER_GITHUB_TOKEN   GitHub PAT (write scope on the repo)
  ROSTER_JIRA_TOKEN     Jira API token
  ROSTER_JIRA_URL       Jira site URL (e.g. https://acme.atlassian.net) — or pass --jira-url
  ROSTER_JIRA_EMAIL     Jira account email — or pass --jira-email
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
			recorder := audit.NewRecorder(audit.DefaultBaseDir())
			mod := issue_to_jira.New(ghClient, jiraClient, issue_to_jira.Config{
				JiraProject:      jiraProject,
				DefaultIssueType: defaultType,
				LabelToIssueType: map[string]string{"bug": "Bug"},
				PriorityMapping: map[string]string{
					"P0": "Highest",
					"P1": "High",
					"P2": "Medium",
				},
			}).WithAudit(recorder)

			// Optional Claude extractor: enabled iff ANTHROPIC_API_KEY is set.
			if claudeKey := os.Getenv("ANTHROPIC_API_KEY"); claudeKey != "" {
				apiClient, apiErr := api.NewClient(api.ClientConfig{
					Provider: api.ProviderDirect,
					APIKey:   claudeKey,
				}, nil)
				if apiErr == nil {
					mod = mod.WithExtractor(issue_to_jira.NewExtractor(apiClient, ""))
					fmt.Println("✓ Claude extractor enabled")
				} else {
					fmt.Fprintf(os.Stderr, "⚠ Claude client init failed (%v); falling back to mechanical mapping\n", apiErr)
				}
			}

			ctx := context.Background()
			res, err := mod.SyncIssue(ctx, repo, issueNum)
			if err != nil && res == nil {
				return err
			}
			if err != nil {
				// partial success: Jira ticket created, but back-link comment failed.
				fmt.Fprintf(os.Stderr, "⚠ partial: %v\n", err)
			}
			marker := ""
			if res.AIExtracted {
				marker = " (AI-extracted)"
			}
			fmt.Printf("✓ Created %s%s\n  %s\n", res.JiraKey, marker, res.JiraURL)
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (owner/name) — required")
	cmd.Flags().IntVar(&issueNum, "issue", 0, "GitHub issue number — required")
	cmd.Flags().StringVar(&jiraProject, "jira-project", "", "Jira project key — required")
	cmd.Flags().StringVar(&jiraURL, "jira-url", "", "Jira site URL (or set ROSTER_JIRA_URL)")
	cmd.Flags().StringVar(&jiraEmail, "jira-email", "", "Jira account email (or set ROSTER_JIRA_EMAIL)")
	cmd.Flags().StringVar(&defaultType, "default-type", "Task", "Default Jira issue type")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("issue")
	_ = cmd.MarkFlagRequired("jira-project")
	return cmd
}
