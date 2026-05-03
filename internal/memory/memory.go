// Package memory is Roster's per-project "team memory" — a small set of
// well-known Markdown files under <repo>/.roster/memory/ that get
// inlined into every AI module's system prompt.
//
// Why files instead of a vector DB / RAG: the docs we want the AI to
// know about (project conventions, recent decisions, module owners,
// glossary) are tiny. They fit in a prompt-cache slot and are
// human-readable / editable / git-diffable. RAG would buy us nothing
// here except indirection, opacity, and a vector store dependency. See
// PRINCIPLES.md for the fact chain.
//
// The file set is deliberately **fixed** (4 well-known names) rather
// than "scan the directory". Predictable input → predictable prompt →
// predictable behaviour. Operators who want more files just put more
// content in the existing four.
package memory

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SubdirName is the conventional subdirectory name under .roster/.
const SubdirName = "memory"

// Limits keep the prompt size bounded. They're chosen to be friendly to
// prompt caching (Anthropic / OpenAI both cache prompts up to ~100K
// tokens) while still being a soft escape hatch when someone pastes a
// novel into conventions.md.
const (
	// PerFileMaxBytes caps a single memory file at read time.
	PerFileMaxBytes = 16 * 1024
	// TotalMaxBytes caps the combined inlined memory across all files.
	TotalMaxBytes = 64 * 1024
)

// File is one of the well-known memory files Roster looks for. Field
// keys exist only for stable iteration order.
type File struct {
	Name        string // file basename, e.g. "conventions.md"
	Heading     string // human label rendered above the contents in the prompt
	Description string // shown in `roster init`-generated placeholder
}

// Files lists the well-known memory files in render order. Adding a new
// entry here is enough to pick it up everywhere — modules, init,
// truncation, etc.
var Files = []File{
	{
		Name:        "conventions.md",
		Heading:     "Project conventions",
		Description: "How this project does things — coding style, PR/Issue norms, what's accepted and what isn't.",
	},
	{
		Name:        "decisions.md",
		Heading:     "Recent architectural decisions",
		Description: "Notable choices made and why. Append-only log; oldest first.",
	},
	{
		Name:        "module_owners.md",
		Heading:     "Module owners",
		Description: "Who owns which part of the codebase, who to defer to on stylistic calls.",
	},
	{
		Name:        "glossary.md",
		Heading:     "Project glossary",
		Description: "Project-specific terms whose meaning isn't obvious from the code.",
	},
}

// Memory holds the loaded contents of the well-known files. Empty
// (zero-value) Memory is a valid state — it just contributes nothing to
// the prompt.
type Memory struct {
	// dir is the .roster/memory directory we loaded from. Kept for
	// diagnostics / Path().
	dir string
	// contents maps file basename → file body (already truncated /
	// validated). Files that were missing or empty are absent.
	contents map[string]string
}

// Load reads the well-known files under <baseDir>/.roster/memory/
// (where baseDir is typically the repo root or the directory holding
// .roster/config.yml). A nonexistent directory is normal — Load
// returns an empty Memory, no error.
//
// Per-file size and aggregate size are clamped at read time. Files
// larger than PerFileMaxBytes are truncated with a "(truncated)"
// marker; once TotalMaxBytes is reached, subsequent files are skipped
// entirely with a similar marker on the last loaded one.
func Load(baseDir string) (*Memory, error) {
	dir := filepath.Join(baseDir, ".roster", SubdirName)
	m := &Memory{dir: dir, contents: map[string]string{}}

	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return m, nil // not a problem — no memory configured
		}
		return m, fmt.Errorf("memory: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return m, fmt.Errorf("memory: %s is not a directory", dir)
	}

	totalBytes := 0
	for _, f := range Files {
		if totalBytes >= TotalMaxBytes {
			break
		}
		path := filepath.Join(dir, f.Name)
		body, ok, err := readClamped(path, PerFileMaxBytes)
		if err != nil {
			return m, fmt.Errorf("memory: read %s: %w", f.Name, err)
		}
		if !ok || strings.TrimSpace(body) == "" {
			continue
		}
		// Aggregate cap: if adding this whole file would blow the
		// total, take only the slice that fits and mark it.
		remaining := TotalMaxBytes - totalBytes
		if len(body) > remaining {
			body = body[:remaining-len(truncationMark)] + truncationMark
		}
		m.contents[f.Name] = body
		totalBytes += len(body)
	}
	return m, nil
}

const truncationMark = "\n\n…(truncated)"

// readClamped reads a file, returning at most max bytes. The bool is
// false when the file simply doesn't exist (no error reported, since
// that's the expected case for projects that haven't filled in every
// well-known file).
func readClamped(path string, max int) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	defer f.Close()

	buf := make([]byte, max+1) // +1 to detect overflow
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false, err
	}
	if n > max {
		return string(buf[:max]) + truncationMark, true, nil
	}
	return string(buf[:n]), true, nil
}

// Empty reports whether no usable memory was loaded.
func (m *Memory) Empty() bool {
	return m == nil || len(m.contents) == 0
}

// Inject returns the memory text formatted for inclusion at the end of
// an AI module's system prompt. Returns "" when Empty().
//
// Format:
//
//	(blank line)
//	── Project memory ──
//	Below is the team's accumulated knowledge about this project. Use
//	it as the source of truth for conventions, decisions, ownership,
//	and terminology. Do not contradict it without explicit reason.
//
//	## Project conventions
//	<conventions.md content>
//
//	## Recent architectural decisions
//	<decisions.md content>
//
//	...
//
// The leading marker line ── Project memory ── lets us spot in the
// audit log whether memory was injected on a given call (just include
// the rendered prompt prefix, or its hash).
func (m *Memory) Inject() string {
	if m.Empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n── Project memory ──\n")
	b.WriteString("Below is the team's accumulated knowledge about this project. ")
	b.WriteString("Use it as the source of truth for conventions, decisions, ownership, ")
	b.WriteString("and terminology. Do not contradict it without an explicit reason.\n")

	for _, f := range Files {
		body, ok := m.contents[f.Name]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n", f.Heading)
		b.WriteString(strings.TrimSpace(body))
		b.WriteString("\n")
	}
	return b.String()
}

// Files returns the basenames of files actually loaded (in render
// order). Useful for `roster status` / audit attribution.
func (m *Memory) LoadedFiles() []string {
	if m == nil {
		return nil
	}
	out := make([]string, 0, len(m.contents))
	for _, f := range Files {
		if _, ok := m.contents[f.Name]; ok {
			out = append(out, f.Name)
		}
	}
	return out
}

// Bytes returns the total byte count of inlined content (pre-rendering).
// Useful for budget / diagnostics.
func (m *Memory) Bytes() int {
	if m == nil {
		return 0
	}
	n := 0
	for _, body := range m.contents {
		n += len(body)
	}
	return n
}

// Path returns the directory path memory was loaded from. Empty for
// nil / not-yet-loaded.
func (m *Memory) Path() string {
	if m == nil {
		return ""
	}
	return m.dir
}

// AllFileBasenames returns the set of well-known filenames as a sorted
// slice — useful for callers that want to iterate without depending on
// the package-level Files variable's order.
func AllFileBasenames() []string {
	out := make([]string, 0, len(Files))
	for _, f := range Files {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}
