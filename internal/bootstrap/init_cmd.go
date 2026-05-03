package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/45online/roster/internal/memory"
	"github.com/45online/roster/internal/projcfg"
)

// newRosterInitCmd builds `roster init`: drop a starter .roster/config.yml
// into the current directory.
func newRosterInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a starter .roster/config.yml in the current directory",
		Long: longDesc(`
Create .roster/config.yml in the current working directory using the
default template. Edit the file to fill in required fields (at minimum:
modules.issue_to_jira.jira_project, and identity if you have multiple
virtual employees).

Subsequent 'roster takeover' invocations in this directory will pick up
the file automatically.

Refuses to overwrite an existing file unless --force is passed.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			dir := filepath.Join(cwd, ".roster")
			path := filepath.Join(dir, "config.yml")

			if !force {
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("%s already exists (use --force to overwrite)", path)
				}
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(projcfg.Template), 0o644); err != nil {
				return err
			}
			fmt.Printf("✓ Wrote %s\n", path)

			// Project memory skeleton — 4 well-known files with
			// instructional placeholders so users see the structure
			// even before filling them in.
			memDir := filepath.Join(dir, memory.SubdirName)
			if err := os.MkdirAll(memDir, 0o755); err != nil {
				return err
			}
			memCreated := 0
			for _, f := range memory.Files {
				p := filepath.Join(memDir, f.Name)
				if _, err := os.Stat(p); err == nil {
					continue // don't overwrite existing memory files even with --force
				}
				body := memoryPlaceholder(f)
				if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
					return err
				}
				memCreated++
			}
			if memCreated > 0 {
				fmt.Printf("✓ Scaffolded %d memory files in %s/\n", memCreated, memDir)
			}

			fmt.Println()
			fmt.Println("Next steps:")
			fmt.Println("  1. Edit .roster/config.yml — fill in jira_project at minimum.")
			fmt.Println("  2. Edit .roster/memory/conventions.md — write 5 lines about how this project")
			fmt.Println("     does PRs / issues / style. AI modules read this on every call.")
			fmt.Println("  3. roster takeover --repo owner/name")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing config file")
	return cmd
}

// memoryPlaceholder renders the initial body for a single memory file.
// We write a short YAML-style header (so it's obvious to a human what
// the file is for) followed by an inline example. The example doubles
// as documentation so newcomers can see the format without leaving the
// editor.
func memoryPlaceholder(f memory.File) string {
	return fmt.Sprintf(`# %s

> %s
>
> This file is read by every AI-powered Roster module on every call,
> and inlined into the system prompt. Keep it concise and factual.
> Roster's PRINCIPLES.md explains why we use plain markdown here
> instead of a vector DB.

<!-- Replace this comment with real content. Example for conventions.md:

## PR
- No force-push to long-lived branches.
- One PR < 300 lines; split larger changes.
- Tests required for non-trivial logic.

## Style
- Functional Go preferred; no ORMs.
- Errors compared with errors.Is / errors.As, never string match.
-->
`, f.Heading, f.Description)
}
