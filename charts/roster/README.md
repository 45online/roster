# Roster Helm chart

Long-running daemon that takes over a virtual-employee GitHub account
and runs project-management workflows on Kubernetes. See the project
[README](../../README.md) (中文) or [README.en.md](../../README.en.md)
(English) for what Roster actually does.

## TL;DR

```bash
# 1. Create a Secret with your credentials (recommended for production)
kubectl create secret generic roster-creds \
  --from-literal=ROSTER_GITHUB_TOKEN=ghp_xxx \
  --from-literal=ROSTER_JIRA_URL=https://acme.atlassian.net \
  --from-literal=ROSTER_JIRA_EMAIL=alice@acme.com \
  --from-literal=ROSTER_JIRA_TOKEN=xxx \
  --from-literal=ROSTER_LLM_PROVIDER=openai-compatible \
  --from-literal=ROSTER_LLM_BASE_URL=https://api.deepseek.com \
  --from-literal=ROSTER_LLM_MODEL=deepseek-chat \
  --from-literal=ROSTER_LLM_API_KEY=sk-...

# 2. Install
helm install roster ./charts/roster \
  --set repo=owner/name \
  --set credentials.existingSecret=roster-creds
```

That's enough for polling mode (Roster pulls GitHub events every 30s,
no public endpoint needed).

## Required values

| key | example | what it is |
|---|---|---|
| `repo` | `acme/backend` | GitHub repo this Roster pod manages (one repo per Helm release) |
| `credentials.existingSecret` | `roster-creds` | Name of a Secret with `ROSTER_*` env keys (preferred), OR fill `credentials.*` inline (lab use only) |

## Common knobs

```yaml
# values.yaml
repo: acme/backend

credentials:
  existingSecret: roster-creds      # see schema in TL;DR above

config:
  yaml: |
    modules:
      issue_to_jira:
        enabled: true
        jira_project: ACME
      pr_review:
        enabled: true
        skip_paths: [docs/, "*.md", vendor/]
    budget:
      monthly_usd: 50
      on_exceed: downgrade

persistence:
  enabled: true
  size: 1Gi
  storageClass: gp3                 # cluster default if blank

webhook:
  enabled: true
  port: 8080
  ingress:
    enabled: true
    className: nginx
    host: roster.acme.com
    tls: { enabled: true }
```

## Webhook mode

Set `webhook.enabled=true` and let the Ingress (or a LoadBalancer
Service you create separately) expose port 8080. Then in the GitHub
repo Settings → Webhooks → Add webhook:

- Payload URL: `https://<your-host>/webhook/github`
- Content type: `application/json`
- Secret: same value as `ROSTER_WEBHOOK_SECRET` in the credentials Secret
- Events: tick **Issues** and **Pull requests**

The chart's Ingress also exposes `/healthz` for external uptime checks.

## Why a single replica

The deployment is hardcoded to `replicas: 1` and uses the `Recreate`
strategy. Roster's poller and webhook receiver both maintain a per-repo
cursor in `~/.roster/cursors/`; running >1 replica would split-brain
the cursor file and dispatch each event multiple times. To manage N
repos, install N Helm releases (one per repo) — each gets its own pod
+ PVC + cursor.

## Upgrade

```bash
helm upgrade roster ./charts/roster -f your-values.yaml
```

The chart computes a checksum of the rendered ConfigMap / chart-managed
Secret and writes it to a pod annotation, so config changes
automatically roll the pod.

## Uninstall

```bash
helm uninstall roster
# The PVC isn't deleted by default — delete it manually if you really
# mean to discard cursor history and audit logs:
kubectl delete pvc roster-roster-state
```

## Producing release artefacts

Bumping the chart:

```bash
# Edit Chart.yaml (version + appVersion)
helm package charts/roster
# Produces roster-X.Y.Z.tgz
```

A `helm repo` entry / OCI publish step isn't wired into the release
workflow yet; the chart is consumed directly from the source tree
(`./charts/roster`). When demand is clear, an `oci://ghcr.io/45online/charts`
or a GitHub-Pages-hosted repo can be added.
