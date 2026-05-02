package audit

import (
	"os"
	"path/filepath"
)

// DefaultBaseDir returns the standard Roster state root: $HOME/.roster, or
// "./.roster" if the home directory is unavailable. Cursors, audit logs,
// and credentials all live underneath it.
func DefaultBaseDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".roster")
	}
	return ".roster"
}
