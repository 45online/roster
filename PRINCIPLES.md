# Roster Principles

> 项目的工作方法 / 决策原则 / 反共识立场。
> 给所有 contributor 和 AI 协作者读 — **设计新功能前过一遍**。

最后更新:2026-05-03(v0.2.x)

---

## 这份文档存在的理由

Roster 是早期产品。功能层面还在演进,**执行过程中容易跑飞** ——
追逐业界主流的 multi-agent / RAG / agent dialog 等热点,
做着做着就偏离了真正的差异化价值。

这份文档把我们的**核心判断 + 工作方法**写下来,
作为防漂移的锚点。读完它,你应该知道:

- Roster 卖的不可替代的东西是什么
- 我们故意不做哪些主流功能,以及为什么
- 每次设计决策前问自己哪 5 个问题

---

## 三个核心论点

### 1. AI 员工 = 工具 + 时间

工具(如 Claude API)是**无状态**的,每次调用是新生。
员工有**时间维度**:昨天的事影响今天的判断,周积月累。

模型变聪明是 Anthropic / OpenAI 的事 —— 产品层做不出。
**Roster 的差异化不在智能,在给一个普通智能的工具加上时间维度。**

### 2. AI 不下班,每天比昨天稍微熟悉这个项目一点

价值不在"AI 比人聪明",而在两件具体的事:

- **持续在岗** —— 24/7 触发,真人下班 AI 继续
- **持续累积** —— 每天对项目的理解多一点(Module C 归档时记录决策、
  Module B review 时学到 repo 偏好、运营时累积惯例)

### 3. 团队 ≠ N 个 agent 的集合

真正的 "AI 团队" 不是数量上的 N,是**结构化的责任分工 + 集体记忆**:

- 谁负责哪类事件(责任)
- 谁的处理结果给谁后续 reference(handoff)
- 团队对项目的共识跨员工存在(集体记忆)

CrewAI / AutoGen 那种 "agent 互相对话产生涌现智能" **不是我们的方向**。
我们做的是**协作机制的工程化**,不是 emergence。

---

## 第一性原理 5 问

每个新功能 / 重大决策,**动手前过一遍**。

### Q1:我们到底在卖什么不可替代的东西?

> **给工具加时间维度,变成员工**。其它都是手段。

如果一个功能不服务这个核心,先问自己**为什么要做**。

### Q2:这是基于 fact 的判断,还是基于"别人都这么做"?

- **Fact 链**:列出关键事实 → 推理结论
- **类比链**("Cursor 这么做我们也这么做"):警惕
- 反共识不一定对,但**默认共识也不一定对** —— 都需要 fact 支撑

### Q3:如果只能交付 80%,砍哪 20%?

强迫识别核心。砍不掉的就是核心。

拒绝 "all or nothing" 心态 —— 它通常意味着 over-design。

### Q4:这功能在 N=1 时还有用吗?

(单用户 / 单 repo / 单项目)

如果答案是 "等 N=10 才有意义",**很可能是过度抽象**。
真实用户从 N=1 开始。

### Q5:它的失败模式是什么?最坏情况谁兜底?

- API 5xx?network partition?LLM 幻觉?secret 泄露?
- 失败时 daemon 不能崩、数据不能损、用户能 recover
- **兜底没想清楚的功能不应该 ship**

---

## 真正第一性的判断 vs 经验直觉

诚实的拆分。**不要把所有判断都贴上"第一性"标签**。

### 真正第一性(从 fact 推导)

- **AI 员工 = 工具 + 时间**
  Fact:工具无状态,员工有时间。差异化在时间。

- **Module D 不归因**
  Fact:correlation ≠ causation。错归因(半夜 at 错人)的代价 >
  没归因(oncall 多看几行 commit)的代价。

- **Memory 用 markdown 不用 RAG**
  Fact:RAG 召回不准 + 黑盒;项目 wiki < 50 KB;prompt cache 接近免费。
  → 全文喂入 + 真人可读可写 > vector DB。

- **Module B 默认非阻塞(`CanApprove=false`)**
  Fact:错 approve 一次(bug 入主干)的代价 >> 少 approve 一次
  (真人多点一下)的代价。
  → 选保守。

- **Webhook 与 polling 互斥**
  Fact:webhook delivery UUID 与 events-API ID 是独立 namespace。
  无法 dedupe → 必须二选一。

- **Undercover Mode 强制**
  Fact:虚拟员工的 cover 一旦破,信任成本远高于设计成本。
  → 默认开,不留 opt-out。

### 不是第一性(经验 / 直觉 / 实用主义)

诚实说,以下决定是基于经验直觉,不是从 fact 推导:

- Module 切成 A/B/C/D 这个粒度(可以按 role 切)
- polling 30s(惯例 + GitHub 推荐值)
- audit 用 JSONL 不用 SQLite(够用就好)
- Helm `replicas: 1` Recreate(基于 cursor 文件不并发安全这个**实现层**事实)
- 价格表内置而不是从 provider API 拉(实用,不是必须)

