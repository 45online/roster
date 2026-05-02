package projcfg

// Template is the YAML body written by `roster init` into a fresh repo.
// It includes inline comments to guide the reader.
const Template = `# Roster project configuration.
# Lives at <repo>/.roster/config.yml. Read by 'roster takeover'.

project_name: ""             # human-friendly name for this project
identity: ""                 # the virtual employee account (must match GH login)

modules:
  # Module A — sync new GitHub issues to Jira tickets.
  issue_to_jira:
    enabled: true
    jira_project: ""         # Jira project key (e.g. "BAPI"); REQUIRED if enabled
    default_issue_type: Task
    priority_mapping:        # map a GH label name to a Jira priority
      P0: Highest
      P1: High
      P2: Medium
    label_to_issue_type:     # override Jira issue type by GH label
      bug: Bug

  # Module B — AI review of pull requests. (not yet implemented)
  pr_review:
    enabled: false

  # Module C — archive closed Issue threads to Confluence as drafts. (not yet)
  issue_to_confluence:
    enabled: false

  # Module D — aggregate external alerts into a Slack channel. (not yet)
  alert_aggregation:
    enabled: false
    slack_channel: ""
    lookback: 1h

budget:
  monthly_usd: 50            # USD spent on Claude API per month for this repo
  on_exceed: downgrade       # 'downgrade' (skip AI) | 'stop' (halt all modules)

dry_run: false               # true = log what would happen but don't write
`
