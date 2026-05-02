# Roster

> **AI 员工花名册**
> 给团队加一个虚拟同事,让 AI 顶替一个真人席位,接管研发管理流。
> 开发者只在 GitHub 工作,Jira / Confluence / Slack 由 AI 同步。

**当前状态**:阶段 0 完成 —— 已 fork 上游骨架并改名,二进制可编译运行。
模块代码尚未实现,正在进入阶段 1(MVP)。

---

## 1. 一句话产品定义

Roster 是一个本地 / VPS 长驻的 CLI 工具。
你给它一个 GitHub 账户(看起来是真人的虚拟员工)、几把外部系统的 token,
它就以这个"员工"的身份接管一个或多个项目的研发管理流:
建 Jira 票、Review PR、归档 Confluence、推送告警 —— 全自动。

类比:像 AI 编码助手之于代码,Roster 之于研发管理。

---

## 2. 核心愿景

**"开发者留在 GitHub,管理留在后台,AI 负责连接。"**

GitHub 是唯一的真理来源,其他系统都是它的自动镜像。
团队里多了一名"AI 同事",ta 没有名字标签 `[bot]`,行为和一个尽职的初级工程师无异 ——
区别只是 ta 不睡觉、不抱怨、月薪是 Claude API 账单。

---

## 3. 角色定义

| 角色 | 实体 | 职责 |
|---|---|---|
| **GitHub** | 真理来源 | 需求(Issue)、代码(PR)、文档(`/docs`) |
| **AI 员工**(Roster 管理) | 虚拟真人账户 | 看起来是个普通同事,实际由 AI 操作 |
| **Roster CLI** | 本地 / VPS 进程 | AI 员工的"大脑+手脚",轮询事件、调 LLM、调外部 API、操作本地仓库 |
| **Jira / Confluence** | 镜像档案 | 给管理层看,无人手动录入 |
| **Slack** | 实时感知 | 接收 AI 的反馈、告警、通知 |

---

## 4. AI 员工的两类能力

Roster 跑起来的 AI 员工,具备**远程**和**本地**两套能力:

### 4.1 远程能力(SaaS 操作)
- **GitHub**:读 Issue / PR / commit、写评论、打 Label、Approve、merge
- **Jira**:建票、流转状态、写评论
- **Confluence**:建草稿、发布页面
- **Slack**:发频道消息、收 slash command

### 4.2 本地能力(类 AI 编码助手)
通过 fork 来的 `internal/tools/` 直接获得:
- 读写文件、跑命令、执行测试
- 创建分支、commit、push
- 跑 lint / build / typecheck
- 让 AI 真正"在仓库里干活"而不只是"在 PR 上评论"

---

## 5. 多项目模型(单实例多租户)

```
~/.roster/
├── credentials.enc        # 全局 token(GitHub/Jira/Slack/Claude)
├── projects.json          # 已接管项目索引
└── audit/                 # 审计日志(按项目分目录)

<repo>/.roster/config.yml  # 每个项目自己的配置
```

- 全局凭证、虚拟员工身份集中放在 `~/.roster/`
- 业务规则、模块开关、路由参数下放到各 repo 的 `.roster/config.yml`
- 一个 Roster 进程同时接管 N 个项目,共享 token / budget meter / connection pool

---

## 6. 功能模块

### Module A:Issue → Jira(单向同步)
- **触发**:GitHub `issues.opened` 事件(只监听创建,不监听 edit)
- **AI 任务**:抽取 title / priority / module / labels
- **动作**:在 Jira 建票 → 在 GH Issue 评论 Jira 链接
- **特性**:无状态、无映射表、天然幂等(每个 Issue 只触发一次)

### Module B:PR AI Review(基于代码本身)
- **触发**:PR opened / synchronize
- **AI 任务**:静态分析、潜在 bug、安全问题、风格规范
- **不做**:对照设计文档校验业务逻辑(文档不可信)
- **动作**:行级评论 + Approve / Request Changes
- **本地能力(可选)**:checkout PR 分支、跑测试、修小问题直接 push

