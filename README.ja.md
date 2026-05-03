# Roster

[中文](README.md) · [English](README.en.md) · **日本語** · [한국어](README.ko.md) · [Español](README.es.md)

> チームに AI のスタッフを一人追加する。

Roster は常駐型 CLI(ローカル / VPS)で、「仮想スタッフ」の GitHub アカウントを AI に運用させ、プロジェクト管理ワークフローを自動で回します。GitHub 上で起きた出来事は Jira / Confluence / Slack に同期され、人間が手動でチケットを行き来させる必要がなくなります。

開発者は GitHub だけにいる。管理は裏方にいる。AI が橋渡しをする。

```
GitHub  ←→  Roster (AI スタッフ)  ←→  Jira / Confluence / Slack
                  │
                  └── 切替可能な LLM(Claude / DeepSeek / Gemini / OpenAI / xAI / ...)
```

---

## ステータス

**リリース済み: [v0.2.1](https://github.com/45online/roster/releases/tag/v0.2.1)** — 設計ドキュメントが計画したフェーズはすべて実装済み:4 つのエンドツーエンドモジュール、2 段階の Budget しきい値、Undercover Mode、Polling + Webhook の二重イベントソース、クロスプラットフォームバイナリ + マルチアーキ Docker、**マルチ LLM プロバイダ**(Claude / DeepSeek / Gemini / OpenAI / xAI / OpenAI 互換エンドポイントすべて)。

**次のフェーズはドッグフード**。機能の積み上げはここで一区切り。これから 1 週間、実リポジトリで動かして、どの仮定が崩れるか(プロンプトのチューニング / モジュールの境界 / UX の粗)を観察します。リリース履歴は [CHANGELOG.md](CHANGELOG.md)。

| フェーズ | 状態 |
|---|---|
| 0. claude-code-go から fork、リブランド、業務レイアウト | ✅ |
| 1. CLI 文言 + 起動 logo | ✅ |
| 2. Module A: Issue → Jira(`sync-issue`) | ✅ |
| 2.x. Poller + アンチループ + `takeover` | ✅ |
| 2.y. Claude API スマートフィールド抽出 | ✅ |
| 2.z₁. JSONL 監査 + `.roster/config.yml` + `roster init` | ✅ |
| 2.z₂. `roster login` 認証情報管理 | ✅ |
| 3. Module B: PR AI Review(`review-pr` + takeover) | ✅ |
| 4. Module C: Issue close → Confluence(`archive-issue` + takeover) | ✅ |
| 5. Module D: アラート集約 → Slack(`aggregate-alert`、AI なし) | ✅ |
| 6.a `roster status` + `roster logs` 監視パネル | ✅ |
| 6.b Budget トラッキング(token + USD、月次集計) | ✅ |
| 6.c Budget しきい値 — stop モード | ✅ |
| 6.c+ Budget しきい値 — downgrade モード(AI を切るが daemon は継続) | ✅ |
| 6.d Webhook モード + GitHub HMAC 検証 | ✅ |
| 7. Undercover Mode(身元隔離 + シークレット redact) | ✅ |
| 8. コンテナ化 + CI(Dockerfile + Actions + GHCR) | ✅ |
| 9. マルチ LLM プロバイダ(Anthropic / OpenAI 互換) | ✅ v0.2.0 |
| 10. Helm chart(K8s デプロイ) | ✅ v0.2.1 |
| 11. Slack スラッシュコマンド(`/roster …`) | ✅ v0.2.1 |

---

## 設計の核

- **GitHub = 唯一の真実の源**。開発者は GitHub にしか Issue を立てず、コードを push せず、PR にコメントしない。
- **Jira / Confluence = 自動ミラー**。AI が同期し、マネジメントは閲覧専用。
- **Slack = リアルタイムの脈拍**。AI がアラート / 通知 / レビュー要約をプッシュ。
- **AI スタッフ = 仮想の人間アカウント**。`[bot]` タグなし — 名前、アバター付きの普通の GitHub collaborator のように見える。ただし全アクションは Roster が代理で実行。

例え:AI コーディングアシスタントがコードに対するもの、それと同じく **Roster がプロジェクト管理に対するもの**。

---

## 4 つのモジュール

| モジュール | やること |
|---|---|
| **A. Issue → Jira** | 新しい GitHub Issue → AI がフィールドを抽出 → Jira チケットを自動作成 |
| **B. PR AI Review** | 新しい PR → AI が静的解析 + 行コメント(オプションでローカル checkout してテスト) |
| **C. Issue close → Confluence** | Issue クローズ → AI がスレッドを要約 → Confluence のドラフト(人間が公開) |
| **D. アラート集約** | 外部アラート → AI が直近の commit/deploy を関連付け → Slack のチャンネル(ログボード型) |

---

## クイックスタート

### 前提
- 仮想スタッフ用の GitHub アカウント(PAT 付き)
- LLM API キー — いずれか:Anthropic Claude / DeepSeek(最安) / Google Gemini / OpenAI / xAI Grok / Together / Groq など
- (オプション)Jira / Confluence / Slack の API トークン

> **LLM プロバイダ**:OpenAI Chat Completions 互換のエンドポイントなら何でも動きます(DeepSeek / Gemini OpenAI 互換 / OpenAI / xAI / Together / Groq)。`roster login llm` で設定。簡単な構造化抽出に対しては、現状 DeepSeek が最安(Claude Haiku の約 1/10、Claude Sonnet の約 1/30)。

### インストール(いずれかを選択)

**A. ソースから**(Go 1.26+)
```bash
git clone https://github.com/45online/roster.git
cd roster
make build
./bin/roster --help
```

**B. Docker**(依存ゼロ)
```bash
docker pull ghcr.io/45online/roster:v0.2.1   # または :latest
docker run --rm ghcr.io/45online/roster:v0.2.1 --version
# → roster v0.2.1
```

完全な実行(`~/.roster` を永続化マウントして credentials + audit + cursor を保持。`-w /work` でマウントしたリポジトリで動作):
```bash
docker run --rm \
  -v "$HOME/.roster:/home/roster/.roster" \
  -v "$PWD:/work" -w /work \
  -e ROSTER_GITHUB_TOKEN -e ROSTER_JIRA_TOKEN -e ROSTER_JIRA_URL -e ROSTER_JIRA_EMAIL -e ROSTER_LLM_API_KEY \
  ghcr.io/45online/roster:v0.2.1 takeover --repo owner/name
```

マルチアーキ(linux/amd64、linux/arm64)、最終イメージ ~40 MB。

**C. Homebrew**(リリース後)
```bash
brew install 45online/tap/roster
```

**D. Helm chart(Kubernetes)**

```bash
# 認証情報 Secret を作成(本番推奨;ラボでは values にインラインも可)
kubectl create secret generic roster-creds \
  --from-literal=ROSTER_GITHUB_TOKEN=ghp_xxx \
  --from-literal=ROSTER_LLM_API_KEY=sk-...

# インストール
helm install roster ./charts/roster \
  --set repo=owner/name \
  --set credentials.existingSecret=roster-creds
```

詳細ガイド:[charts/roster/README.md](charts/roster/README.md) — webhook + Ingress + TLS、永続化、本番ノブまで。**シングル Pod 前提**(cursor は同時並行不可)。N 個のリポジトリを管理するには N 個の release をインストール。

---

## 設定

2 つの方法のいずれか。env vars(CI 向け)または `roster login`(対話、`~/.roster/credentials.json` mode 0600 に永続化)。

```bash
# A: 対話 login(ローカル / VPS 推奨)
roster login github     # PAT を貼り付け
roster login jira       # URL / email / token
roster login llm        # provider / base_url / model / api_key
roster login status     # 現状を表示

# B: env vars(CI / Docker)
export ROSTER_GITHUB_TOKEN=ghp_xxx
export ROSTER_JIRA_URL=https://yourorg.atlassian.net
export ROSTER_JIRA_EMAIL=you@example.com
export ROSTER_JIRA_TOKEN=xxxx

# LLM(provider/model の任意組合せ)
export ROSTER_LLM_PROVIDER=openai-compatible
export ROSTER_LLM_BASE_URL=https://api.deepseek.com
export ROSTER_LLM_MODEL=deepseek-chat
export ROSTER_LLM_API_KEY=sk-...
# またはレガシーな Anthropic 専用パス:
# export ANTHROPIC_API_KEY=sk-ant-xxx
```

### LLM プロバイダの選択肢

| プロバイダ | base_url | 推奨 model | 価格(in/out per Mtok) |
|---|---|---|---|
| Anthropic Claude | (デフォルト) | `claude-haiku-4-5-20251001` | $1 / $5 |
| Anthropic Claude | (デフォルト) | `claude-sonnet-4-6-20250514` | $3 / $15 |
| **DeepSeek**(安価) | `https://api.deepseek.com` | `deepseek-chat` | $0.27 / $1.10 |
| Google Gemini | `https://generativelanguage.googleapis.com/v1beta/openai/` | `gemini-2.5-flash` | $0.075 / $0.30 |
| OpenAI | `https://api.openai.com/v1` | `gpt-4o-mini` | $0.15 / $0.60 |
| xAI Grok | `https://api.x.ai/v1` | `grok-3` | $2 / $10 |

`roster status` は実モデルレートで月次累計コストを集計します(価格表は内蔵)。

---

## モジュール使い方

### A. Issue → Jira(ワンショット)
```bash
./bin/roster sync-issue --repo owner/name --issue 42 --jira-project ABC
# → ✓ Created ABC-123
#   GH issue #42 にコメント:📋 Tracking in Jira: ABC-123
```

### B. バックグラウンド daemon(issues.opened / PR opened/sync / issue closed を自動 dispatch)
```bash
cd <your-repo>
roster init                          # .roster/config.yml を生成
# jira_project を編集、他のモジュールを有効化…
roster takeover --repo owner/name
# → ✓ Loaded config from .roster/config.yml
#   ✓ Authenticated as @virtual-employee (anti-loop filter armed)
#   ✓ AI extractor enabled (provider=openai-compatible · model=deepseek-chat)
#   [poller] starting (interval=30s, ...)
#   [mod-a] dispatching: ...#43 → ✓ ABC-124
# Ctrl+C で停止。cursor + audit ログは ~/.roster/ に永続化。
```

### C. PR AI Review
```bash
./bin/roster review-pr --repo owner/name --pr 42
# → ✓ Review submitted (comment, 2 inline comments)
# デフォルト:全 verdict は COMMENT に降格(Approve / Block は人間がクリック)。
# --can-approve / --can-request-changes でゲート解除。
```

`.roster/config.yml`:
```yaml
modules:
  pr_review:
    enabled: true
    skip_paths: ["docs/", "*.md"]
    max_diff_bytes: 65536
    can_approve: false
    can_request_changes: false
```

### D. Issue close → Confluence ドラフト
```bash
./bin/roster archive-issue \
  --repo owner/name --issue 42 \
  --space-id 12345 \
  --slack-channel "#archives"   # オプション
# → ✓ Draft created (id=987654)
# ドラフトは owner にしか見えず、人間が Publish するまで公開されない。
```

### E. アラート集約 → Slack
Claude 不要(テンプレート、決定論的、$0)。
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

**設計哲学**:Module D は「ログボード」役 — オンコールが自分で判断するために最近のアクティビティを列挙する。因果帰属しない、@-メンションしない、チケットを作らない。誤った帰属は帰属しないことよりタチが悪い。

---

## 運用

### `roster status`
1 画面で credentials / 管理リポジトリ / 直近 24h のモジュール別アクティビティ / 最新エラー / **月次 Budget** をまとめて表示。

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
JSONL audit ログを `--module` / `--status` / `--since 30m` / `-f` のフィルタ付きで tail。
両コマンドとも `--json` で機械可読出力。

### Budget しきい値

`.roster/config.yml`:
```yaml
budget:
  monthly_usd: 50
  on_exceed: downgrade   # 'stop' | 'downgrade'
```

- `stop`(デフォルト):既に上限超なら起動拒否。最初の超過イベントで daemon を停止。
- `downgrade`:daemon は走らせ続けるが AI 呼び出しを無効化。Module A はメカニカルなラベルマッピングにフォールバック(Jira チケットは作る)、Module B / C はスキップ、Module D は影響なし。月をまたいで MTD が下がれば自動で復帰。

### Webhook モード(polling の代替)

`.roster/config.yml`:
```yaml
webhook:
  enabled: true
  listen: ":8080"
  path: /webhook/github
  secret: ""                 # または export ROSTER_WEBHOOK_SECRET=...
```

GitHub のリポジトリ Settings → Webhooks:
- Payload URL: `<public URL>/webhook/github`
- Content type: `application/json`
- Secret: `ROSTER_WEBHOOK_SECRET` と同じ
- Issues + Pull requests にチェック

特徴:HMAC-SHA256 検証(constant-time)、event mapping、`ping` → 200、`/healthz`、anti-loop、5MB ボディ上限。**polling と排他** — webhook UUID と events-API ID は別の名前空間で重複排除できないため。

### Slack スラッシュコマンド(オプション)

webhook モードが有効な時、チームが Slack から直接 Roster をトリガーできる:

```
/roster status                        Roster の状態を表示
/roster sync-issue owner/name#42      Module A をトリガー
/roster review-pr  owner/name#42      Module B をトリガー
/roster archive-issue owner/name#42   Module C をトリガー
/roster help                          ヘルプ
```

`.roster/config.yml`:
```yaml
slack:
  enabled: true
  path: /slack/command
  signing_secret: ""    # または ROSTER_SLACK_SIGNING_SECRET=...
```

Slack app の設定([api.slack.com/apps](https://api.slack.com/apps) → Create New App):
1. Slash Commands → Create New Command:
   - Command: `/roster`
   - Request URL: `https://<your-public-host>/slack/command`
2. Basic Information → "Signing Secret" をコピー → `signing_secret` または env に。
3. Install to Workspace。

特徴:HMAC-v0 署名検証(constant-time + 5 分のリプレイウィンドウ)。`status` は同期。`sync-issue` / `review-pr` / `archive-issue` は即時 ack(`:hourglass_flowing_sand: queued`)してバックグラウンド goroutine で実行(Slack の 3 秒応答制限のため)。結果は GitHub / Jira / Confluence に直接出るので、Slack に戻ってこない。Roster Pod は単一リポジトリ管理のため、Slack コマンドのリポジトリと一致必須。

### Undercover Mode(常時 ON)

仮想スタッフは外から見て人間と区別できない。2 層の保護:

1. **プロンプト層** — 各 AI モジュールの system prompt に invariant を追加:「As an AI」を言わない、ベンダー名を出さない、モデル ID を出さない、ディスクレーマーなし。
2. **出力層** — GitHub / Jira / Confluence / Slack に送る前に regex scrubber を通す:
   - シークレット(`sk-ant-*`、`ghp_*`、`xox[abp]-*`) → 明示的なマーカー
   - ベンダー / モデル ID → 静かに削除
   - AI の自己暴露フレーズ → 静かに削除またはリライト

PR レビューボディには `🤖 AI Review (model)` ヘッダはない代わりに、控えめな `_automated review_` フッタを残す。Module D(アラート集約)も redactor を通す — commit メッセージや PR タイトルをそのまま引用するため。

---

## 出典

Roster は [claude-code-go](https://github.com/tunsuy/claude-code-go)(MIT)の上に構築されています。上流の `internal/api`、`internal/tools`、`internal/engine`、`internal/coordinator` などを直接再利用し、`adapters/`、`modules/`、`poller/`、`webhookreceiver/`、`audit/`、`budget/`、`creds/`、`projcfg/`、`undercover/` などのパッケージを上に追加しています。

fork の関係は [NOTICE](NOTICE) を参照。

---

## ライセンス

[MIT](LICENSE) — 完全な attribution は [LICENSE](LICENSE) と [LICENSE.upstream](LICENSE.upstream) を参照。
