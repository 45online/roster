package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/45online/roster/internal/audit"
)

// newRosterLogsCmd builds `roster logs <repo>`: print audit entries for a
// project. Supports tail-follow (-f), time-window (--since), and filter
// flags. Useful for debugging mid-incident.
func newRosterLogsCmd() *cobra.Command {
	var (
		follow         bool
		since          time.Duration
		moduleFilter   string
		statusFilter   string
		jsonOutput     bool
	)
	cmd := &cobra.Command{
		Use:   "logs <repo>",
		Short: "Tail audit log entries for a repo",
		Args:  cobra.ExactArgs(1),
		Long: longDesc(`
Print audit entries from ~/.roster/audit/<owner>_<repo>.jsonl. Without
flags, prints everything in chronological order.

  --since 30m            only entries within the last 30 minutes
  --module pr_review     only this module's entries
  --status error         only this outcome (success/partial/error/skipped)
  -f, --follow           keep printing new entries as they're appended
  --json                 emit raw JSONL instead of formatted text

The audit file is JSONL (one JSON object per line), so 'tail -f' on the
file works too — this command just adds filtering and pretty-printing.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]
			rec := audit.NewRecorder(audit.DefaultBaseDir())
			path := rec.PathForRepo(repo)

			if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("no audit file for %s (expected %s)", repo, path)
			}

			filter := func(e audit.Entry) bool {
				if moduleFilter != "" && e.Module != moduleFilter {
					return false
				}
				if statusFilter != "" && e.Status != statusFilter {
					return false
				}
				return true
			}

			// One-shot read.
			var entries []audit.Entry
			var err error
			if since > 0 {
				entries, err = rec.ReadSince(repo, time.Now().UTC().Add(-since))
			} else {
				entries, err = rec.ReadAll(repo)
			}
			if err != nil {
				return err
			}
			lastSize, err := emitEntries(os.Stdout, entries, filter, jsonOutput)
			if err != nil {
				return err
			}

			if !follow {
				return nil
			}

			// Follow loop: poll the file size, read new bytes, parse.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				cancel()
			}()

			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					lastSize, err = followOnce(os.Stdout, path, lastSize, filter, jsonOutput)
					if err != nil {
						return err
					}
				}
			}
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Keep printing new entries as they're appended")
	cmd.Flags().DurationVar(&since, "since", 0, "Only entries within this lookback window (e.g. 30m, 1h)")
	cmd.Flags().StringVar(&moduleFilter, "module", "", "Filter to one module (e.g. issue_to_jira)")
	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter to one status (success/partial/error/skipped)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit raw JSONL instead of formatted text")
	return cmd
}

// emitEntries pretty-prints (or JSON-prints) entries that pass filter.
// Returns the file size after the read so a follow loop can resume.
func emitEntries(w *os.File, entries []audit.Entry, filter func(audit.Entry) bool, jsonOutput bool) (int64, error) {
	for _, e := range entries {
		if !filter(e) {
			continue
		}
		if jsonOutput {
			b, _ := json.Marshal(e)
			fmt.Fprintln(w, string(b))
			continue
		}
		fmt.Fprintln(w, formatLogLine(e))
	}
	// Caller will set lastSize from os.Stat after this returns.
	return 0, nil
}

// followOnce stats the file, reads any newly-appended bytes, parses lines
// that pass the filter and prints them. Returns the new file size.
func followOnce(w *os.File, path string, lastSize int64, filter func(audit.Entry) bool, jsonOutput bool) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return lastSize, nil // file rotated away — quietly retry next tick
	}
	size := info.Size()
	if size <= lastSize {
		return lastSize, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return lastSize, nil
	}
	defer f.Close()
	if _, err := f.Seek(lastSize, 0); err != nil {
		return lastSize, nil
	}
	dec := json.NewDecoder(f)
	for dec.More() {
		var e audit.Entry
		if err := dec.Decode(&e); err != nil {
			break
		}
		if !filter(e) {
			continue
		}
		if jsonOutput {
			b, _ := json.Marshal(e)
			fmt.Fprintln(w, string(b))
			continue
		}
		fmt.Fprintln(w, formatLogLine(e))
	}
	return size, nil
}

// formatLogLine renders a single Entry as a one-line human view.
func formatLogLine(e audit.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s [%s/%s]", e.Timestamp.Format(time.RFC3339), e.Module, e.Status)
	if e.Repo != "" {
		fmt.Fprintf(&b, " %s", e.Repo)
	}
	if e.Issue > 0 {
		fmt.Fprintf(&b, "#%d", e.Issue)
	}
	if e.Actor != "" {
		fmt.Fprintf(&b, " by @%s", e.Actor)
	}
	if e.JiraKey != "" {
		fmt.Fprintf(&b, " → %s", e.JiraKey)
	}
	if e.AIExtracted {
		fmt.Fprintf(&b, " (AI)")
	}
	if e.DurationMS > 0 {
		fmt.Fprintf(&b, " [%dms]", e.DurationMS)
	}
	if e.Error != "" {
		fmt.Fprintf(&b, "  ! %s", truncateString(e.Error, 200))
	}
	return b.String()
}