### Module C:Issue close → Confluence(草稿模式)
- **触发**:Issue closed 且带 `completed` label
- **AI 任务**:汇总 Issue 描述 + 评论 + 关联 PR → 生成文档
- **动作**:在 Confluence 建**草稿**(不直接发布) + Slack 通知 owner 审核
- **真人确认**:点链接发布

### Module D:告警聚合(类日志平台)
- **触发**:外部告警(CloudWatch / Datadog webhook 或定期 poll)
- **AI 任务**:拉过去 1h 的相关 commit + deploy 事件,格式化
- **动作**:发到 Slack 公共频道,**不归因、不建票、不 at 人**

---

## 7. 权限模型

### 默认:全权限

新接入的项目默认给 AI 员工**完全权限**,等同一个真人同事。

### 可选:细粒度降级

```yaml
# .roster/config.yml(可选,不写就是全开)
permissions:
  github:
    pull_requests:
      review_approve: false   # 出过事故后临时关闭 Approve
      merge: false
  confluence:
    publish: false
```

### 预设角色快速切换

```bash
roster role set observer      # 只读,出 bug 时一键降级
roster role set reviewer      # 只评论不 approve
roster role set full          # 默认值,完全权限
```

---

## 8. 运行流程

### 8.1 接管前

```
1. GitHub 创建/打开 repo
2. 把虚拟员工账户加为 collaborator(write 权限)
3. 写第一个 Issue:#1 "项目目标 / PRD"(可选但推荐)
4. cd <repo> && roster init             # 生成 .roster/config.yml
5. 编辑 .roster/config.yml 填 Jira project key / Slack channel
6. roster takeover                      # 验证连通 + 注册到 daemon
```

### 8.2 接管后(AI 持续在做的事)

```
[每 30s poll GitHub events]
  │
  ├─ actor 是 AI 自己? → drop(防循环)
  ├─ Module 启用? Budget 够? → 继续
  │
  ├─→ 新 Issue            → Module A
  ├─→ 新 PR/push          → Module B(可选 checkout 跑测试)
  ├─→ Issue close + label → Module C(草稿 + Slack 通知)
  └─→ 外部告警             → Module D(Slack 聚合)
  │
  └─ 写审计日志(JSONL)
```

---

## 9. CLI 命令(规划)

```bash
# 全局凭证
roster login github           # 粘贴 PAT
roster login jira             # API token + 域名
roster login slack            # OAuth token
roster login claude           # Claude API key

# 项目管理
roster init                   # 生成 .roster/config.yml 模板
roster takeover               # 验证 + 启动监听
roster list                   # 已接管项目 + 状态 + budget
roster status <name>
roster pause <name>           # kill switch
roster logs <name> -f

# 模式切换
roster dry-run <name> on
roster role set <name> <role>

# 守护进程
roster run                    # 前台运行
roster daemon start           # 后台守护
```

---

## 10. 架构图

```
┌──────────────────── Roster CLI ─────────────────────┐
│                                                      │
│  Command Layer (login/init/takeover/run/list/pause)  │
│  ────────────────────────────────────────────────    │
│  Config Layer                                        │
│   ~/.roster/credentials.enc  (全局 token)             │
│   ~/.roster/projects.json    (项目索引)               │
│   <repo>/.roster/config.yml  (项目配置)               │
│  ────────────────────────────────────────────────    │
│  Event Source                                        │
│   ▸ GitHub Poller(默认,每 30s)                       │
│   ▸ Webhook receiver(可选,需 tunnel)                 │
│   ▸ External alerts(CloudWatch/Datadog)             │
│  ────────────────────────────────────────────────    │
│  Dispatcher (复用 internal/coordinator)               │
│   ① actor filter(防循环)                            │
│   ② module enabled?                                  │
│   ③ permission check                                 │
│   ④ budget check                                     │
│   ⑤ dry-run check                                    │
│   ⑥ kill-switch check                                │
│  ────────────────────────────────────────────────    │
│  ┌────────┬────────┬────────┬────────┐               │
│  │ Mod A  │ Mod B  │ Mod C  │ Mod D  │               │
│  │Issue→J │PR Rev  │Close→C │Alert→S │               │
│  └────────┴────────┴────────┴────────┘               │
│   (复用 internal/engine 执行,注册为 agenttype)         │
│  ────────────────────────────────────────────────    │
│  Local Toolset(直接复用 internal/tools/)              │
│   fileops · shell · web · interact · memory · tasks  │
│  ────────────────────────────────────────────────    │
│  External Adapters(Roster 新写)                      │
│   GitHub | Jira | Confluence | Slack | Claude        │
│  ────────────────────────────────────────────────    │
│  Persistence(SQLite,单文件)                          │
│   audit_log · event_cursor · budget_meter · failed_q │
└──────────────────────────────────────────────────────┘
```

