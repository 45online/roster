package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/45online/roster/internal/adapters/confluence"
	gh "github.com/45online/roster/internal/adapters/github"
	"github.com/45online/roster/internal/adapters/jira"
	"github.com/45online/roster/internal/adapters/slack"
	"github.com/45online/roster/internal/api"
	"github.com/45online/roster/internal/audit"
	"github.com/45online/roster/internal/budget"
	"github.com/45online/roster/internal/memory"
	"github.com/45online/roster/internal/modules/issue_to_confluence"
	"github.com/45online/roster/internal/modules/issue_to_jira"
	"github.com/45online/roster/internal/modules/pr_review"
	"github.com/45online/roster/internal/poller"
	"github.com/45online/roster/internal/projcfg"
	"github.com/45online/roster/internal/slackbridge"
	"github.com/45online/roster/internal/webhookreceiver"
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

			// Project memory: read once at startup. Cheap (≤64 KB) and
			// stable for the daemon's lifetime; if conventions change
			// just restart. Errors are non-fatal — empty memory means
			// modules behave like v0.2.x.
			memDir := "."
			if cfgFile != "" {
				memDir = filepath.Dir(filepath.Dir(cfgFile)) // <root>/.roster/config.yml → <root>
			}
			mem, memErr := memory.Load(memDir)
			if memErr != nil {
				fmt.Fprintf(os.Stderr, "⚠ memory load: %v (continuing with empty memory)\n", memErr)
			}
			if !mem.Empty() {
				fmt.Printf("✓ Project memory loaded: %d files, %d bytes\n",
					len(mem.LoadedFiles()), mem.Bytes())
			}

			modA := issue_to_jira.New(ghClient, jiraClient, issue_to_jira.Config{
				JiraProject:      jiraProject,
				DefaultIssueType: orDefault(cfg.Modules.IssueToJira.DefaultIssueType, "Task"),
				LabelToIssueType: orMap(cfg.Modules.IssueToJira.LabelToIssueType, map[string]string{"bug": "Bug"}),
				PriorityMapping: orMap(cfg.Modules.IssueToJira.PriorityMapping, map[string]string{
					"P0": "Highest",
					"P1": "High",
					"P2": "Medium",
				}),
			}).WithAudit(recorder).WithMemory(mem)

			// Optional AI extractor for Modules A / B / C — supports any
			// configured LLM provider (Anthropic / OpenAI-compatible).
			var apiClient api.Client
			var llmModel string
			if llmCfg, ok := r.llm(cfg.LLM); ok {
				if c, apiErr := llmCfg.NewClient(); apiErr == nil {
					apiClient = c
					llmModel = llmCfg.Model
					modA = modA.WithExtractor(issue_to_jira.NewExtractor(apiClient, llmModel))
					fmt.Printf("✓ AI extractor enabled (provider=%s%s)\n",
						llmCfg.Provider, modelHint(llmModel))
				} else {
					fmt.Fprintf(os.Stderr, "⚠ LLM client init failed (%v); modules will run without AI\n", apiErr)
				}
			}

			// Module B (PR AI Review): enabled by config + Claude availability.
			var modB *pr_review.Module
			if cfg.Modules.PRReview.Enabled {
				if apiClient == nil {
					fmt.Fprintln(os.Stderr, "⚠ pr_review.enabled=true but no LLM provider configured — Module B disabled")
				} else {
					modB = pr_review.New(ghClient, apiClient, llmModel, pr_review.Config{
						SkipPaths:         cfg.Modules.PRReview.SkipPaths,
						MaxDiffBytes:      cfg.Modules.PRReview.MaxDiffBytes,
						CanApprove:        cfg.Modules.PRReview.CanApprove,
						CanRequestChanges: cfg.Modules.PRReview.CanRequestChanges,
					}).WithAudit(recorder).WithMemory(mem)
					fmt.Println("✓ Module B (PR review) armed")
				}
			}

			// Module C (Issue close → Confluence draft): enabled by config +
			// Claude + Atlassian credentials.
			var modC *issue_to_confluence.Module
			if cfg.Modules.IssueToConfluence.Enabled {
				switch {
				case apiClient == nil:
					fmt.Fprintln(os.Stderr, "⚠ issue_to_confluence.enabled=true but no LLM provider configured — Module C disabled")
				case cfg.Modules.IssueToConfluence.SpaceID == "":
					fmt.Fprintln(os.Stderr, "⚠ issue_to_confluence.enabled=true but space_id is empty — Module C disabled")
				default:
					confClient := confluence.NewClient(jiraURL, jiraEmail, jiraToken)
					var slackCli *slack.Client
					if cfg.Modules.IssueToConfluence.SlackChannel != "" {
						if s := r.load(); s.Has("slack") {
							slackCli = slack.NewClient(s.Slack.Token)
						} else if tok := os.Getenv("ROSTER_SLACK_TOKEN"); tok != "" {
							slackCli = slack.NewClient(tok)
						} else {
							fmt.Fprintln(os.Stderr, "⚠ slack_channel set but no Slack token; Module C will skip notifications")
						}
					}
					modC = issue_to_confluence.New(ghClient, confClient, slackCli, apiClient, llmModel, issue_to_confluence.Config{
						SpaceID:        cfg.Modules.IssueToConfluence.SpaceID,
						ParentPageID:   cfg.Modules.IssueToConfluence.ParentPageID,
						CompletedLabel: cfg.Modules.IssueToConfluence.CompletedLabel,
						SlackChannel:   cfg.Modules.IssueToConfluence.SlackChannel,
					}).WithAudit(recorder).WithMemory(mem)
					fmt.Println("✓ Module C (issue archive) armed")
				}
			}

			ctx, cancel := signalContext()
			defer cancel()

			selfLogin, err := ghClient.AuthUser(ctx)
			if err != nil {
				return fmt.Errorf("identify virtual employee account: %w", err)
			}
			fmt.Printf("✓ Authenticated as @%s (anti-loop filter armed)\n", selfLogin)

			// Budget threshold (optional): refuses to start if already
			// over (when on_exceed=stop). When on_exceed=downgrade,
			// flips a flag that suppresses AI-spending paths but keeps
			// the daemon running.
			var (
				threshold  *budget.Threshold
				aiAllowed  atomic.Bool
			)
			aiAllowed.Store(true) // start permissive
			if cfg.Budget.MonthlyUSD > 0 {
				threshold = budget.NewThreshold(recorder, repo, cfg.Budget.MonthlyUSD, cfg.Budget.OnExceed)
				d := threshold.Check(time.Now().UTC())
				fmt.Printf("✓ Budget MTD: $%.2f / $%.2f cap (on_exceed=%s)\n",
					d.MTDUSD, d.Limit, orDefault(cfg.Budget.OnExceed, "stop"))
				if d.ShouldStop {
					return fmt.Errorf("budget already exceeded ($%.2f / $%.2f); refusing to start (set on_exceed=downgrade or raise the cap)",
						d.MTDUSD, d.Limit)
				}
				if d.ShouldDowngrade {
					aiAllowed.Store(false)
					fmt.Println("⚠ already past cap; starting in downgraded mode (no AI calls)")
				}
				// Module A: skip the extractor when downgraded; mechanical
				// label mapping still produces a Jira ticket.
				modA = modA.WithAIGuard(func() bool { return aiAllowed.Load() })
			}

			handler := func(ctx context.Context, ev gh.Event) error {
				// Budget gate — runs before every dispatch.
				//   on_exceed=stop      → cancel daemon ctx; poller exits.
				//   on_exceed=downgrade → flip aiAllowed off; modules
				//                         requiring AI are skipped this
				//                         tick, modA falls back to
				//                         mechanical mapping.
				downgraded := false
				if threshold != nil {
					d := threshold.Check(time.Now().UTC())
					if d.ShouldStop {
						log.Printf("⛔ budget exceeded: MTD $%.2f / cap $%.2f — stopping per on_exceed=stop",
							d.MTDUSD, d.Limit)
						cancel()
						return fmt.Errorf("budget exceeded")
					}
					if d.ShouldDowngrade {
						if aiAllowed.Swap(false) {
							log.Printf("⚠ budget exceeded: MTD $%.2f / cap $%.2f — downgrading (no AI calls until next month)",
								d.MTDUSD, d.Limit)
						}
						downgraded = true
					} else if !aiAllowed.Load() {
						// Cost dropped back under cap (e.g. month rolled over).
						aiAllowed.Store(true)
						log.Printf("✓ budget within cap: MTD $%.2f / cap $%.2f — restoring full operation",
							d.MTDUSD, d.Limit)
					}
				}
				switch ev.Type {
				case "IssuesEvent":
					p, err := ev.DecodeIssuesPayload()
					if err != nil {
						return err
					}
					switch p.Action {
					case "opened":
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
					case "closed":
						if modC == nil {
							return nil
						}
						if downgraded {
							log.Printf("[mod-c] ⏭  skipped (budget downgrade): %s#%d", repo, p.Issue.Number)
							return nil
						}
						log.Printf("[mod-c] dispatching: %s#%d closed (by @%s)",
							repo, p.Issue.Number, ev.Actor.Login)
						res, err := modC.ArchiveIssue(ctx, repo, p.Issue.Number)
						if err != nil && res == nil {
							return err
						}
						if err != nil {
							log.Printf("[mod-c] partial: %s created, slack failed: %v", res.PageID, err)
							return nil
						}
						if res.Skipped {
							log.Printf("[mod-c] ⏭  skipped: %s", res.Reason)
							return nil
						}
						log.Printf("[mod-c] ✓ draft %s → %s", res.PageID, res.PageURL)
						return nil
					}
					return nil

				case "PullRequestEvent":
					if modB == nil {
						return nil
					}
					if downgraded {
						log.Printf("[mod-b] ⏭  skipped (budget downgrade)")
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

			// Two event sources, mutually exclusive: webhook receiver OR
			// poller. Webhook is push (low-latency, no API quota cost,
			// needs a public endpoint); poller is pull (works anywhere,
			// 30s lag).
			if cfg.Webhook.Enabled {
				secret := cfg.Webhook.Secret
				if secret == "" {
					secret = os.Getenv("ROSTER_WEBHOOK_SECRET")
				}
				if secret == "" {
					return fmt.Errorf("webhook.enabled=true but no secret (set webhook.secret in config or ROSTER_WEBHOOK_SECRET in env)")
				}

				// Optional Slack slash-command bridge (shares the HTTP
				// listener with the GitHub webhook on a different path).
				extraRoutes := map[string]http.Handler{}
				if cfg.Slack.Enabled {
					slackSecret := cfg.Slack.SigningSecret
					if slackSecret == "" {
						slackSecret = os.Getenv("ROSTER_SLACK_SIGNING_SECRET")
					}
					if slackSecret == "" {
						return fmt.Errorf("slack.enabled=true but no signing_secret (set slack.signing_secret or ROSTER_SLACK_SIGNING_SECRET)")
					}
					path := orDefault(cfg.Slack.Path, "/slack/command")
					extraRoutes[path] = &slackbridge.Handler{
						Secret: slackSecret,
						Dispatcher: &takeoverSlackDispatcher{
							repo: repo,
							modA: modA,
							modB: modB,
							modC: modC,
						},
					}
					fmt.Printf("✓ Slack slash-command receiver listening on %s (configure /roster command's Request URL accordingly)\n", path)
				}

				srv, err := webhookreceiver.NewServer(webhookreceiver.Config{
					Listen:      cfg.Webhook.Listen,
					Path:        cfg.Webhook.Path,
					Secret:      secret,
					SelfLogin:   selfLogin,
					Handler:     handler,
					ExtraRoutes: extraRoutes,
				})
				if err != nil {
					return err
				}
				fmt.Printf("✓ Webhook mode (poller disabled); GitHub repo Settings → Webhooks must point at <public-url>%s\n",
					orDefault(cfg.Webhook.Path, "/webhook/github"))
				err = srv.Run(ctx)
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}

			if cfg.Slack.Enabled && !cfg.Webhook.Enabled {
				fmt.Fprintln(os.Stderr, "⚠ slack.enabled=true but webhook.enabled=false — Slack receiver requires the embedded HTTP server; skipping")
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

// modelHint formats the model id (if any) as " · model=<name>" so the
// startup banner can append it without hardcoding punctuation.
func modelHint(model string) string {
	if model == "" {
		return ""
	}
	return " · model=" + model
}

// takeoverSlackDispatcher adapts the daemon's already-wired modules to
// the slackbridge.Dispatcher interface. Each method does a repo guard
// (this daemon instance manages a single repo; trying to act on a
// different one is a no-op) and translates Module Result structs into
// short human-readable strings for the Slack acknowledgement log line.
type takeoverSlackDispatcher struct {
	repo string
	modA *issue_to_jira.Module
	modB *pr_review.Module
	modC *issue_to_confluence.Module
}

func (d *takeoverSlackDispatcher) checkRepo(repo string) error {
	if repo != d.repo {
		return fmt.Errorf("this Roster instance manages %q, not %q", d.repo, repo)
	}
	return nil
}

func (d *takeoverSlackDispatcher) SyncIssue(ctx context.Context, repo string, n int) (string, error) {
	if err := d.checkRepo(repo); err != nil {
		return "", err
	}
	res, err := d.modA.SyncIssue(ctx, repo, n)
	if err != nil && res == nil {
		return "", err
	}
	return fmt.Sprintf("Created %s", res.JiraKey), nil
}

func (d *takeoverSlackDispatcher) ReviewPR(ctx context.Context, repo string, n int) (string, error) {
	if d.modB == nil {
		return "", fmt.Errorf("Module B (pr_review) is not enabled")
	}
	if err := d.checkRepo(repo); err != nil {
		return "", err
	}
	res, err := d.modB.ReviewPR(ctx, repo, n)
	if err != nil {
		return "", err
	}
	if res.Skipped {
		return "Skipped: " + res.SkipReason, nil
	}
	return fmt.Sprintf("Submitted (%s, %d inline comments)", res.Verdict, res.CommentCount), nil
}

func (d *takeoverSlackDispatcher) ArchiveIssue(ctx context.Context, repo string, n int) (string, error) {
	if d.modC == nil {
		return "", fmt.Errorf("Module C (issue_to_confluence) is not enabled")
	}
	if err := d.checkRepo(repo); err != nil {
		return "", err
	}
	res, err := d.modC.ArchiveIssue(ctx, repo, n)
	if err != nil && res == nil {
		return "", err
	}
	if res.Skipped {
		return "Skipped: " + res.Reason, nil
	}
	return "Draft " + res.PageID + " created", nil
}

func (d *takeoverSlackDispatcher) Status(ctx context.Context) (string, error) {
	return fmt.Sprintf("Roster managing *%s* — run `roster status` from a shell for the full dashboard.", d.repo), nil
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
