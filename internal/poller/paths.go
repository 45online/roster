package poller

import (
	"os"
	"path/filepath"
)

// defaultBaseDir returns "$HOME/.roster", or "./.roster" if HOME is unset.
func defaultBaseDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".roster")
	}
	return ".roster"
}
