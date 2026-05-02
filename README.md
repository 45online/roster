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

🚧 **早期开发(Pre-alpha)**

| 阶段 | 状态 |
|---|---|
| 0. Fork claude-code-go 改名 + 业务目录 | ✅ 已完成 |
| 1. CLI 文案 + 启动 logo | ✅ 已完成 |
| 2. Module A: Issue → Jira(手动一次性 `sync-issue`) | ✅ 已完成 |
| 2.x. Poller + 防循环 + `takeover` 自动触发 | ✅ 已完成 |
| 2.y. Claude API 接入(智能字段抽取) | ✅ 已完成 |
| 2.z. SQLite 审计日志 + `.roster/config.yml` + `roster login` | ⏳ 下一步 |
| 3. Module B: PR AI Review | ⏳ |
| 4. Module C: Issue close → Confluence | ⏳ |
| 5. Module D: 告警聚合 → Slack | ⏳ |

二进制可编译运行,Module A 已可通过 `roster sync-issue` 手动触发完成
GitHub Issue → Jira 的端到端同步。后台 poller 与其他模块尚未实现。

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

⚠️ 目前还没有可用的 release。需要从源码构建。

### 前置要求
- Go 1.26+
- 一个虚拟员工身份的 GitHub 账户(配 PAT)
- Claude API key
- (可选)Jira / Confluence / Slack 的 API token

### 从源码构建
```bash
git clone https://github.com/45online/roster.git
cd roster
make build
./bin/roster --help
```

### 试用 Module A(已可用)

```bash
export ROSTER_GITHUB_TOKEN=ghp_xxx                    # GitHub PAT(虚拟员工账户)
export ROSTER_JIRA_URL=https://yourorg.atlassian.net  # Jira 站点
export ROSTER_JIRA_EMAIL=you@example.com              # Jira 账户邮箱
export ROSTER_JIRA_TOKEN=xxxx                         # Jira API token
```

**A. 一次性手动同步**
```bash
./bin/roster sync-issue --repo owner/name --issue 42 --jira-project ABC
# → ✓ Created ABC-123
#   同时 GH issue #42 出现评论:📋 Tracking in Jira: **ABC-123**
```

**B. 后台 daemon(自动监听 issues.opened)**
```bash
./bin/roster takeover --repo owner/name --jira-project ABC --interval 30s
# → ✓ Authenticated as @virtual-employee (anti-loop filter armed)
#   [poller] owner/name: starting (interval=30s, ...)
#   [mod-a] dispatching: owner/name#43 "fix login bug" (by @real-user)
#   [mod-a] ✓ ABC-124
# Ctrl+C 停止;cursor 持久化到 ~/.roster/cursors/owner_name.json
```

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
