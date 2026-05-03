# Roster

[中文](README.md) · [English](README.en.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · **Español**

> Suma un empleado IA a tu equipo.

> 📐 Principios de diseño / método de trabajo / postura contracorriente: [PRINCIPLES.md](PRINCIPLES.md) — lectura obligatoria para contribuidores y colaboradores IA antes de diseñar nuevas funciones.

Roster es un CLI de larga ejecución (caja local o VPS) que deja a una IA tomar el control de una cuenta de GitHub "empleado virtual" y operar el flujo de gestión del proyecto: lo que sucede en GitHub se sincroniza con Jira / Confluence / Slack, sin que un humano tenga que mover tickets entre sistemas.

Los desarrolladores se quedan en GitHub. La gestión vive en la trastienda. La IA hace de puente.

```
GitHub  ←→  Roster (empleado IA)  ←→  Jira / Confluence / Slack
                  │
                  └── LLM intercambiable (Claude / DeepSeek / Gemini / OpenAI / xAI / ...)
```

---

## Estado

**Publicado: [v0.2.1](https://github.com/45online/roster/releases/tag/v0.2.1)** — todas las fases del documento de diseño están implementadas: 4 módulos punto-a-punto, umbral de Budget en dos modos, aislamiento de identidad Undercover, fuentes de eventos polling + webhook, binarios multiplataforma + Docker multiarquitectura, **proveedor LLM múltiple** (Claude / DeepSeek / Gemini / OpenAI / xAI / cualquier endpoint compatible con OpenAI).

**Próxima etapa: dogfood.** Aquí termina el llenado de funcionalidades. La próxima semana es para correr Roster contra un repo real y observar qué supuestos se rompen (ajuste de prompts / fronteras de módulos / asperezas de UX). Historial completo: [CHANGELOG.md](CHANGELOG.md).

| Fase | Estado |
|---|---|
| 0. Fork de claude-code-go, rebrand, layout de negocio | ✅ |
| 1. Texto del CLI + logo de arranque | ✅ |
| 2. Módulo A: Issue → Jira (`sync-issue`) | ✅ |
| 2.x. Poller + anti-loop + `takeover` | ✅ |
| 2.y. Extracción inteligente de campos por Claude API | ✅ |
| 2.z₁. Auditoría JSONL + `.roster/config.yml` + `roster init` | ✅ |
| 2.z₂. Gestión de credenciales `roster login` | ✅ |
| 3. Módulo B: PR AI Review (`review-pr` + takeover) | ✅ |
| 4. Módulo C: Issue cerrado → Confluence (`archive-issue` + takeover) | ✅ |
| 5. Módulo D: agregación de alertas → Slack (`aggregate-alert`, sin IA) | ✅ |
| 6.a Panel de observabilidad `roster status` + `roster logs` | ✅ |
| 6.b Seguimiento de Budget (token + USD, totales mensuales) | ✅ |
| 6.c Umbral de Budget — modo stop | ✅ |
| 6.c+ Umbral de Budget — modo downgrade (sin IA, daemon sigue) | ✅ |
| 6.d Modo webhook + verificación HMAC GitHub | ✅ |
| 7. Undercover Mode (aislamiento de identidad + redacción de secretos) | ✅ |
| 8. Contenedorización + CI (Dockerfile + Actions + GHCR) | ✅ |
| 9. Proveedor LLM múltiple (Anthropic / OpenAI-compatible) | ✅ v0.2.0 |
| 10. Helm chart (despliegue Kubernetes) | ✅ v0.2.1 |
| 11. Slash command de Slack (`/roster …`) | ✅ v0.2.1 |

---

## Idea central

- **GitHub = única fuente de verdad**. Los desarrolladores solo abren Issues / hacen push de código / comentan PRs allí.
- **Jira / Confluence = espejos automáticos**. Sincronizados por la IA, solo lectura para gestión.
- **Slack = pulso en tiempo real**. La IA empuja alertas / notificaciones / resúmenes de revisión.
- **Empleado IA = cuenta humana virtual**. Sin etiqueta `[bot]` — tiene nombre, avatar, parece un colaborador GitHub normal. Solo que cada acción la realiza Roster en su nombre.

Analogía: como un asistente de programación IA es al código, **Roster es a la gestión de proyectos**.

---

## Cuatro módulos

| Módulo | Qué hace |
|---|---|
| **A. Issue → Jira** | Issue nuevo en GitHub → la IA extrae campos → ticket Jira creado automáticamente |
| **B. PR AI Review** | PR nuevo → análisis estático de IA + comentarios en línea (opcionalmente checkout local para correr tests) |
| **C. Issue cerrado → Confluence** | Issue cerrado → la IA resume el hilo → borrador en Confluence (humano publica) |
| **D. Agregación de alertas** | Alerta externa → la IA correlaciona commits/deploys recientes → canal público de Slack (formato de tablero de logs) |

---

## Inicio rápido

### Requisitos previos
- Una cuenta de GitHub para el empleado virtual (con un PAT)
- Una clave de API LLM — cualquiera de: Anthropic Claude, DeepSeek (la más barata), Google Gemini, OpenAI, xAI Grok, Together, Groq, etc.
- (Opcional) Tokens de API de Jira / Confluence / Slack

> **Proveedor LLM**: cualquier endpoint compatible con OpenAI Chat Completions funciona (DeepSeek / Gemini OpenAI-compat / OpenAI / xAI / Together / Groq). Configura con `roster login llm`. DeepSeek es actualmente la opción más barata capaz (~10× más barata que Claude Haiku, ~30× más barata que Claude Sonnet para extracción estructurada simple).

### Instalación (elige una)

**A. Desde el código fuente** (Go 1.26+)
```bash
git clone https://github.com/45online/roster.git
cd roster
make build
./bin/roster --help
```

**B. Docker** (cero dependencias)
```bash
docker pull ghcr.io/45online/roster:v0.2.1   # o :latest
docker run --rm ghcr.io/45online/roster:v0.2.1 --version
# → roster v0.2.1
```

Ejecución completa (monta `~/.roster` para persistir credenciales + audit + cursor; `-w /work` deja el comando dentro del repo montado):
```bash
docker run --rm \
  -v "$HOME/.roster:/home/roster/.roster" \
  -v "$PWD:/work" -w /work \
  -e ROSTER_GITHUB_TOKEN -e ROSTER_JIRA_TOKEN -e ROSTER_JIRA_URL -e ROSTER_JIRA_EMAIL -e ROSTER_LLM_API_KEY \
  ghcr.io/45online/roster:v0.2.1 takeover --repo owner/name
```

Multiarquitectura (linux/amd64, linux/arm64), imagen final ~40 MB.

**C. Homebrew** (post-release)
```bash
brew install 45online/tap/roster
```

**D. Helm chart (Kubernetes)**

```bash
# Crea Secret con credenciales (recomendado en producción; en lab puedes inline)
kubectl create secret generic roster-creds \
  --from-literal=ROSTER_GITHUB_TOKEN=ghp_xxx \
  --from-literal=ROSTER_LLM_API_KEY=sk-...

# Instala
helm install roster ./charts/roster \
  --set repo=owner/name \
  --set credentials.existingSecret=roster-creds
```

Guía completa: [charts/roster/README.md](charts/roster/README.md) — cubre webhook + Ingress + TLS, persistencia, ajustes de producción. **Diseño de un solo Pod** (el cursor no es concurrent-safe); instala N releases para gestionar N repos.

---

## Configuración

Dos rutas, elige una — variables de entorno (CI-friendly) o `roster login` (interactivo, persistido en `~/.roster/credentials.json` mode 0600).

```bash
# Ruta A: login interactivo (recomendado para local / VPS)
roster login github     # pega el PAT
roster login jira       # URL / email / token
roster login llm        # provider / base_url / model / api_key
roster login status     # muestra qué está configurado

# Ruta B: variables de entorno (CI / Docker)
export ROSTER_GITHUB_TOKEN=ghp_xxx
export ROSTER_JIRA_URL=https://yourorg.atlassian.net
export ROSTER_JIRA_EMAIL=you@example.com
export ROSTER_JIRA_TOKEN=xxxx

# LLM (cualquier combinación provider/model)
export ROSTER_LLM_PROVIDER=openai-compatible
export ROSTER_LLM_BASE_URL=https://api.deepseek.com
export ROSTER_LLM_MODEL=deepseek-chat
export ROSTER_LLM_API_KEY=sk-...
# o quédate en la ruta legada solo-Anthropic:
# export ANTHROPIC_API_KEY=sk-ant-xxx
```

### Opciones de proveedor LLM

| Proveedor | base_url | model sugerido | precio (in/out por Mtok) |
|---|---|---|---|
| Anthropic Claude | (default) | `claude-haiku-4-5-20251001` | $1 / $5 |
| Anthropic Claude | (default) | `claude-sonnet-4-6-20250514` | $3 / $15 |
| **DeepSeek** (barato) | `https://api.deepseek.com` | `deepseek-chat` | $0.27 / $1.10 |
| Google Gemini | `https://generativelanguage.googleapis.com/v1beta/openai/` | `gemini-2.5-flash` | $0.075 / $0.30 |
| OpenAI | `https://api.openai.com/v1` | `gpt-4o-mini` | $0.15 / $0.60 |
| xAI Grok | `https://api.x.ai/v1` | `grok-3` | $2 / $10 |

`roster status` acumula el coste mensual a la fecha usando la tarifa real de cada modelo (la tabla de precios va incorporada).

---

## Uso de los módulos

### A. Issue → Jira (one-shot)
```bash
./bin/roster sync-issue --repo owner/name --issue 42 --jira-project ABC
# → ✓ Created ABC-123
#   Comentario en GH issue #42: 📋 Tracking in Jira: ABC-123
```

### B. Daemon en segundo plano (auto-dispatch sobre issues.opened, PR opened/sync, issue closed)
```bash
cd <your-repo>
roster init                          # genera .roster/config.yml
# edita jira_project, habilita otros módulos…
roster takeover --repo owner/name
# → ✓ Loaded config from .roster/config.yml
#   ✓ Authenticated as @virtual-employee (anti-loop filter armed)
#   ✓ AI extractor enabled (provider=openai-compatible · model=deepseek-chat)
#   [poller] starting (interval=30s, ...)
#   [mod-a] dispatching: ...#43 → ✓ ABC-124
# Ctrl+C para detener. Cursor + audit log persisten bajo ~/.roster/.
```

### C. PR AI Review
```bash
./bin/roster review-pr --repo owner/name --pr 42
# → ✓ Review submitted (comment, 2 inline comments)
# Por defecto: cada veredicto se baja a COMMENT (un humano sigue teniendo que Approve / Block).
# --can-approve / --can-request-changes desbloquean esas puertas.
```

### D. Issue cerrado → borrador en Confluence
```bash
./bin/roster archive-issue \
  --repo owner/name --issue 42 \
  --space-id 12345 \
  --slack-channel "#archives"   # opcional
# → ✓ Draft created (id=987654)
# El borrador es solo para el owner hasta que un humano haga clic en Publish.
```

### E. Agregación de alertas → Slack
No requiere Claude (templated, determinista, $0).
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

**Filosofía de diseño**: El Módulo D juega el papel de "tablero de logs" — lista la actividad reciente para que oncall juzgue por sí mismo. Sin atribución causal, sin @-menciones, sin creación de tickets. La atribución equivocada es peor que ninguna atribución.

---

## Operaciones

### `roster status`
Una pantalla con todo: estado de credenciales, repos bajo gestión, actividad de las últimas 24h por módulo, último error, **gasto Budget mensual a la fecha** con desglose por módulo. `--json` para consumo programático.

### `roster logs <repo>`
Hace tail del audit JSONL con filtros: `--module` / `--status` / `--since 30m` / `-f` follow.

### Umbral de Budget

`.roster/config.yml`:
```yaml
budget:
  monthly_usd: 50
  on_exceed: downgrade   # 'stop' | 'downgrade'
```

- `stop` (default): rechaza arrancar si ya está sobre el límite; cancela el daemon en el primer evento que lo cruce.
- `downgrade`: mantiene el daemon corriendo pero deshabilita las llamadas a IA — el Módulo A vuelve al mapeo mecánico por etiquetas (sigue creando tickets en Jira), los Módulos B/C se saltan, el Módulo D no se ve afectado (no llama a IA de todos modos). Auto-restaura cuando el MTD baja del límite (p. ej. cambio de mes).

### Modo Webhook (reemplaza polling)

`.roster/config.yml`:
```yaml
webhook:
  enabled: true
  listen: ":8080"
  path: /webhook/github
  secret: ""                 # o export ROSTER_WEBHOOK_SECRET=...
```

En el repo de GitHub Settings → Webhooks:
- Payload URL: `<URL pública>/webhook/github`
- Content type: `application/json`
- Secret: igual a `ROSTER_WEBHOOK_SECRET`
- Marca "Issues" y "Pull requests"

Características: verificación HMAC-SHA256 (constant-time), event mapping `issues→IssuesEvent` / `pull_request→PullRequestEvent`, `ping` devuelve 200, `/healthz`, anti-loop, límite de cuerpo de 5 MB. **Mutuamente excluyente** con polling — los UUID de webhook y los IDs de la events-API no se deduplican entre las dos fuentes.

### Slack slash command (opcional)

Cuando el modo webhook está activo, los compañeros pueden disparar Roster desde Slack:

```
/roster status                        muestra el estado actual de Roster
/roster sync-issue owner/name#42      dispara el Módulo A
/roster review-pr  owner/name#42      dispara el Módulo B
/roster archive-issue owner/name#42   dispara el Módulo C
/roster help                          muestra esta lista
```

`.roster/config.yml`:
```yaml
slack:
  enabled: true
  path: /slack/command
  signing_secret: ""    # o export ROSTER_SLACK_SIGNING_SECRET=...
```

Configurando la app de Slack ([api.slack.com/apps](https://api.slack.com/apps) → Create New App):
1. Slash Commands → Create New Command:
   - Command: `/roster`
   - Request URL: `https://<tu-host-publico>/slack/command`
2. Basic Information → copia "Signing Secret" → ponlo en `signing_secret` (o env)
3. Install to Workspace

Características: verificación de firma HMAC-v0 (constant-time + ventana anti-replay de 5 min). `status` corre síncrono y devuelve inline; `sync-issue` / `review-pr` / `archive-issue` reconocen al instante con `:hourglass_flowing_sand: queued` y corren en una goroutine de fondo (la ventana de 3 segundos de Slack prohíbe correr la llamada completa al módulo inline). Los resultados aparecen en GitHub / Jira / Confluence directamente, no vuelven a Slack. El despachador protege contra invocaciones cross-repo: un Pod de Roster gestiona un solo repo, y el repo del comando Slack debe coincidir.

### Undercover Mode (siempre activo)

El empleado virtual es, para los lectores externos, indistinguible de un compañero humano. Dos capas:

1. **Capa de prompt** — cada system prompt de módulo IA agrega un invariante: no decir "As an AI", no nombrar el vendor, no exponer el id del modelo, sin disclaimers.
2. **Capa de output** — todo texto destinado a GitHub / Jira / Confluence / Slack pasa por un scrubber regex:
   - Secretos (`sk-ant-*`, `ghp_*`, `xox[abp]-*`) → marcadores explícitos
   - Identificadores de vendor / modelo → eliminados silenciosamente
   - Frases de auto-revelación de IA → eliminadas silenciosamente o reescritas

Los cuerpos de PR review no llevan cabecera `🤖 AI Review (model)` — en su lugar un footer discreto `_automated review_` mantiene orientados a los revisores humanos sin romper la persona. El Módulo D (agregación de alertas) también pasa por el redactor, ya que cita textualmente mensajes de commit y títulos de PR.

---

## Origen

Roster está construido sobre [claude-code-go](https://github.com/tunsuy/claude-code-go) (MIT). Reutiliza directamente los `internal/api`, `internal/tools`, `internal/engine`, `internal/coordinator`, etc. del upstream, y agrega encima los paquetes `adapters/`, `modules/`, `poller/`, `webhookreceiver/`, `audit/`, `budget/`, `creds/`, `projcfg/`, `undercover/`.

Ver [NOTICE](NOTICE) para la relación de fork.

---

## Licencia

[MIT](LICENSE) — ver [LICENSE](LICENSE) y [LICENSE.upstream](LICENSE.upstream) para la atribución completa.
