package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMemoryDir(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".roster", SubdirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func TestLoad_NoDir_ReturnsEmpty(t *testing.T) {
	m, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("expected no error for missing dir, got %v", err)
	}
	if !m.Empty() {
		t.Errorf("expected Empty() on missing dir")
	}
	if got := m.Inject(); got != "" {
		t.Errorf("Inject should be empty, got %q", got)
	}
}

func TestLoad_OneFile(t *testing.T) {
	root := writeMemoryDir(t, map[string]string{
		"conventions.md": "PRs must have tests.",
	})
	m, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Empty() {
		t.Fatal("expected non-empty memory")
	}
	out := m.Inject()
	for _, want := range []string{
		"── Project memory ──",
		"## Project conventions",
		"PRs must have tests.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Inject missing %q\n--- got ---\n%s", want, out)
		}
	}
}

func TestLoad_AllFour_RendersInOrder(t *testing.T) {
	root := writeMemoryDir(t, map[string]string{
		"conventions.md":   "A",
		"decisions.md":     "B",
		"module_owners.md": "C",
		"glossary.md":      "D",
	})
	m, _ := Load(root)
	out := m.Inject()
	// All four headings must appear, in declared order.
	idx := func(s string) int { return strings.Index(out, s) }
	a := idx("## Project conventions")
	b := idx("## Recent architectural decisions")
	c := idx("## Module owners")
	d := idx("## Project glossary")
	if !(a < b && b < c && c < d) {
		t.Errorf("headings out of order: a=%d b=%d c=%d d=%d", a, b, c, d)
	}
}

func TestLoad_EmptyFileSkipped(t *testing.T) {
	root := writeMemoryDir(t, map[string]string{
		"conventions.md": "",
		"glossary.md":    "real content",
	})
	m, _ := Load(root)
	out := m.Inject()
	if strings.Contains(out, "## Project conventions") {
		t.Error("empty file should not render a heading")
	}
	if !strings.Contains(out, "## Project glossary") {
		t.Error("non-empty file should render")
	}
}

func TestLoad_PerFileTruncation(t *testing.T) {
	huge := strings.Repeat("X", PerFileMaxBytes*2)
	root := writeMemoryDir(t, map[string]string{
		"conventions.md": huge,
	})
	m, _ := Load(root)
	out := m.Inject()
	if !strings.Contains(out, "(truncated)") {
		t.Error("expected truncation marker")
	}
	if len(out) > PerFileMaxBytes+1024 {
		// rendered prompt has some framing, but should be near the cap
		t.Errorf("rendered too large: %d bytes", len(out))
	}
}

func TestLoad_AggregateCap(t *testing.T) {
	// Each file is below per-file cap but together they blow the
	// aggregate cap. Last file should be partially truncated.
	huge := strings.Repeat("Y", PerFileMaxBytes-1)
	root := writeMemoryDir(t, map[string]string{
		"conventions.md":   huge,
		"decisions.md":     huge,
		"module_owners.md": huge,
		"glossary.md":      huge,
	})
	m, _ := Load(root)
	if m.Bytes() > TotalMaxBytes {
		t.Errorf("aggregate cap exceeded: %d > %d", m.Bytes(), TotalMaxBytes)
	}
}

func TestLoadedFiles_PreservesDeclaredOrder(t *testing.T) {
	root := writeMemoryDir(t, map[string]string{
		"glossary.md":      "g",
		"conventions.md":   "c",
		"module_owners.md": "m",
	})
	m, _ := Load(root)
	got := m.LoadedFiles()
	want := []string{"conventions.md", "module_owners.md", "glossary.md"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: %v vs %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("position %d: got %q, want %q", i, got[i], name)
		}
	}
}

func TestInject_NilSafe(t *testing.T) {
	var m *Memory
	if !m.Empty() {
		t.Error("nil Memory should report Empty")
	}
	if got := m.Inject(); got != "" {
		t.Errorf("nil Memory Inject should be empty, got %q", got)
	}
}

func TestAllFileBasenames(t *testing.T) {
	got := AllFileBasenames()
	want := []string{"conventions.md", "decisions.md", "glossary.md", "module_owners.md"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, n := range want {
		if got[i] != n {
			t.Errorf("[%d]: got %q, want %q", i, got[i], n)
		}
	}
}