---

## 11. 项目配置示例

```yaml
# <repo>/.roster/config.yml
project_name: backend-api
identity: chen-xiaolu              # 虚拟员工账户

modules:
  issue_to_jira:
    enabled: true
    jira_project: BAPI
    priority_mapping:
      P0: Highest
      P1: High
      bug: Bug

  pr_review:
    enabled: true
    trigger:
      labels: []                   # 空 = 所有 PR
      skip_paths: ["docs/**", "*.md", "vendor/**"]
      max_diff_lines: 2000
    local_tools: true              # 是否允许 checkout + 跑测试

  issue_to_confluence:
    enabled: false

  alert_aggregation:
    enabled: true
    slack_channel: "#backend-alerts"
    lookback: 1h

permissions: {}                    # 不写则用 role 默认值

budget:
  monthly_usd: 50
  on_exceed: downgrade

dry_run: false
```

---

## 12. 横切关注点

### 12.1 防循环
- webhook / event 入口第一道过滤:`actor.login == 虚拟员工` → drop

### 12.2 Kill Switch(三层)
- repo Label `no-roster` → 该 repo 所有事件 drop
- 全局:`roster pause-all`
- Slack `/roster pause <project>`

### 12.3 审计日志
- SQLite + JSONL 双写
- 字段:`timestamp / project / module / actor / event / prompt_hash / model / output / actions / result`
- 保留 ≥ 90 天,debug 时能完整回放

### 12.4 Budget 控制
- 每项目独立月度预算
- 超限时按 `on_exceed`:`downgrade` / `stop`

### 12.5 失败重试
- 失败事件入 SQLite `failed_queue`
- 指数退避重试 5 次,最终失败 → Slack 告警

### 12.6 Dry-run
- 新项目接入第一周强烈建议开启
- AI 正常推理,但所有写操作仅记录到审计日志

---

## 13. 技术栈

