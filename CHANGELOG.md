# Changelog

All notable changes to Roster will be documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

Nothing pending after v0.2.1. Real-world dogfood feedback is the next
chapter — both the multi-LLM provider switch (DeepSeek vs Claude on
actual workloads) and the operational paths just shipped (Helm /
Slack).

---

## [v0.2.1] — 2026-05-03

**Helm chart + Slack slash command.** Two operational additions on top
of v0.2.0's multi-LLM-provider work; binary semantics unchanged.

### Added
- `charts/roster/` Helm chart (chart v0.1.0, appVersion 0.2.1):
  - Deployment + ConfigMap + Secret + PVC, optional Service + Ingress
    when webhook mode is on.
  - `replicas: 1` + `Recreate` strategy (cursor isn't concurrent-safe).
  - `repo` is required; chart fails fast when missing.
  - `credentials.existingSecret` lets users keep keys in Sealed
    Secrets / External Secrets / SOPS instead of Helm values history.
  - Pod annotations (`checksum/config`, `checksum/secret`) roll the
    deployment when the rendered config / chart-managed secret changes.
- `internal/slackbridge/` Slack slash-command receiver:
  - HMAC-v0 signature verification (`X-Slack-Signature` /
    `X-Slack-Request-Timestamp`) with a 5-minute replay window.
  - Verbs: `help`, `status`, `sync-issue`, `review-pr`, `archive-issue`.
  - `status` runs synchronously (cheap); the three module verbs
    immediately ack with `queued` and run in a background goroutine
    so the 3-second Slack response budget isn't blown.
  - Single-repo guard: the Slack command's repo must match the repo
    the daemon manages.
  - 18 tests covering signature, parser, and HTTP integration.
- `internal/projcfg.Slack` config block — enabled / path /
  signing_secret. Validation guarded; env override
  `ROSTER_SLACK_SIGNING_SECRET`.
- `internal/webhookreceiver.Config.ExtraRoutes` so Slack reuses the
  same HTTP listener as the GitHub webhook (different path).

### Changed
- `roster takeover` startup logs a "✓ Slack slash-command receiver
  listening on /slack/command" line when both webhook and slack are
  enabled.

### Notes
- Slack mode requires `webhook.enabled=true` (it shares the HTTP
  listener). A warning is printed if `slack.enabled=true` is set
  without webhook.

---

## [v0.2.0] — 2026-05-03

**Multi-LLM provider.** Roster's AI calls are no longer hard-bound to
Claude. Configure any OpenAI Chat Completions-compatible endpoint
(DeepSeek, xAI, Gemini's OpenAI-compat, OpenAI, Together, Groq, ...)
via `roster login llm` or `ROSTER_LLM_*` env vars.

Why this matters: Module A / B / C all do simple structured extraction.
DeepSeek-Chat at $0.27/M input is ~10× cheaper than Claude Haiku for
the same job, ~30× cheaper than Claude Sonnet.

### Added
- `internal/creds.LLMCreds` (`provider` / `base_url` / `model` /
  `api_key`) and matching `Has("llm")` / `Clear("llm")`. Legacy
  `ClaudeCreds` remains for backward compat.
- `internal/projcfg.LLM` block in `.roster/config.yml`. `Validate()`
  rejects unknown providers and missing `base_url` / `model` for
  openai-compatible.
- `roster login llm` subcommand (provider / base_url / model / api_key
  prompts). `roster login status` shows the current provider+model.
- `roster login logout llm` — symmetric removal.
- 5-layer credential resolution: env vars → `~/.roster/credentials.json
  llm` → project config → legacy `ANTHROPIC_API_KEY` →
  `~/.roster/credentials.json claude`. v0.1.x users upgrade with no
  config change.
- `internal/budget/pricing.go` extended with GPT-4o / 4o-mini / 4.1[-mini],
  DeepSeek chat / reasoner, Gemini 2.0/2.5 flash + 2.5 pro, Grok 2/3.
  `roster status` MTD cost tracks each model's actual rate.

### Changed
- `roster takeover` startup banner: `✓ AI extractor enabled
  (provider=openai-compatible · model=deepseek-chat)` instead of
  `✓ Claude extractor enabled`.
- `roster sync-issue` / `review-pr` / `archive-issue` switched from
  the legacy `.claude()` resolver to the unified `.llm(cfg.LLM)` one.

### Fixed
- TUI welcome banner no longer falls back to a hardcoded vendor model
  string (`claude-sonnet-4-20250514`) when no model is configured —
  instead nudges the operator to fill in `llm.model`. Undercover
  invariant: don't leak vendor defaults via display.
- Removed " · API Usage Billing" from the welcome banner — that was
  upstream Claude Code's billing hint and meaningless for Roster.
- `promptLine()` rebuilt `bufio.NewReader(os.Stdin)` on every call,
  losing buffered bytes — the second prompt of a multi-prompt
  subcommand saw EOF. Hoisted to a single package-level reader.
  Surfaced by `roster login llm` (4 prompts);
  `roster login github` (1 prompt) didn't trigger it.

### Visual polish
- Welcome banner is now wrapped in a rounded card with the indigo
  brand colour on the border.

---

## [v0.1.5] — 2026-05-03

**Webhook mode.** GitHub events arrive via HTTP push instead of 30s
polling — sub-second latency, no GitHub API quota cost. Polling stays
the default; webhook is opt-in.

### Added
- `internal/webhookreceiver`: embedded HTTP server with two routes:
  - `POST /webhook/github` for deliveries
  - `GET /healthz` for LB / tunnel health checks
- HMAC-SHA256 signature verification on `X-Hub-Signature-256` (constant-
  time compare). Bad/missing signature → 401.
- `ping` event handled with a 200 "pong" so the GitHub setup probe goes
  green; unknown event types → 200 "ignored" (GitHub stops retrying).
- 5 MB request-body cap (DoS guard).
- `webhook` block in `.roster/config.yml`: `enabled` / `listen` / `path`
  / `secret`. `ROSTER_WEBHOOK_SECRET` env var as fallback.
- `roster takeover` runs the receiver instead of the poller when
  `webhook.enabled=true`. They are mutually exclusive — webhook delivery
  UUIDs and events-API IDs are independent spaces, so running both
  produces duplicate dispatches.
- 13 tests cover signature match/mismatch/tampered/malformed, event-type
  mapping, the full HTTP integration via `httptest`, anti-loop, GET → 405.

---

## [v0.1.4] — 2026-05-03

**Budget `downgrade` mode.** Completes the two-mode threshold contract.

### Added
- `Threshold.Decision` gains a `ShouldDowngrade` field. `Check()`
  branches on `OnExceed`: `stop` / `downgrade` / `unknown→stop`. The
  two action flags are mutually exclusive (asserted by tests).
- `Module A` exposes `WithAIGuard(func() bool)`. Returning `false`
  bypasses the Claude extractor for that invocation; mechanical label
  mapping still produces a Jira ticket.
- `roster takeover` flips an `atomic.Bool` on `ShouldDowngrade` and
  passes it to Module A through the AIGuard. Module B (PR review) and
  Module C (Confluence archive) are skipped during downgrade. Module D
  is unaffected (no AI calls).

### Changed
- Edge-triggered logging: a single `⚠ downgrading` line on entry, a
  single `✓ restoring full operation` line when MTD drops back under
  cap (e.g. month rollover).

---

## [v0.1.3] — 2026-05-03

**Undercover Mode.** The "virtual employee is indistinguishable from a
human teammate" invariant becomes code-enforced.

### Added
- `internal/undercover`:
  - `SystemSuffix` appended to every Claude module's system prompt —
    forbids self-identifying as AI / bot / Claude / vendor and bans the
    "As an AI assistant, ..." disclaimer family.
  - `Redact()` runs as a final scrub on every outbound text:
    - secrets (`sk-ant-*`, `ghp_*`, `xox[abp]-*`) → explicit markers
    - vendor / model identifiers → silently stripped
    - AI-disclaimer phrases → silently stripped or rewritten
- 9 redactor tests including idempotency and a "leave bare 'Claude'
  alone" false-positive guard.

### Changed
- `pr_review` body no longer prefixes with `🤖 AI Review (model)`. A
  subtle `_automated review_` footer keeps real reviewers oriented
  without breaking persona. `buildReviewBody` signature drops the
  `model` parameter.
- Module A / B / C / D outputs all run through `Redact` before reaching
  GitHub / Jira / Confluence / Slack.

---

## [v0.1.2] — 2026-05-03

**Version banner fix + budget threshold (stop mode).**

### Added
- `internal/version` package — single `var Version = "dev"` so a single
  `-X .../internal/version.Version=...` ldflags target stamps the
  binary. Default `"dev"` is the right signal for a build that didn't
  go through the release pipeline.
- `internal/budget/threshold.go`: `Threshold` + `Decision` with 30s TTL
  cache. `roster takeover` now refuses to start when already over the
  cap, and stops the daemon at first event past the breach.
- 5 threshold tests: nil-safe, below-limit, default-stop on each
  on_exceed, TTL cache, `MarkSpend` incremental update.

### Fixed
- `--version` banner showed `roster 0.1.0` because the previous ldflags
  target (`-X main.version=...`) hit no symbol — `appVersion` was in
  `internal/bootstrap`, with a parallel hardcoded copy in `internal/tui`.
- Both `.goreleaser.yaml` and `Dockerfile` now target the new
  `internal/version` package.
- `docker.yml` build-args use `inputs.ref || github.ref_name`, so a
  workflow_dispatch with `inputs.ref=v0.1.x` stamps the image
  correctly.

---

## [v0.1.1] — 2026-05-03

**Release pipeline fixes.** No business changes.

### Fixed
- `archives.files` referenced `CHANGELOG.md` — deleted in phase 1
  rebrand. Replaced with `LICENSE` + `LICENSE.upstream` + `NOTICE`.
- `format_overrides.format` (deprecated) → `formats: [zip]`.
- Docker `workflow_dispatch` previously only pushed `:latest` because
  `metadata-action` read `github.ref_name` (= `main` in dispatch mode);
  now it also pushes `:v{ref}` when invoked manually.
- `--version` banner said `claude` instead of `roster` (residual
  upstream string).

---

## [v0.1.0] — 2026-05-03

**First end-to-end release.** Every module the design doc planned, in
one binary, distributed three ways.

### Modules (all end-to-end)
- **A. Issue → Jira sync.** GitHub `issues.opened` → Claude extracts
  fields (summary / type / priority / component) → Jira ticket created
  → back-link comment posted on the issue. Mechanical label-based
  mapping is the fallback when Claude isn't available.
- **B. PR AI Review.** PR `opened` / `synchronize` → Claude reads the
  unified diff → posts line comments + verdict. Default verdict gates
  (no APPROVE / REQUEST_CHANGES) keep the AI non-blocking until a human
  flips them on.
- **C. Issue close → Confluence draft.** Issue closed with `completed`
  label → Claude summarises the thread → Confluence draft page (not
  published) → optional Slack ping to the issue owner.
- **D. Alert aggregation → Slack.** External alert system POSTs an
  alert; Roster pairs it with the last hour of commits + merged PRs in
  the repo, posts a single message to a Slack channel. *No AI*: the
  message is templated, deterministic, and explicitly avoids causal
  attribution.

### Foundation
- `roster init` — scaffolds `.roster/config.yml` from a commented template.
- `roster login` — saves credentials at `~/.roster/credentials.json`
  (mode 0600, atomic temp+rename).
- `roster takeover` — long-running poller. 30s cadence, ETag conditional
  fetch, anti-loop on virtual employee login, file-based per-repo cursor
  in `~/.roster/cursors/`.
- `roster sync-issue` / `review-pr` / `archive-issue` / `aggregate-alert`
  — manual single-shot triggers for each module.
- `roster status` — credential / project / 24h activity dashboard
  (with `--json` for programmatic consumption).
- `roster logs <repo>` — tail audit JSONL with `--since` / `--module`
  / `--status` / `-f` filters.
- JSONL audit at `~/.roster/audit/<repo>.jsonl` — every module
  invocation captures inputs, AI usage, tokens, USD cost, outcome,
  duration. Tail-followable.
- Budget tracking: per-call token + cost on every audit row, monthly
  rollup in `roster status` (`budget MTD $X over N AI calls`).

### Distribution
- 5 cross-platform binaries (linux/darwin amd64 + arm64, windows amd64)
  via `goreleaser`.
- Multi-arch Docker image (linux/amd64, linux/arm64), ~40 MB final
  size, published to `ghcr.io/45online/roster`.
- CI: `vet` + `test -race` + `build` + `--help` smoke on every PR.
- Release workflow: tag push → `goreleaser` + `docker push GHCR` in
  parallel.

### Forked from
[`claude-code-go`](https://github.com/tunsuy/claude-code-go) (MIT). See
`NOTICE`. Roster keeps the upstream `internal/api`, `internal/tools`,
`internal/engine`, `internal/coordinator`, etc., and adds the
`adapters/`, `modules/`, `poller/`, `audit/`, `budget/`, `creds/`,
`projcfg/`, `undercover/`, and `webhookreceiver/` packages on top.
