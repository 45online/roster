# Roster

> 给团队加一个 AI 员工。

Roster 是一个本地 / VPS 长驻的 CLI 工具,它让 AI 以"虚拟真人员工"的身份接管你的研发管理流:GitHub 上发生的事,自动同步到 Jira / Confluence / Slack,无需人工搬运。

开发者只在 GitHub 工作,管理留在后台,AI 负责连接。

```
GitHub  ←→  Roster (AI 员工)  ←→  Jira / Confluence / Slack
                  │
                  └── powered by Claude
```

---

## 状态

**已发布:[v0.1.4](https://github.com/45online/roster/releases/tag/v0.1.4)** —— 4 个业务模块全部端到端可用,有 budget 跟踪 + 阈值告警,有跨平台二进制 + 多架构 docker 镜像。还在 alpha 阶段,等待真实场景的反馈。

| 阶段 | 状态 |
|---|---|
| 0. Fork claude-code-go 改名 + 业务目录 | ✅ |
| 1. CLI 文案 + 启动 logo | ✅ |
| 2. Module A: Issue → Jira(`sync-issue`) | ✅ |
| 2.x. Poller + 防循环 + `takeover` | ✅ |
| 2.y. Claude API 智能字段抽取 | ✅ |
| 2.z₁. JSONL 审计 + `.roster/config.yml` + `roster init` | ✅ |
| 2.z₂. `roster login` 凭证管理 | ✅ |
| 3. Module B: PR AI Review(`review-pr` + takeover) | ✅ |
| 4. Module C: Issue close → Confluence(`archive-issue` + takeover) | ✅ |
| 5. Module D: 告警聚合 → Slack(`aggregate-alert`,纯模板无 AI) | ✅ |
| 6.a `roster status` + `roster logs` 观察面板 | ✅ |
| 6.b Budget 跟踪(token + USD,月度汇总) | ✅ |
| 6.c Budget 阈值 stop 模式 | ✅ |
| 8. 容器化 + CI(Dockerfile + Actions + GHCR) | ✅ |
| 7. Undercover Mode(身份隔离 + 秘密 redact) | ✅ |
| 6.c+ Budget downgrade 模式(关 AI 不停 daemon) | ✅ |
| 6.d Webhook 模式 + GitHub HMAC 校验 | ⏳ |

---

## 核心理念

- **GitHub = 唯一真理来源**:开发者只在 GitHub 提 Issue / 写代码 / 评 PR
- **Jira / Confluence = 自动镜像**:由 AI 同步,管理层只读
- **Slack = 实时感知**:AI 推送告警 / 通知 / Review 摘要
- **AI 员工 = 虚拟真人账户**:不是 `[bot]` 标记,是有名字、有头像、像真人一样的 GitHub collaborator,只是所有操作由 Roster 代理

类比:像 AI 编码助手之于代码,**Roster 之于研发管理**。

---

## 四个核心模块

| 模块 | 职责 |
|---|---|
| **A. Issue → Jira** | GitHub 新 Issue → AI 抽取字段 → Jira 自动建票 |
| **B. PR AI Review** | 新 PR → AI 静态分析 + 行级评论(可选本地 checkout 跑测试) |
| **C. Issue close → Confluence** | Issue 关闭 → AI 汇总 → Confluence 草稿(真人发布) |
| **D. 告警聚合** | 外部告警 → AI 关联近期 commit/deploy → Slack 公共频道(类日志平台) |

---

## 快速开始

### 前置要求
- 一个虚拟员工身份的 GitHub 账户(配 PAT)
- Claude API key
- (可选)Jira / Confluence / Slack 的 API token

### 安装(三选一)

**A. 从源码**(需要 Go 1.26+)
```bash
git clone https://github.com/45online/roster.git
cd roster
make build
./bin/roster --help
```

**B. Docker**(零依赖)

```bash
docker pull ghcr.io/45online/roster:v0.1.4   # 或 :latest
docker run --rm ghcr.io/45online/roster:v0.1.4 --version
# → roster v0.1.4
```

完整运行(挂载 `~/.roster` 持久化凭证 + 审计 + cursor;`-w /work` 让命令在挂载的 repo 中执行):

```bash
docker run --rm \
  -v "$HOME/.roster:/home/roster/.roster" \
  -v "$PWD:/work" -w /work \
  -e ROSTER_GITHUB_TOKEN -e ROSTER_JIRA_TOKEN -e ROSTER_JIRA_URL -e ROSTER_JIRA_EMAIL -e ANTHROPIC_API_KEY \
  ghcr.io/45online/roster:v0.1.4 takeover --repo owner/name
```

支持 linux/amd64 和 linux/arm64,镜像 ~40 MB。

**C. Homebrew**(release 后可用)
```bash
brew install 45online/tap/roster
```

### 试用 Module A(已可用)

凭证两种方式任选其一(env vars 优先,文件 fallback):

```bash
# 方式 A:一次性 login(推荐)
roster login github          # 提示输入 PAT,保存到 ~/.roster/credentials.json (0600)
roster login jira            # URL / email / token
roster login claude          # 可选,启用 AI 字段抽取
roster login status          # 看哪些已配置

# 方式 B:环境变量(临时 / CI 友好)
export ROSTER_GITHUB_TOKEN=ghp_xxx
export ROSTER_JIRA_URL=https://yourorg.atlassian.net
export ROSTER_JIRA_EMAIL=you@example.com
export ROSTER_JIRA_TOKEN=xxxx
export ANTHROPIC_API_KEY=sk-ant-xxx       # 可选
```

**A. 一次性手动同步**
```bash
./bin/roster sync-issue --repo owner/name --issue 42 --jira-project ABC
# → ✓ Created ABC-123
#   同时 GH issue #42 出现评论:📋 Tracking in Jira: **ABC-123**
```

**B. 后台 daemon(自动监听 issues.opened)**

推荐先 `roster init` 在仓库内生成项目配置:
```bash
cd <your-repo>
roster init                          # 生成 .roster/config.yml(改 jira_project)
roster takeover --repo owner/name    # 自动从 .roster/config.yml 读 jira_project / mappings
# → ✓ Loaded config from .roster/config.yml
#   ✓ Authenticated as @virtual-employee (anti-loop filter armed)
#   [poller] owner/name: starting (interval=30s, ...)
#   [mod-a] dispatching: owner/name#43 "fix login bug" (by @real-user)
#   [mod-a] ✓ ABC-124
# Ctrl+C 停止
```

每次同步都会写一行到 `~/.roster/audit/<owner>_<repo>.jsonl`(JSON-per-line,可 `tail -f`)。Cursor 持久化到 `~/.roster/cursors/<owner>_<repo>.json`,重启不会重处理。

**D. Module B:PR AI Review**

需要 Claude 凭证(`roster login claude` 或 `ANTHROPIC_API_KEY`)。

手动一次性:
```bash
./bin/roster review-pr --repo owner/name --pr 42
# → ✓ Review submitted (comment, 2 inline comments)
# 默认所有 verdict 都被降级为 COMMENT(真人才能 Approve / Block)
# --can-approve / --can-request-changes 解锁
```

后台 daemon(配 `.roster/config.yml`):
```yaml
modules:
  pr_review:
    enabled: true
    skip_paths:               # 全部命中则跳过整个 PR
      - "docs/"
      - "*.md"
    max_diff_bytes: 65536
    can_approve: false
    can_request_changes: false
```

`roster takeover` 会在 PR opened / synchronize 事件触发 review,draft PR 自动跳过。

**E. Module C:Issue close → Confluence 草稿**

需要 Atlassian 凭证(`roster login jira` —— Confluence 复用同一组)+ Claude 凭证。Slack 通知是可选的。

手动一次性:
```bash
./bin/roster archive-issue \
  --repo owner/name --issue 42 \
  --space-id 12345 \
  --slack-channel "#archives"   # 可选
# → ✓ Draft created (id=987654)
#   https://yourorg.atlassian.net/wiki/...
# 草稿仅 owner 可见,真人在 Confluence 点 Publish 才会公开
```

后台 daemon(`.roster/config.yml`):
```yaml
modules:
  issue_to_confluence:
    enabled: true
    space_id: "12345"
    completed_label: completed   # 只归档带这个 label 的 closed issue
    slack_channel: "#archives"   # 可选
```

`roster takeover` 会在 issue closed 事件触发归档。如果没有 `completed` label,会被 skip,审计里有记录但不创建草稿。

**F. Module D:告警聚合 → Slack**

需要 GitHub PAT(只读够用)+ `roster login slack`。**不需要 Claude**(纯模板化,零成本)。

由外部告警系统(CloudWatch / Datadog / PagerDuty)调用:
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

输出到 `#oncall` 的消息是这样:

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

**设计哲学**:Module D 是"日志看板"角色 —— 列举近期活动让 oncall 自己判断,不归因、不 at 人、不建票。错误归因比没有归因更糟。

### 观察面板

`roster status` —— 一屏看全:凭证状态 / 接管的项目 / 最近 24h 各模块调用统计 / 最新错误。

```
Roster status — 2026-05-03T14:23:00Z
Base dir: /Users/me/.roster

Credentials:
  github  ✓ configured
  jira    ✓ configured
  slack   ✗ not set
  claude  ✓ configured

Projects (1, last 24h):

  foo/bar
    cursor       last polled 30m ago, event_id=…
    audit        3 events: 2 success, 0 partial, 1 error, 0 skipped
    by module    issue_to_jira=1, pr_review=2
    last event   15m ago
    last error   15m ago — diff too large
```

`roster logs <repo>` —— 看单项目的审计流(可加 `--module` / `--status` / `--since 30m` / `-f` follow):

```
$ roster logs foo/bar --status error -f
2026-05-03T14:25Z [pr_review/error] foo/bar#3 by @carol [50ms]  ! diff too large
2026-05-03T14:30Z [issue_to_jira/error] foo/bar#7 by @alice [820ms]  ! create jira issue: 401 Unauthorized
```

`--json` 标志在两个命令上都可用,方便接外部监控。

每次 Claude 调用的 token 数 / 美元成本会写入 audit 行,`roster status` 自动汇总当月支出:

```
Projects (1, last 24h):

  foo/bar
    audit        4 events: 4 success, 0 partial, 0 error, 0 skipped
    by module    alert_aggregation=1, issue_to_jira=1, pr_review=2
    last event   just now
    budget MTD   $0.11 over 3 AI calls (issue_to_jira=$0.00, pr_review=$0.11) · 29k in / 1k out
```

价格表内置 Claude 4.x 系列(Opus / Sonnet / Haiku),cache write/read 按 1.25× / 0.10× input 自动派生。Module D 不调 AI,自然不产生成本。

### 预算阈值(防 runaway)

`.roster/config.yml` 设 `budget.monthly_usd` 后,`roster takeover` 会:

1. **启动时** 显示 MTD vs cap,如果当前已经超限就拒绝启动:
   ```
   ✓ Budget MTD: $0.43 / $50.00 cap (on_exceed=stop)
   ```
2. **每次事件分发前** 检查当月累计成本(30 秒 TTL 缓存,免每次重读 audit)
3. **超限时** 按 `on_exceed` 处理:
   - `stop`(默认):打印 `⛔ budget exceeded` 然后退出 daemon
   - `downgrade`:**保持 daemon 运行,但禁用所有 AI 调用** ——
     - Module A:跳过 Claude 字段抽取,走 label 机械映射(仍能建 Jira 票)
     - Module B(PR review):整个 skip
     - Module C(Issue 归档):整个 skip
     - Module D(告警聚合):**不受影响**(本来就不调 AI)

   downgrade 状态会随每次 budget check 自动恢复 —— 例如月份滚动后 MTD 归零,模块自动恢复。

读 audit 失败时**fail open**(不阻塞业务),避免成为新的故障点。

### Undercover Mode(默认开启)

虚拟员工对外永远是真人。两层保护:

1. **Prompt 层**:每个 AI 模块的 system prompt 末尾附加一条 invariant —— 不许说"As an AI"、不许提 vendor、不许暴露 model name、不许加任何 disclaimer。
2. **输出层**:在送给 GitHub / Jira / Confluence / Slack 之前,文本过一遍 redactor:
   - **秘密**:`sk-ant-*` / `ghp_*` / `xoxp-*` → `[redacted-*-key]`(显式标记,审计可见)
   - **vendor / model identifier**:`Anthropic` / `claude-sonnet-4-6-...` → 静默移除(让 cover 不破)
   - **AI 自我暴露套话**:`As an AI`, `As a large language model`, `Generated by an AI` → 静默移除或改写

PR review body 也去掉了原本的 `🤖 AI Review (claude-sonnet-...)` 头,改成结尾一行 `_automated review_` —— 真人 reviewer 仍能识别,但不暴露身份/模型。

Module D(告警聚合)虽然不调 AI,也走 redactor,因为消息里引用了 commit message 与 PR 标题,这些可能被恶意写入 token 或残留信息。

**C. 启用 Claude 智能字段抽取(可选)**

设置 `ANTHROPIC_API_KEY` 后,Module A 会让 Claude 读 issue body,生成更精炼的 summary、推断 issue type / priority / component:
```bash
export ANTHROPIC_API_KEY=sk-ant-xxx
./bin/roster sync-issue --repo owner/name --issue 42 --jira-project ABC
# → ✓ Claude extractor enabled
#   ✓ Created ABC-125 (AI-extracted)
```

未设置时自动 fallback 到 label 机械映射;Claude 调用失败时也会安静地降级。

### 计划中的使用流程(daemon 模式,尚未实现)
```bash
# 一次性凭证
roster login github          # 粘贴 PAT
roster login claude          # Claude API key
roster login jira            # 可选
roster login slack           # 可选

# 接管一个 GitHub repo
cd <your-repo>
roster init                  # 生成 .roster/config.yml
roster takeover              # 验证并启动监听

# 查看 / 管理
roster list                  # 已接管项目
roster logs <project> -f     # 实时审计日志
roster pause <project>       # kill switch
```

详细命令规划见 [设计文档 §9](docs/DESIGN.md#9-cli-命令规划)。

---

## 文档

- 📐 **[设计文档(完整方案)](docs/DESIGN.md)** —— 架构、模块、配置、运行流程、MVP 路径
- 📜 [LICENSE](LICENSE) —— MIT
- 📜 [LICENSE.upstream](LICENSE.upstream) —— claude-code-go 原始许可
- 📜 [NOTICE](NOTICE) —— Fork 关系与设计参考说明
- 🛡️ [SECURITY.md](SECURITY.md) —— 安全漏洞报告流程

---

## 来源

Roster 基于 [claude-code-go](https://github.com/tunsuy/claude-code-go)(MIT)构建,直接复用了其完整的 LLM API 客户端、本地工具系统(fileops / shell / web / interact)、执行引擎、协调器、9 步权限管线等基础设施。Roster 在此之上添加业务模块和外部 SaaS 适配器。

详见 [NOTICE](NOTICE)。

---

## License

[MIT](LICENSE) — 详见 [LICENSE](LICENSE) 与 [LICENSE.upstream](LICENSE.upstream)。
