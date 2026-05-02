package creds

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoad_MissingFile_ReturnsEmptyStore(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if s == nil {
		t.Fatal("expected empty store, got nil")
	}
	if s.Has("github") || s.Has("jira") {
		t.Error("empty store should not Has anything")
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	in := &Store{
		GitHub: &GitHubCreds{Token: "ghp_test"},
		Jira: &JiraCreds{
			URL: "https://acme.atlassian.net", Email: "a@b.com", Token: "tk",
		},
		Claude: &ClaudeCreds{APIKey: "sk-ant-test"},
	}
	if err := in.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !out.Has("github") || out.GitHub.Token != "ghp_test" {
		t.Errorf("github not preserved: %+v", out.GitHub)
	}
	if !out.Has("jira") || out.Jira.Email != "a@b.com" {
		t.Errorf("jira not preserved: %+v", out.Jira)
	}
	if !out.Has("claude") || out.Claude.APIKey != "sk-ant-test" {
		t.Errorf("claude not preserved: %+v", out.Claude)
	}
	if out.Has("slack") {
		t.Error("slack should be absent")
	}
}

func TestSave_FilePermissionsAre0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("0600 perms are POSIX-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	s := &Store{GitHub: &GitHubCreds{Token: "x"}}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("expected mode 0600, got %#o", mode)
	}
}

func TestHas_RequiresAllJiraFields(t *testing.T) {
	cases := []struct {
		name string
		j    *JiraCreds
		want bool
	}{
		{"complete", &JiraCreds{URL: "u", Email: "e", Token: "t"}, true},
		{"missing token", &JiraCreds{URL: "u", Email: "e"}, false},
		{"missing url", &JiraCreds{Email: "e", Token: "t"}, false},
		{"missing email", &JiraCreds{URL: "u", Token: "t"}, false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		s := &Store{Jira: tc.j}
		if got := s.Has("jira"); got != tc.want {
			t.Errorf("%s: Has(jira) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestClear_RemovesProvider(t *testing.T) {
	s := &Store{
		GitHub: &GitHubCreds{Token: "x"},
		Jira:   &JiraCreds{URL: "u", Email: "e", Token: "t"},
	}
	s.Clear("github")
	if s.Has("github") {
		t.Error("github should be cleared")
	}
	if !s.Has("jira") {
		t.Error("jira should still be present")
	}
	// Clearing absent provider is a no-op.
	s.Clear("slack")
}

func TestSave_AtomicAcrossCorruption(t *testing.T) {
	// If a previous .tmp is left around, a successful Save still produces
	// a usable file at `path`.
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path+".tmp", []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{GitHub: &GitHubCreds{Token: "x"}}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// .tmp should be gone (renamed away)
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not survive: %v", err)
	}
	// Final file is loadable
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.GitHub.Token != "x" {
		t.Errorf("token mismatch: %v", got.GitHub)
	}
}