| 组件 | 选型 | 理由 |
|---|---|---|
| **语言** | Go(1.26+) | 单二进制分发、无运行时、长驻 daemon 性能好 |
| **代码基础** | **fork 自 [claude-code-go](https://github.com/tunsuy/claude-code-go) (MIT)** | 已有完整 API/tools/engine/coordinator/permissions 骨架 |
| **设计模式** | 主动 agent 循环 / 后台维护 / Skills 抽象等 | clean-room 重新实现 |
| **持久化** | SQLite | 单文件、零运维 |
| **配置** | YAML | 注释友好 |
| **AI 模型** | Claude Opus 4.7 / Sonnet 4.6 | 长上下文、prompt caching 省成本 |
| **事件源** | GitHub Polling(默认)+ Webhook(可选) | 零网络要求即可启动 |
| **打包** | `goreleaser`(已配置) | 多平台二进制 + Homebrew tap |

---

## 14. MVP 路径

### ✅ 阶段 0:Fork 与改名(已完成)
- 从 claude-code-go 复制源码到工作目录
- module path 改为 `github.com/45online/roster`
- `cmd/claude` → `cmd/roster`
- Makefile / .goreleaser.yaml 更新
- 创建 Roster 业务目录:`internal/{modules,adapters,poller,identity}/`
- ✅ 二进制可编译:`go build ./cmd/roster`

### 阶段 1:CLI 文案 + 凭证管理
- 替换所有 "claude" CLI 输出文案为 Roster
- `roster login *` 全套(GitHub PAT / Jira / Slack / Claude)
- 复用 `internal/oauth` 的 store 加密机制
- 配置加载与校验(`~/.roster/credentials.enc` + `<repo>/.roster/config.yml`)

### 阶段 2:Module A 端到端(最简)
- `internal/adapters/github`:Events Polling + Issue API
- `internal/adapters/jira`:Create Issue API
- `internal/poller`:周期性事件抓取 + cursor 持久化
- 把 `issue_to_jira` 注册为一种 `agenttype`,prompt 用 Claude 抽取字段
- 复用 `internal/coordinator` 做 actor filter / permission check
- 审计日志写入 SQLite

### 阶段 3:Module B(本地能力)
- PR diff 拉取 + Claude review
- 行级评论(GitHub Reviews API)
- 可选:本地 checkout + 跑 lint(用 `internal/tools/shell`)

### 阶段 4:Module C
- Issue close 检测
- Confluence Adapter
- 草稿生成 + Slack 通知

### 阶段 5:Module D
- 外部告警接收
- Commit / deploy 事件聚合
- Slack 推送

### 阶段 6:打磨
- `roster status` Dashboard
- Budget 告警
- Webhook 模式(可选)

### 阶段 7+:补设计模式(按需)
- Undercover Mode(虚拟员工身份隔离)
- KAIROS 风格主动循环
- autoDream 风格审计自维护
- Skills 系统改造(把 module 抽象为 skill)

---

## 15. 开放问题

- [ ] 多 AI 员工身份切换:同一 Roster 是否支持挂多个虚拟员工?
- [ ] Module B 的本地能力安全边界:AI 直接 push 代码的兜底机制
- [ ] Polling 频率与 GitHub API rate limit 平衡
- [ ] 多机部署时的 leader election(避免重复处理)
- [ ] CLI 帮助文案、命令名、错误消息全面替换 "claude" → "roster"
- [ ] LICENSE 处理:是否在 README + NOTICE 中显式说明 fork 来源
- [ ] 是否保留 internal/tui(daemon 不用,但 `roster status` 可能想要)

---

## 16. 项目结构(实际)

Fork 自 roster,加上 Roster 业务层:

```
roster/
├── cmd/
│   ├── roster/                       # ← 主入口(改名自 cmd/claude)
│   └── docgen/                       # 文档自动生成器(继承)
├── internal/
│   ├── api/                          # 继承:LLM API 客户端(多 Provider)
│   ├── tools/                        # 继承:fileops/shell/web/interact/memory/tasks
│   ├── engine/                       # 继承:query loop / orchestration / budget
│   ├── coordinator/                  # 继承:多 agent 路由(将作为 Dispatcher)
│   ├── session/                      # 继承:会话存储
│   ├── state/                        # 继承:状态存储
│   ├── permissions/                  # 继承:9 步权限管线
│   ├── hooks/                        # 继承:Hook 系统
│   ├── msgqueue/                     # 继承:消息队列(Roster failed_queue 用)
│   ├── config/                       # 继承:配置加载
│   ├── bootstrap/                    # 继承:启动装配
│   ├── agentctx/                     # 继承:agent 上下文
│   ├── agenttype/                    # 继承:agent 类型注册(将注册 module)
│   ├── commands/                     # 继承:CLI 命令注册
│   ├── mcp/                          # 继承:MCP 客户端
│   ├── compact/                      # 继承:上下文压缩(后期用)
│   ├── memdir/                       # 继承:记忆目录
│   ├── oauth/                        # 继承:本地凭证加密(我们用 PAT 但加密机制可复用)
│   ├── tui/                          # 继承:TUI(可能用于 roster status)
│   ├── plugin/                       # 继承:插件骨架
│   │
│   ├── modules/                      # 🆕 Roster 业务模块
│   │   ├── issue_to_jira/
│   │   ├── pr_review/
│   │   ├── issue_to_confluence/
│   │   └── alert_aggregator/
│   ├── adapters/                     # 🆕 外部 SaaS 客户端
│   │   ├── github/
│   │   ├── jira/
│   │   ├── confluence/
│   │   └── slack/
│   ├── poller/                       # 🆕 GitHub events 轮询
│   └── identity/                     # 🆕 虚拟员工身份管理
│
├── pkg/
│   ├── types/                        # 继承:公共类型
│   ├── utils/                        # 继承:env/fs/ids/jsonutil/permission
│   └── testutil/                     # 继承
├── docs/
│   ├── ROADMAP.md                    # 继承(roster 自身的 roadmap,后续替换)
│   └── ...
├── test/
│   └── integration/
├── .references/                      # 仅本地参考,不入 git
│   └── claude-code-go/               # 上游基础(MIT)
├── .goreleaser.yaml                  # 已改:binary=roster, owner=45online
├── Makefile                          # 已改:build → bin/roster
├── go.mod                            # 已改:module=github.com/45online/roster
├── LICENSE                           # MIT(Roster);上游许可保留为 LICENSE.upstream
└── README.md                         # 本文件
```

---

## 17. 来源

Roster 不是从零开始:

### 代码基础
- **[claude-code-go](https://github.com/tunsuy/claude-code-go)**(MIT License)
  Roster 直接 fork 自此项目。提供了完整的 LLM API 客户端、本地工具系统、执行引擎、协调器、权限管线等基础设施。Roster 在此之上添加业务模块和外部 SaaS 适配器。

### 设计模式(后续阶段)
Roster 后续阶段计划引入若干设计模式 —— 主动 agent 循环、后台维护 subagent、内部代号黑名单、能力封装(Skills)、远程 API 触发等 —— 均用 clean-room 方式重新实现。

### LICENSE
- 保留上游 LICENSE 副本为 `LICENSE.upstream`(满足 MIT attribution)
- Roster 自身代码以 MIT 发布

---

## 18. 当前进度

| 阶段 | 状态 | 备注 |
|---|---|---|
| 0. Fork 改名 | ✅ 完成 | 二进制可编译 |
| 1. CLI 文案 + 启动 logo | ✅ 完成 | rebrand 完成,`roster --help` 干净 |
| 2. Module A(手动) | ✅ 完成 | `roster sync-issue` 可端到端跑 |
| 2.x. Poller + 防循环 + 自动触发 | ✅ 完成 | `roster takeover` 后台监听 issues.opened |
| 2.y. Claude API 接入 | ✅ 完成 | 智能 summary / issue_type / priority / component 抽取 |
| 2.z₁. JSONL 审计 + `.roster/config.yml` + `roster init` | ✅ 完成 | 审计日志 + 项目配置加载 |
| 2.z₂. `roster login` 凭证管理 | ✅ 完成 | github/jira/slack/claude + status/logout |
| 3. Module B: PR AI Review | ✅ 完成 | `review-pr` 手动 + takeover handler 接入,默认非阻塞 |
| 3. Module B | ⏳ | |
| 4. Module C | ⏳ | |
| 5. Module D | ⏳ | |
| 6. 打磨 | ⏳ | |
| 7+. 设计模式补齐 | ⏳ | |

### 阶段 0 已完成的具体动作
- [x] `rsync` 复制 roster 源码到主目录
- [x] `go mod edit -module github.com/45online/roster`
- [x] 全局 sed 替换 import path
- [x] `mv cmd/claude cmd/roster`
- [x] Makefile binary 更新
- [x] `.goreleaser.yaml` 更新(binary / owner / description)
- [x] 创建 `internal/{modules,adapters,poller,identity}/` 业务目录
- [x] `go mod tidy` + `go build ./cmd/roster` 通过

### 已知遗留问题
- ⚠️ CLI 输出文案仍显示 "Roster"(`./bin/roster --help` 仍是上游帮助文本) → 阶段 1 解决
- ⚠️ LICENSE 文件还是上游原文件,未添加 Roster 的 NOTICE
- ⚠️ docs/ROADMAP.md 是 roster 自己的路线图,与 Roster 不符
- ⚠️ AGENTS.md / CLAUDE.md / CONTRIBUTING.md 等仍是上游文档,需要重写或删除