**碰到这些决策,可以更动**。它们不是宪法,是当下选择。
**不要把"碰巧 work 了"误认成"第一性正确"**。

---

## 反共识清单(我们主动不做的)

每一项都是行业主流,但放在 Roster 的 fact 链上是**负 ROI**。

| 主流方向 | 我们不做的理由 |
|---|---|
| Multi-agent dialogue(CrewAI / AutoGen) | token 成本爆炸 + debug 噩梦 + 没 fact 支撑"涌现"对 Roster 场景必要 |
| RAG / vector DB / embedding | 项目 wiki 体积 < cache 容量,直接喂入更准更便宜 |
| LLM-as-router(manager-LLM 分配 task) | deterministic rules 80% 场景够;LLM 路由引入不可预测性 |
| 追"更聪明的 agent" | 那是 model 厂的战场,产品层做不出 |
| AI 员工互相 dialogue | 协作的本质是 handoff + 共享记忆,不是聊天 |
| AI 自动招聘 / 生成新员工 | 递归创建几乎必然失控,目前无场景 |
| AI 员工的"个性 / 情感" | 对工作没有正向价值,徒增不可预测性 |
| 因果归因 / 自动 root-cause(Module D) | 错归因比没归因更糟 |

每条都是**有意识的拒绝**,不是没想到。
PR 提议做这些之前,先看这张表 + 写出 fact 反驳。

---

## 已落地决策的原则一致性自检

每个 ship 出去的功能都该能溯源到 fact / 原则:

| 功能 | 对应的 fact / 原则 |
|---|---|
| Module A 默认机械映射,AI 是 fallback | Q5 失败模式 — Claude 不可用时仍能产出 ticket |
| Module B `_automated review_` 而不是 `🤖 AI Review` | Undercover Mode — 不暴露身份 |
| Module C 草稿 → 真人发布 | AI 决策的可逆性 — 文档进入正式空间前必须人 review |
| Module D 不归因 | correlation ≠ causation |
| Budget threshold downgrade 边缘触发 | Q5 失败模式 — 月度滚动后自动恢复 |
| Webhook 与 polling 互斥 | 防双重处理(无法 dedupe) |
| Multi-LLM provider | "AI 员工 ≠ Claude" — 工具是可换的,身份不是 |
| Slack `/roster` 命令异步 ack | Q5 失败模式 — Slack 3 秒响应限制 vs 实际 module 调用时长 |
| Project Memory(规划中) | "AI 每天比昨天熟悉一点" — 时间维度的最低门槛 |
| W1 多 identity 静态路由(规划中) | Q2 fact-based — deterministic 比 LLM 路由更可信 |

**如果某个未来 PR 找不到对应的原则 / fact,先回头问 Q1-Q5。**

---

## 怎么用这份文档

### 真人 contributor

- 提 issue / PR 时,**在描述里显式声明**违背了哪条原则,以及为什么
- 提 "加 RAG / Multi-agent" 之前先看反共识清单 — 我们已经讨论过为什么不做
- 大设计决策的 PR 里 ref 这份文档的章节(便于未来追溯)

### AI 协作者(Claude Code / Cursor / Aider 等)

这份文档应该被 ingest 到 system prompt / context window。
**每次涉及架构 / 模块边界 / 引入新依赖**的工作前,
显式过一遍 Q1-Q5,**并在响应里说明判断依据**。

特别注意:

- ❌ 不要因为"业界都这么做"就向 RAG / multi-agent / agent dialogue 靠拢
- ❌ 不要追"更聪明" — 我们的差异化不在那里
- ❌ 任何"等 N=10 才有用"的功能默认拒绝
- ✅ 提议反共识方向时,**写出 fact 链**让真人评审

### 这份文档自身的更新规则

- 加新原则:**必须有具体 fact 支撑**(不能只是直觉)
- 删旧原则:**必须解释新的 fact 推翻了它**
- 每次 minor 版本(v0.x → v0.x+1)review 一遍,看是否还成立
- timestamp 永远更新

---

## 一个反共识的副产物

按这套方法走,Roster 会**主动跟行业脱钩**几件事(列在反共识清单)。

这种"主流但错"的判断,正是第一性原理的实际作用 ——
**不是发明新东西,而是敢于不做大家都在做的事**。

每次有人问 "Roster 为啥不上 RAG / Multi-agent / 主流 agent 框架",
答案应该是:**我们看了 fact,得出了不同的结论**。

不是叛逆,是诚实。

---

> 这份文档是 living document。
> Roster 的产品形态会变,但这套**思考方法**应该跨版本保持稳定。
>
> 如果有一天我们做了 vector DB / multi-agent dialogue,
> 那意味着 fact 变了 —— 把变化的 fact 写进这份文档,
> 然后才动手。
