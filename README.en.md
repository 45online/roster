# Roster

**English** · [中文](README.md)

> Add an AI employee to your team.

Roster is a long-running CLI (local box or VPS) that lets an AI take
over a "virtual employee" GitHub account and run your project
management workflow: events on GitHub get synced to Jira / Confluence /
Slack, with no human shuffling tickets between systems.

Developers stay on GitHub. Management lives in the back office. The AI
does the bridging.

```
GitHub  ←→  Roster (AI employee)  ←→  Jira / Confluence / Slack
                  │
                  └── pluggable LLM (Claude / DeepSeek / Gemini / OpenAI / xAI / ...)
```

---

## Status

**Released: [v0.2.1](https://github.com/45online/roster/releases/tag/v0.2.1)** — every phase the design doc planned is implemented: four end-to-end modules, two-mode budget threshold, undercover identity isolation, polling + webhook event sources, cross-platform binaries + multi-arch Docker, **multi-LLM provider** (Claude / DeepSeek / Gemini / OpenAI / xAI / any OpenAI-compatible endpoint).

**Next chapter: dogfood.** Feature-filling stops here. The next week is
about running Roster against a real repo and watching what assumptions
break (prompt tuning / module boundaries / UX rough edges). Full release
history: [CHANGELOG.md](CHANGELOG.md).

| Phase | Status |
|---|---|
| 0. Fork claude-code-go, rebrand, business layout | ✅ |
| 1. CLI copy + boot logo | ✅ |
| 2. Module A: Issue → Jira (`sync-issue`) | ✅ |
| 2.x. Poller + anti-loop + `takeover` | ✅ |
| 2.y. Claude API smart field extraction | ✅ |
| 2.z₁. JSONL audit + `.roster/config.yml` + `roster init` | ✅ |
| 2.z₂. `roster login` credential management | ✅ |
| 3. Module B: PR AI Review (`review-pr` + takeover) | ✅ |
| 4. Module C: Issue close → Confluence (`archive-issue` + takeover) | ✅ |
| 5. Module D: alert aggregation → Slack (`aggregate-alert`, no AI) | ✅ |
| 6.a `roster status` + `roster logs` observability | ✅ |
| 6.b Budget tracking (token + USD, monthly rollup) | ✅ |
| 6.c Budget threshold — stop mode | ✅ |
| 6.c+ Budget threshold — downgrade mode (no AI, daemon stays) | ✅ |
| 6.d Webhook mode + GitHub HMAC verification | ✅ |
| 7. Undercover Mode (identity isolation + secret redaction) | ✅ |
| 8. Containerise + CI (Dockerfile + Actions + GHCR) | ✅ |
| 9. Multi-LLM provider (Anthropic / OpenAI-compatible) | ✅ v0.2.0 |
| 10. Helm chart (Kubernetes deployment) | ✅ v0.2.1 |
| 11. Slack slash command (`/roster …`) | ✅ v0.2.1 |

---

## Core idea

- **GitHub = single source of truth**. Developers only file Issues / push code / comment on PRs there.
- **Jira / Confluence = automatic mirrors**. Synced by AI, read-only for management.
- **Slack = real-time pulse**. AI pushes alerts / notifications / review summaries.
- **AI employee = virtual human account**. No `[bot]` tag — has a name, an avatar, looks like a regular GitHub collaborator. Just every action is performed by Roster on its behalf.

Analogy: like an AI coding assistant is to code, **Roster is to project management**.

---

## Four modules

| Module | What it does |
|---|---|
| **A. Issue → Jira** | New GitHub Issue → AI extracts fields → Jira ticket created automatically |
| **B. PR AI Review** | New PR → AI static analysis + line comments (optional local checkout to run tests) |
| **C. Issue close → Confluence** | Issue closed → AI summarises the thread → Confluence draft (human publishes) |
| **D. Alert aggregation** | External alert → AI correlates recent commits/deploys → Slack public channel (log-board style) |

---

## Quick start

### Prerequisites
- A virtual-employee GitHub account (with a PAT)
- An LLM API key — any of: Anthropic Claude, DeepSeek (cheapest), Google Gemini, OpenAI, xAI Grok, Together, Groq, etc.
- (Optional) Jira / Confluence / Slack API tokens

> **LLM provider**: any OpenAI Chat Completions-compatible endpoint works (DeepSeek / Gemini OpenAI-compat / OpenAI / xAI / Together / Groq). Configure with `roster login llm`. DeepSeek is currently the cheapest capable option (~10× cheaper than Claude Haiku, ~30× cheaper than Claude Sonnet for simple structured extraction).

### Install (pick one)

**A. From source** (Go 1.26+)
```bash
git clone https://github.com/45online/roster.git
cd roster
make build
./bin/roster --help
```

**B. Docker** (zero deps)
```bash
docker pull ghcr.io/45online/roster:v0.2.1   # or :latest
docker run --rm ghcr.io/45online/roster:v0.2.1 --version
# → roster v0.2.1
```

Full run (mount `~/.roster` for persistent creds + audit + cursor;
`-w /work` puts the command in the mounted repo):
```bash
docker run --rm \
  -v "$HOME/.roster:/home/roster/.roster" \
  -v "$PWD:/work" -w /work \
  -e ROSTER_GITHUB_TOKEN -e ROSTER_JIRA_TOKEN -e ROSTER_JIRA_URL -e ROSTER_JIRA_EMAIL -e ROSTER_LLM_API_KEY \
  ghcr.io/45online/roster:v0.2.1 takeover --repo owner/name
```

Multi-arch (linux/amd64, linux/arm64), final image ~40 MB.

**C. Homebrew** (post-release)
```bash
brew install 45online/tap/roster
```

**D. Helm chart (Kubernetes)**

```bash
# Create credentials Secret (recommended for prod; lab use can inline values)
kubectl create secret generic roster-creds \
  --from-literal=ROSTER_GITHUB_TOKEN=ghp_xxx \
  --from-literal=ROSTER_LLM_API_KEY=sk-...

# Install
helm install roster ./charts/roster \
  --set repo=owner/name \
  --set credentials.existingSecret=roster-creds
```

Full guide: [charts/roster/README.md](charts/roster/README.md) — covers webhook + Ingress + TLS, persistence, production knobs. **Single-pod by design** (cursor can't be concurrent); install N releases to manage N repos.

---

## Configuration

Two paths, pick one — env vars (CI-friendly) or `roster login` (interactive, persisted at `~/.roster/credentials.json` mode 0600).

```bash
# Path A: interactive login (recommended for local / VPS)
roster login github     # paste PAT
roster login jira       # URL / email / token
roster login llm        # provider / base_url / model / api_key
roster login status     # show what's configured

# Path B: env vars (CI / Docker)
export ROSTER_GITHUB_TOKEN=ghp_xxx
export ROSTER_JIRA_URL=https://yourorg.atlassian.net
export ROSTER_JIRA_EMAIL=you@example.com
export ROSTER_JIRA_TOKEN=xxxx

# LLM (any one provider/model combo)
export ROSTER_LLM_PROVIDER=openai-compatible
export ROSTER_LLM_BASE_URL=https://api.deepseek.com
export ROSTER_LLM_MODEL=deepseek-chat
export ROSTER_LLM_API_KEY=sk-...
# or stay on the legacy Anthropic-only path:
# export ANTHROPIC_API_KEY=sk-ant-xxx
```

### LLM provider options

| Provider | base_url | suggested model | price (in/out per Mtok) |
|---|---|---|---|
| Anthropic Claude | (default) | `claude-haiku-4-5-20251001` | $1 / $5 |
| Anthropic Claude | (default) | `claude-sonnet-4-6-20250514` | $3 / $15 |
| **DeepSeek** (cheap) | `https://api.deepseek.com` | `deepseek-chat` | $0.27 / $1.10 |
| Google Gemini | `https://generativelanguage.googleapis.com/v1beta/openai/` | `gemini-2.5-flash` | $0.075 / $0.30 |
| OpenAI | `https://api.openai.com/v1` | `gpt-4o-mini` | $0.15 / $0.60 |
| xAI Grok | `https://api.x.ai/v1` | `grok-3` | $2 / $10 |

`roster status` accumulates month-to-date cost using the actual model rate (price table is built-in).

---

## Module usage

### A. Issue → Jira (one-shot)
```bash
./bin/roster sync-issue --repo owner/name --issue 42 --jira-project ABC
# → ✓ Created ABC-123
#   Comment posted on GH issue #42: 📋 Tracking in Jira: ABC-123
```

### B. Background daemon (auto-dispatch on issues.opened, PR opened/sync, issue closed)
```bash
cd <your-repo>
roster init                          # generate .roster/config.yml
# edit jira_project, enable other modules…
roster takeover --repo owner/name
# → ✓ Loaded config from .roster/config.yml
#   ✓ Authenticated as @virtual-employee (anti-loop filter armed)
#   ✓ AI extractor enabled (provider=openai-compatible · model=deepseek-chat)
#   [poller] starting (interval=30s, ...)
#   [mod-a] dispatching: ...#43 → ✓ ABC-124
# Ctrl+C to stop. Cursor + audit log persist under ~/.roster/.
```

### C. PR AI Review
```bash
./bin/roster review-pr --repo owner/name --pr 42
# → ✓ Review submitted (comment, 2 inline comments)
# Default: every verdict downgraded to COMMENT (human still has to Approve / Block).
# --can-approve / --can-request-changes unlock those gates.
```

In `.roster/config.yml`:
```yaml
modules:
  pr_review:
    enabled: true
    skip_paths: ["docs/", "*.md"]
    max_diff_bytes: 65536
    can_approve: false
    can_request_changes: false
```

`roster takeover` triggers reviews on PR opened / synchronize. Draft PRs are auto-skipped.

### D. Issue close → Confluence draft
```bash
./bin/roster archive-issue \
  --repo owner/name --issue 42 \
  --space-id 12345 \
  --slack-channel "#archives"   # optional
# → ✓ Draft created (id=987654)
#   https://yourorg.atlassian.net/wiki/...
# Draft is owner-only until a human clicks Publish.
```

In `.roster/config.yml`:
```yaml
modules:
  issue_to_confluence:
    enabled: true
    space_id: "12345"
    completed_label: completed   # only archive issues with this label
    slack_channel: "#archives"   # optional
```

### E. Alert aggregation → Slack
No Claude required (templated, deterministic, $0).
```bash
roster aggregate-alert \
  --repo owner/name \
  --slack-channel "#oncall" \
  --source CloudWatch \
  --severity critical \
  --title "5xx error rate at 8.2%" \
  --body "Threshold 2%, sustained 5min" \
  --lookback 1h \
  --link "Logs=https://..." \
  --link "Runbook=https://wiki/runbook"
```

Slack message:
```
🚨 [CloudWatch] 5xx error rate at 8.2%
> Threshold 2%, sustained 5min
_Time: 2026-05-03T14:23:00Z_
_Repo: <https://github.com/owner/name|owner/name>_

📋 *Recent activity:*
• `a3f9c1d` <…|commit> by @alice — "fix: rate limiter"  _12m ago_
• <…|PR #234> merged by @bob — "auth refactor"  _28m ago_
• `b2e4d5a` <…|commit> by @carol — "config: bump db pool"  _45m ago_

🔗 <https://...|Logs>  ·  <https://wiki/runbook|Runbook>
```

**Design philosophy**: Module D plays the "log board" role — list recent activity for oncall to judge themselves. No causal attribution, no @-mentions, no ticket creation. Wrong attribution is worse than no attribution.

---

## Operations

### `roster status`
One screen for everything: credentials state, repos under management,
recent 24h activity per module, latest error, **month-to-date budget** with per-module breakdown.

```
Roster status — 2026-05-03T14:23:00Z
Base dir: /Users/me/.roster

Credentials:
  github  ✓ configured
  jira    ✓ configured
  slack   ✗ not set
  claude  ✗ not set
  llm     ✓ configured  (openai-compatible / deepseek-chat)

Projects (1, last 24h):

  foo/bar
    cursor       last polled 30m ago, event_id=…
    audit        3 events: 2 success, 0 partial, 1 error, 0 skipped
    by module    issue_to_jira=1, pr_review=2
    last event   15m ago
    last error   15m ago — diff too large
    budget MTD   $0.04 over 3 AI calls (issue_to_jira=$0.00, pr_review=$0.04) · 29k in / 1k out
```

### `roster logs <repo>`
Tail the audit JSONL with filters: `--module` / `--status` / `--since 30m` / `-f` follow.
```
$ roster logs foo/bar --status error -f
2026-05-03T14:25Z [pr_review/error] foo/bar#3 by @carol [50ms]  ! diff too large
2026-05-03T14:30Z [issue_to_jira/error] foo/bar#7 by @alice [820ms]  ! create jira issue: 401 Unauthorized
```
Both commands accept `--json` for programmatic consumption.

### Budget threshold

`.roster/config.yml`:
```yaml
budget:
  monthly_usd: 50
  on_exceed: downgrade   # 'stop' | 'downgrade'
```

- `stop` (default): refuse to start when already over; cancel daemon at first event past the cap.
- `downgrade`: keep daemon running but disable AI calls — Module A falls back to mechanical label mapping (still files Jira tickets), Modules B/C are skipped, Module D unaffected (no AI calls anyway). Auto-restores when MTD drops below cap (e.g. month rollover).

Audit-read failure is **fail-open** — never freeze the daemon over a missing file.

### Webhook mode (replaces polling)

`.roster/config.yml`:
```yaml
webhook:
  enabled: true
  listen: ":8080"
  path: /webhook/github
  secret: ""                 # or export ROSTER_WEBHOOK_SECRET=...
```

Then on GitHub repo Settings → Webhooks:
- Payload URL = `<public URL>/webhook/github`
- Content type = `application/json`
- Secret = same as `ROSTER_WEBHOOK_SECRET`
- Tick "Issues" and "Pull requests"

Features: HMAC-SHA256 verification (constant-time), event mapping `issues→IssuesEvent` / `pull_request→PullRequestEvent`, `ping` returns 200, `/healthz`, anti-loop, 5MB body cap. **Mutually exclusive** with polling — webhook UUIDs and events-API IDs don't dedupe across the two sources.

### Slack slash command (optional)

When webhook mode is on, team members can trigger Roster from Slack:

```
/roster status                         show Roster's current state
/roster sync-issue owner/name#42       trigger Module A
/roster review-pr  owner/name#42       trigger Module B
/roster archive-issue owner/name#42    trigger Module C
/roster help                           show this list
```

`.roster/config.yml`:
```yaml
slack:
  enabled: true
  path: /slack/command
  signing_secret: ""    # or export ROSTER_SLACK_SIGNING_SECRET=...
```

Configuring the Slack app:
1. [api.slack.com/apps](https://api.slack.com/apps) → Create New App → From scratch
2. Slash Commands → Create New Command:
   - Command: `/roster`
   - Request URL: `https://<your-public-host>/slack/command`
3. Basic Information → copy "Signing Secret" → put it in `signing_secret` (or env)
4. Install to Workspace

Features: HMAC-v0 signature verification (constant-time + 5-min replay window). `status` runs synchronously and returns inline; `sync-issue` / `review-pr` / `archive-issue` immediately ack with `:hourglass_flowing_sand: queued` and run in a background goroutine (Slack's 3-second response window forbids running the full module call inline). Results land in GitHub / Jira / Confluence directly, not back in Slack. The dispatcher guards against cross-repo invocations: a Roster pod manages a single repo, and the Slack command's repo must match.

### Undercover Mode (always on)

The virtual employee is, to outside readers, indistinguishable from a human teammate. Two layers:

1. **Prompt layer** — every AI module's system prompt appends an invariant: don't say "As an AI", don't name the vendor, don't surface the model id, no disclaimers.
2. **Output layer** — every text bound for GitHub / Jira / Confluence / Slack runs through a regex scrubber:
   - Secrets (`sk-ant-*`, `ghp_*`, `xox[abp]-*`) → explicit markers
   - Vendor / model identifiers → silently stripped
   - AI self-disclosure phrases → silently stripped or rewritten

PR review bodies don't carry an `🤖 AI Review (model)` header — instead a discreet `_automated review_` footer keeps real reviewers oriented without breaking persona. Module D (alert aggregation) also runs the redactor, since it quotes commit messages and PR titles verbatim.

---

## Source

Roster is built on top of [claude-code-go](https://github.com/tunsuy/claude-code-go) (MIT). It directly reuses the upstream `internal/api`, `internal/tools`, `internal/engine`, `internal/coordinator`, etc., and adds the `adapters/`, `modules/`, `poller/`, `webhookreceiver/`, `audit/`, `budget/`, `creds/`, `projcfg/`, and `undercover/` packages on top.

See [NOTICE](NOTICE) for the fork relationship.

---

## License

[MIT](LICENSE) — see [LICENSE](LICENSE) and [LICENSE.upstream](LICENSE.upstream) for full attribution.
