package bootstrap

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/adapters/slack"
	"github.com/45online/roster/internal/audit"
	"github.com/45online/roster/internal/modules/alert_aggregation"
)

// newAggregateAlertCmd builds `roster aggregate-alert`: Module D's manual
// invocation. Wraps an alert text + recent repo activity into a single
// Slack message.
func newAggregateAlertCmd() *cobra.Command {
	var (
		repo, title, body, source, severity, channel string
		bodyFile                                      string
		lookback                                      time.Duration
		links                                         []string
	)
	cmd := &cobra.Command{
		Use:   "aggregate-alert",
		Short: "Post an alert + recent commits/PRs to Slack (Module D)",
		Long: longDesc(`
Module D — alert aggregation. Pair an external alert with the last hour
of commits and merged PRs in the repo, post the bundle to Slack.

This is "log platform"-shaped: no @-mentions, no Jira tickets, no causal
attribution. Oncall reads the message and decides for themselves.

Credentials: needs GitHub PAT (read-only is fine) and a Slack token
(via 'roster login slack' or ROSTER_SLACK_TOKEN). Claude is NOT used —
the message is templated, not generated.

Body input:
  --body "..."           inline string
  --body-file path       file containing the alert body
  --body-file -          read from stdin

Links:
  --link "Logs=https://logs.example/q?id=42"
  --link "Runbook=https://wiki/runbook"
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := &credsResolver{}
			ghToken, err := r.gh()
			if err != nil {
				return err
			}

			slackToken := os.Getenv("ROSTER_SLACK_TOKEN")
			if slackToken == "" {
				if s := r.load(); s.Has("slack") {
					slackToken = s.Slack.Token
				}
			}
			if slackToken == "" {
				return fmt.Errorf("no Slack token: run 'roster login slack' or set ROSTER_SLACK_TOKEN")
			}

			if bodyFile != "" {
				b, err := readBody(bodyFile)
				if err != nil {
					return fmt.Errorf("read body file: %w", err)
				}
				body = b
			}

			parsedLinks, err := parseNamedLinks(links)
			if err != nil {
				return err
			}

			ghClient := gh.NewClient(ghToken)
			slackClient := slack.NewClient(slackToken)
			recorder := audit.NewRecorder(audit.DefaultBaseDir())

			mod := alert_aggregation.New(ghClient, slackClient, alert_aggregation.Config{
				SlackChannel: channel,
				Lookback:     lookback,
			}).WithAudit(recorder)

			ctx := context.Background()
			res, err := mod.Aggregate(ctx, repo, alert_aggregation.Alert{
				Source:   source,
				Severity: severity,
				Title:    title,
				Body:     body,
				FiredAt:  time.Now().UTC(),
				Links:    parsedLinks,
			})
			if err != nil {
				return err
			}
			fmt.Printf("✓ Posted to Slack (ts=%s, %d commits, %d PRs)\n",
				res.SlackTs, res.NumCommits, res.NumPulls)
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (owner/name) — required")
	cmd.Flags().StringVar(&title, "title", "", "Alert title — required")
	cmd.Flags().StringVar(&body, "body", "", "Alert body (or use --body-file)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", `Read body from a file ("-" = stdin)`)
	cmd.Flags().StringVar(&source, "source", "", "Alert source label (e.g. CloudWatch)")
	cmd.Flags().StringVar(&severity, "severity", "", `Severity: critical | warning | info`)
	cmd.Flags().StringVar(&channel, "slack-channel", "", "Slack channel ID or #name — required")
	cmd.Flags().DurationVar(&lookback, "lookback", time.Hour, "How far back to gather commits/PRs")
	cmd.Flags().StringSliceVar(&links, "link", nil, `Footer link, format "Label=URL" (repeatable)`)
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("slack-channel")
	return cmd
}

// readBody reads from a file path, or stdin if path is "-".
func readBody(path string) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

// parseNamedLinks turns []string{"Logs=https://...", "Runbook=https://..."}
// into []NamedLink. Returns an error on malformed entries.
func parseNamedLinks(in []string) ([]alert_aggregation.NamedLink, error) {
	out := make([]alert_aggregation.NamedLink, 0, len(in))
	for _, raw := range in {
		idx := strings.Index(raw, "=")
		if idx <= 0 || idx == len(raw)-1 {
			return nil, fmt.Errorf("invalid --link %q (want \"Label=URL\")", raw)
		}
		out = append(out, alert_aggregation.NamedLink{
			Label: strings.TrimSpace(raw[:idx]),
			URL:   strings.TrimSpace(raw[idx+1:]),
		})
	}
	return out, nil
}
