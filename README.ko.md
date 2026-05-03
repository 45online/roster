# Roster

[中文](README.md) · [English](README.en.md) · [日本語](README.ja.md) · **한국어** · [Español](README.es.md)

> 팀에 AI 직원을 한 명 추가합니다.

Roster는 로컬 / VPS에서 상시 동작하는 CLI 도구로, "가상 직원" GitHub 계정을 AI에게 맡겨 프로젝트 관리 워크플로우를 자동으로 처리합니다. GitHub에서 발생한 일은 Jira / Confluence / Slack으로 동기화되어, 사람이 시스템 사이로 티켓을 옮길 필요가 없어집니다.

개발자는 GitHub에만 머문다. 관리는 백오피스에 있다. AI가 다리를 놓는다.

```
GitHub  ←→  Roster (AI 직원)  ←→  Jira / Confluence / Slack
                  │
                  └── 교체 가능한 LLM(Claude / DeepSeek / Gemini / OpenAI / xAI / ...)
```

---

## 상태

**릴리스됨: [v0.2.1](https://github.com/45online/roster/releases/tag/v0.2.1)** — 설계 문서가 계획한 모든 단계가 구현됨: 4개의 엔드투엔드 모듈, 2가지 모드의 Budget 임계값, Undercover 신원 격리, Polling + Webhook 이중 이벤트 소스, 크로스플랫폼 바이너리 + 멀티아키 Docker, **멀티 LLM provider** (Claude / DeepSeek / Gemini / OpenAI / xAI / OpenAI 호환 엔드포인트 모두).

**다음 단계는 도그푸드(dogfood)**. 기능 채우기는 여기까지. 다음 한 주는 실제 리포지토리에서 돌려가며 어떤 가정이 깨지는지(프롬프트 튜닝 / 모듈 경계 / UX 거친 부분) 관찰하는 시간. 전체 릴리스 이력은 [CHANGELOG.md](CHANGELOG.md).

| 단계 | 상태 |
|---|---|
| 0. claude-code-go에서 fork, 리브랜드, 비즈니스 레이아웃 | ✅ |
| 1. CLI 카피 + 부팅 logo | ✅ |
| 2. Module A: Issue → Jira (`sync-issue`) | ✅ |
| 2.x. Poller + 안티루프 + `takeover` | ✅ |
| 2.y. Claude API 스마트 필드 추출 | ✅ |
| 2.z₁. JSONL 감사 + `.roster/config.yml` + `roster init` | ✅ |
| 2.z₂. `roster login` 자격증명 관리 | ✅ |
| 3. Module B: PR AI Review (`review-pr` + takeover) | ✅ |
| 4. Module C: Issue close → Confluence (`archive-issue` + takeover) | ✅ |
| 5. Module D: 알림 집계 → Slack (`aggregate-alert`, AI 미사용) | ✅ |
| 6.a `roster status` + `roster logs` 관찰 패널 | ✅ |
| 6.b Budget 추적 (token + USD, 월별 집계) | ✅ |
| 6.c Budget 임계값 — stop 모드 | ✅ |
| 6.c+ Budget 임계값 — downgrade 모드 (AI 끄고 daemon 유지) | ✅ |
| 6.d Webhook 모드 + GitHub HMAC 검증 | ✅ |
| 7. Undercover Mode (신원 격리 + 비밀 redaction) | ✅ |
| 8. 컨테이너화 + CI (Dockerfile + Actions + GHCR) | ✅ |
| 9. 멀티 LLM provider (Anthropic / OpenAI 호환) | ✅ v0.2.0 |
| 10. Helm chart (Kubernetes 배포) | ✅ v0.2.1 |
| 11. Slack 슬래시 명령 (`/roster …`) | ✅ v0.2.1 |

---

## 핵심 아이디어

- **GitHub = 단일 진리 원천**. 개발자는 GitHub에서만 Issue 등록 / 코드 push / PR 코멘트.
- **Jira / Confluence = 자동 미러**. AI가 동기화하고, 매니지먼트는 읽기 전용.
- **Slack = 실시간 맥박**. AI가 알림 / 통지 / 리뷰 요약을 푸시.
- **AI 직원 = 가상 사람 계정**. `[bot]` 태그 없음 — 이름과 아바타가 있는, 보통의 GitHub collaborator처럼 보임. 단지 모든 액션은 Roster가 대신 수행.

비유: AI 코딩 어시스턴트가 코드에 대한 것이라면, **Roster는 프로젝트 관리에 대한 것**.

---

## 4 가지 모듈

| 모듈 | 하는 일 |
|---|---|
| **A. Issue → Jira** | 새 GitHub Issue → AI가 필드 추출 → Jira 티켓 자동 생성 |
| **B. PR AI Review** | 새 PR → AI 정적 분석 + 라인 코멘트 (옵션: 로컬 checkout 후 테스트 실행) |
| **C. Issue close → Confluence** | Issue 닫힘 → AI가 스레드 요약 → Confluence 초안 (사람이 게시) |
| **D. 알림 집계** | 외부 알림 → AI가 최근 commit/deploy를 연관 → Slack 채널 (로그 보드 형태) |

---

## 빠른 시작

### 사전 요구 사항
- 가상 직원용 GitHub 계정 (PAT 포함)
- LLM API 키 — Anthropic Claude / DeepSeek (가장 저렴) / Google Gemini / OpenAI / xAI Grok / Together / Groq 등
- (옵션) Jira / Confluence / Slack API 토큰

> **LLM provider**: OpenAI Chat Completions 호환 엔드포인트면 모두 동작 (DeepSeek / Gemini OpenAI 호환 / OpenAI / xAI / Together / Groq). `roster login llm`으로 설정. 단순 구조화 추출에 대해서는 현재 DeepSeek가 가장 저렴 (Claude Haiku의 약 1/10, Claude Sonnet의 약 1/30).

### 설치 (선택)

**A. 소스에서** (Go 1.26+)
```bash
git clone https://github.com/45online/roster.git
cd roster
make build
./bin/roster --help
```

**B. Docker** (의존성 제로)
```bash
docker pull ghcr.io/45online/roster:v0.2.1   # 또는 :latest
docker run --rm ghcr.io/45online/roster:v0.2.1 --version
# → roster v0.2.1
```

전체 실행 (`~/.roster`를 마운트하여 credentials + audit + cursor 영구 보관; `-w /work`로 마운트한 리포지토리에서 동작):
```bash
docker run --rm \
  -v "$HOME/.roster:/home/roster/.roster" \
  -v "$PWD:/work" -w /work \
  -e ROSTER_GITHUB_TOKEN -e ROSTER_JIRA_TOKEN -e ROSTER_JIRA_URL -e ROSTER_JIRA_EMAIL -e ROSTER_LLM_API_KEY \
  ghcr.io/45online/roster:v0.2.1 takeover --repo owner/name
```

멀티아키 (linux/amd64, linux/arm64), 최종 이미지 ~40 MB.

**C. Homebrew** (릴리스 이후)
```bash
brew install 45online/tap/roster
```

**D. Helm chart (Kubernetes)**

```bash
# 자격증명 Secret 생성 (프로덕션 권장; 실습용은 values 인라인도 가능)
kubectl create secret generic roster-creds \
  --from-literal=ROSTER_GITHUB_TOKEN=ghp_xxx \
  --from-literal=ROSTER_LLM_API_KEY=sk-...

# 설치
helm install roster ./charts/roster \
  --set repo=owner/name \
  --set credentials.existingSecret=roster-creds
```

전체 가이드: [charts/roster/README.md](charts/roster/README.md) — webhook + Ingress + TLS, persistence, 프로덕션 noise까지. **단일 Pod 설계** (cursor는 동시성 안전하지 않음); N개의 리포를 관리하려면 N개의 release를 설치.

---

## 설정

env vars (CI 친화) 또는 `roster login` (대화형, `~/.roster/credentials.json` mode 0600에 영구 저장) 중 하나 선택.

```bash
# A: 대화형 login (로컬 / VPS 권장)
roster login github     # PAT 붙여넣기
roster login jira       # URL / email / token
roster login llm        # provider / base_url / model / api_key
roster login status     # 현재 상태 표시

# B: env vars (CI / Docker)
export ROSTER_GITHUB_TOKEN=ghp_xxx
export ROSTER_JIRA_URL=https://yourorg.atlassian.net
export ROSTER_JIRA_EMAIL=you@example.com
export ROSTER_JIRA_TOKEN=xxxx

# LLM (provider/model 임의 조합)
export ROSTER_LLM_PROVIDER=openai-compatible
export ROSTER_LLM_BASE_URL=https://api.deepseek.com
export ROSTER_LLM_MODEL=deepseek-chat
export ROSTER_LLM_API_KEY=sk-...
# 또는 레거시 Anthropic 전용 경로:
# export ANTHROPIC_API_KEY=sk-ant-xxx
```

### LLM provider 옵션

| Provider | base_url | 권장 model | 가격 (in/out per Mtok) |
|---|---|---|---|
| Anthropic Claude | (default) | `claude-haiku-4-5-20251001` | $1 / $5 |
| Anthropic Claude | (default) | `claude-sonnet-4-6-20250514` | $3 / $15 |
| **DeepSeek** (저렴) | `https://api.deepseek.com` | `deepseek-chat` | $0.27 / $1.10 |
| Google Gemini | `https://generativelanguage.googleapis.com/v1beta/openai/` | `gemini-2.5-flash` | $0.075 / $0.30 |
| OpenAI | `https://api.openai.com/v1` | `gpt-4o-mini` | $0.15 / $0.60 |
| xAI Grok | `https://api.x.ai/v1` | `grok-3` | $2 / $10 |

`roster status`는 실제 모델 요율로 월간 누적 비용을 집계합니다 (가격표 내장).

---

## 모듈 사용법

### A. Issue → Jira (원샷)
```bash
./bin/roster sync-issue --repo owner/name --issue 42 --jira-project ABC
# → ✓ Created ABC-123
#   GH issue #42에 코멘트: 📋 Tracking in Jira: ABC-123
```

### B. 백그라운드 daemon (issues.opened, PR opened/sync, issue closed 자동 디스패치)
```bash
cd <your-repo>
roster init                          # .roster/config.yml 생성
# jira_project 편집, 다른 모듈 활성화…
roster takeover --repo owner/name
# → ✓ Loaded config from .roster/config.yml
#   ✓ Authenticated as @virtual-employee (anti-loop filter armed)
#   ✓ AI extractor enabled (provider=openai-compatible · model=deepseek-chat)
#   [poller] starting (interval=30s, ...)
#   [mod-a] dispatching: ...#43 → ✓ ABC-124
# Ctrl+C로 정지. cursor + audit 로그는 ~/.roster/에 영구 보관.
```

### C. PR AI Review
```bash
./bin/roster review-pr --repo owner/name --pr 42
# → ✓ Review submitted (comment, 2 inline comments)
# 기본: 모든 verdict는 COMMENT로 다운그레이드 (Approve / Block은 사람이 클릭).
# --can-approve / --can-request-changes로 게이트 해제.
```

### D. Issue close → Confluence 초안
```bash
./bin/roster archive-issue \
  --repo owner/name --issue 42 \
  --space-id 12345 \
  --slack-channel "#archives"   # 옵션
# → ✓ Draft created (id=987654)
# 초안은 owner만 보이고, 사람이 Publish할 때까지 비공개.
```

### E. 알림 집계 → Slack
Claude 불필요 (템플릿 기반, 결정적, $0).
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

**설계 철학**: Module D는 "로그 보드" 역할 — 온콜이 직접 판단할 수 있도록 최근 활동을 나열. 인과 귀속하지 않음, @-멘션하지 않음, 티켓 생성하지 않음. 잘못된 귀속이 귀속하지 않음보다 더 나쁘다.

---

## 운영

### `roster status`
한 화면에 자격증명 / 관리 중인 리포지토리 / 최근 24h 모듈별 활동 / 최신 에러 / **월간 Budget**까지 모두 표시. `--json`으로 기계 가독 출력.

### `roster logs <repo>`
JSONL audit 로그를 `--module` / `--status` / `--since 30m` / `-f` 필터로 tail.

### Budget 임계값

`.roster/config.yml`:
```yaml
budget:
  monthly_usd: 50
  on_exceed: downgrade   # 'stop' | 'downgrade'
```

- `stop` (기본): 이미 한도 초과면 시작 거부; 한도 초과 첫 이벤트에서 daemon 종료.
- `downgrade`: daemon은 계속 돌지만 AI 호출은 비활성화. Module A는 라벨 기반 메커니컬 매핑으로 폴백 (Jira 티켓은 여전히 생성), Module B / C는 스킵, Module D는 영향 없음. 월이 바뀌어 MTD가 한도 아래로 내려오면 자동 복구.

### Webhook 모드 (polling 대체)

`.roster/config.yml`:
```yaml
webhook:
  enabled: true
  listen: ":8080"
  path: /webhook/github
  secret: ""                 # 또는 export ROSTER_WEBHOOK_SECRET=...
```

GitHub 리포지토리 Settings → Webhooks:
- Payload URL: `<public URL>/webhook/github`
- Content type: `application/json`
- Secret: `ROSTER_WEBHOOK_SECRET`과 동일
- Issues + Pull requests 체크

특징: HMAC-SHA256 검증 (constant-time), event mapping, `ping` → 200, `/healthz`, anti-loop, 5MB 본문 상한. **polling과 상호 배타적** — webhook UUID와 events-API ID는 다른 namespace이며 두 소스 간 중복 제거가 불가능하므로.

### Slack 슬래시 명령 (옵션)

webhook 모드가 켜져 있으면, 팀원이 Slack에서 직접 Roster를 트리거 가능:

```
/roster status                        Roster의 현재 상태
/roster sync-issue owner/name#42      Module A 트리거
/roster review-pr  owner/name#42      Module B 트리거
/roster archive-issue owner/name#42   Module C 트리거
/roster help                          도움말
```

`.roster/config.yml`:
```yaml
slack:
  enabled: true
  path: /slack/command
  signing_secret: ""    # 또는 ROSTER_SLACK_SIGNING_SECRET=...
```

Slack app 구성 ([api.slack.com/apps](https://api.slack.com/apps) → Create New App):
1. Slash Commands → Create New Command:
   - Command: `/roster`
   - Request URL: `https://<your-public-host>/slack/command`
2. Basic Information → "Signing Secret" 복사 → `signing_secret` 또는 env에 입력
3. Install to Workspace

특징: HMAC-v0 서명 검증 (constant-time + 5분 리플레이 윈도우). `status`는 동기. `sync-issue` / `review-pr` / `archive-issue`는 즉시 ack(`:hourglass_flowing_sand: queued`)하고 백그라운드 goroutine에서 실행 (Slack의 3초 응답 한계 때문). 결과는 GitHub / Jira / Confluence에 직접 나오고, Slack으로 돌아오지 않음. Roster 인스턴스는 단일 리포 관리이므로 Slack 명령의 리포가 일치해야 함.

### Undercover Mode (항상 ON)

가상 직원은 외부 독자에게는 사람과 구별되지 않음. 2개 층:

1. **프롬프트 층** — 각 AI 모듈의 system prompt에 invariant를 추가: "As an AI"라고 말하지 말 것, vendor 이름 말하지 말 것, model id 노출하지 말 것, disclaimer 추가하지 말 것.
2. **출력 층** — GitHub / Jira / Confluence / Slack로 보내기 직전 regex scrubber 통과:
   - 시크릿 (`sk-ant-*`, `ghp_*`, `xox[abp]-*`) → 명시적 마커
   - vendor / model 식별자 → 조용히 제거
   - AI 자기 노출 표현 → 조용히 제거 또는 재작성

PR 리뷰 본문에는 `🤖 AI Review (model)` 헤더가 없는 대신, 조용한 `_automated review_` 푸터가 남음. Module D (알림 집계)도 redactor를 통과 — commit 메시지와 PR 제목을 그대로 인용하기 때문.

---

## 출처

Roster는 [claude-code-go](https://github.com/tunsuy/claude-code-go) (MIT) 위에 만들어졌습니다. 상위의 `internal/api`, `internal/tools`, `internal/engine`, `internal/coordinator` 등을 직접 재사용하고, `adapters/`, `modules/`, `poller/`, `webhookreceiver/`, `audit/`, `budget/`, `creds/`, `projcfg/`, `undercover/` 등의 패키지를 그 위에 추가합니다.

fork 관계는 [NOTICE](NOTICE)를 참조.

---

## 라이선스

[MIT](LICENSE) — 전체 attribution은 [LICENSE](LICENSE)와 [LICENSE.upstream](LICENSE.upstream) 참조.
