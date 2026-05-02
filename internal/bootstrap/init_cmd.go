package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

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
			fmt.Println("  Next: edit jira_project, then run 'roster takeover'.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing config file")
	return cmd
}
