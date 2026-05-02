package projcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault_ReasonableValues(t *testing.T) {
	c := Default()
	if !c.Modules.IssueToJira.Enabled {
		t.Error("issue_to_jira should default to enabled")
	}
	if c.Modules.IssueToJira.DefaultIssueType != "Task" {
		t.Errorf("default issue type = %q, want Task", c.Modules.IssueToJira.DefaultIssueType)
	}
	if c.Budget.OnExceed != "downgrade" {
		t.Errorf("default on_exceed = %q, want downgrade", c.Budget.OnExceed)
	}
}

func TestLoad_FullConfig(t *testing.T) {
	yaml := `
project_name: backend-api
identity: chen-xiaolu
modules:
  issue_to_jira:
    enabled: true
    jira_project: BAPI
    default_issue_type: Story
    priority_mapping:
      Critical: Highest
    label_to_issue_type:
      defect: Bug
budget:
  monthly_usd: 100
  on_exceed: stop
dry_run: true
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ProjectName != "backend-api" {
		t.Errorf("ProjectName = %q", cfg.ProjectName)
	}
	if cfg.Modules.IssueToJira.JiraProject != "BAPI" {
		t.Errorf("JiraProject = %q", cfg.Modules.IssueToJira.JiraProject)
	}
	if cfg.Modules.IssueToJira.PriorityMapping["Critical"] != "Highest" {
		t.Errorf("priority mapping not loaded: %+v", cfg.Modules.IssueToJira.PriorityMapping)
	}
	if !cfg.DryRun {
		t.Error("dry_run should be true")
	}
	if cfg.Budget.OnExceed != "stop" {
		t.Errorf("on_exceed = %q", cfg.Budget.OnExceed)
	}
}

func TestValidate_RequiresJiraProjectWhenEnabled(t *testing.T) {
	yaml := `
modules:
  issue_to_jira:
    enabled: true
    jira_project: ""
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing jira_project")
	}
	if !strings.Contains(err.Error(), "jira_project") {
		t.Errorf("error should mention jira_project, got: %v", err)
	}
}

func TestValidate_RejectsBogusOnExceed(t *testing.T) {
	yaml := `
modules:
  issue_to_jira:
    enabled: true
    jira_project: X
budget:
  on_exceed: nuke
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "on_exceed") {
		t.Errorf("expected on_exceed validation error, got: %v", err)
	}
}

func TestLoadOrDefault_MissingFile(t *testing.T) {
	cfg, found, err := LoadOrDefault("/nonexistent/path/.roster/config.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for missing file")
	}
	if cfg == nil {
		t.Fatal("expected default config")
	}
	if !cfg.Modules.IssueToJira.Enabled {
		t.Error("default should have issue_to_jira enabled")
	}
}

func TestTemplate_ParsesValidly(t *testing.T) {
	// The shipped init template must itself pass through Load (after the
	// required jira_project is filled in).
	body := strings.Replace(Template, `jira_project: ""`, `jira_project: TEST`, 1)
	body = strings.Replace(body, `identity: ""`, `identity: alice`, 1)

	path := writeTemp(t, body)
	if _, err := Load(path); err != nil {
		t.Errorf("template should parse cleanly after filling required fields: %v", err)
	}
}

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write tmp config: %v", err)
	}
	return path
}
