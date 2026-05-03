package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/audit"
	"github.com/45online/roster/internal/memory"
	"github.com/45online/roster/internal/modules/pr_review"
	"github.com/45online/roster/internal/projcfg"
)

// newReviewPRCmd builds `roster review-pr`: Module B's manual one-shot.
// Useful for sanity-checking Claude review output on a known PR before
// the takeover handler dispatches automatically.
func newReviewPRCmd() *cobra.Command {
	var (
		repo              string
		prNum             int
		canApprove        bool
		canRequestChanges bool
	)
	cmd := &cobra.Command{
		Use:   "review-pr",
		Short: "Run Module B (PR AI Review) once on a single PR",
		Long: longDesc(`
Manually trigger Module B (PR AI Review) on a single pull request. Fetches
the PR + unified diff, asks Claude for a structured review, and posts it
to GitHub.

By default Roster's review is non-blocking — even an "approve" verdict is
submitted as a regular COMMENT review (so a human still has to click the
green button). Pass --can-approve / --can-request-changes to allow Roster
to submit those event types.

Credentials read from env vars or ~/.roster/credentials.json.
ANTHROPIC_API_KEY (or 'roster login claude') is REQUIRED for Module B.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := &credsResolver{}
			ghToken, err := r.gh()
			if err != nil {
				return err
			}
			llmCfg, ok := r.llm(projcfg.LLM{})
			if !ok {
				return fmt.Errorf("LLM provider required for Module B (run 'roster login llm' or 'roster login claude', or set ROSTER_LLM_API_KEY / ANTHROPIC_API_KEY)")
			}

			ghClient := gh.NewClient(ghToken)
			apiClient, err := llmCfg.NewClient()
			if err != nil {
				return fmt.Errorf("init LLM client: %w", err)
			}

			recorder := audit.NewRecorder(audit.DefaultBaseDir())
			mem, _ := memory.Load(".") // empty memory if absent — non-fatal
			mod := pr_review.New(ghClient, apiClient, llmCfg.Model, pr_review.Config{
				CanApprove:        canApprove,
				CanRequestChanges: canRequestChanges,
			}).WithAudit(recorder).WithMemory(mem)

			ctx := context.Background()
			res, err := mod.ReviewPR(ctx, repo, prNum)
			if err != nil {
				return err
			}
			if res.Skipped {
				fmt.Fprintf(os.Stderr, "⏭  Skipped: %s\n", res.SkipReason)
				return nil
			}
			fmt.Printf("✓ Review submitted (%s, %d inline comments)\n", res.Verdict, res.CommentCount)
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (owner/name) — required")
	cmd.Flags().IntVar(&prNum, "pr", 0, "Pull request number — required")
	cmd.Flags().BoolVar(&canApprove, "can-approve", false, "Allow Roster to submit APPROVE reviews (default: off)")
	cmd.Flags().BoolVar(&canRequestChanges, "can-request-changes", false, "Allow Roster to submit REQUEST_CHANGES reviews (default: off)")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("pr")
	return cmd
}
