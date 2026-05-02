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

  # Module B — AI review of pull requests.
  pr_review:
    enabled: false
    skip_paths:               # if every changed file matches, skip the review
      - "docs/"
      - "*.md"
      - "vendor/"
    max_diff_bytes: 65536     # 64 KB; larger diffs are truncated
    # Safety gates (default false): when off, even an "approve" verdict is
    # submitted as a plain COMMENT — a human still has to click the button.
    can_approve: false
    can_request_changes: false

  # Module C — archive closed Issue threads to Confluence as drafts.
  # Uses the same Atlassian credentials as Jira ('roster login jira').
  issue_to_confluence:
    enabled: false
    space_id: ""             # numeric Confluence space ID (REQUIRED if enabled)
    parent_page_id: ""       # optional: nest drafts under this page
    completed_label: completed   # only archive issues carrying this label
    slack_channel: ""        # optional: post a draft URL here ('roster login slack')

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
