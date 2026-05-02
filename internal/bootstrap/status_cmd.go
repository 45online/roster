package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/45online/roster/internal/audit"
	"github.com/45online/roster/internal/creds"
)

// newRosterStatusCmd builds `roster status`: a single-shot dashboard that
// shows credential state, every repo Roster has touched (inferred from
// cursor files + audit files), and a 24h activity summary per repo.
func newRosterStatusCmd() *cobra.Command {
	var (
		jsonOutput bool
		windowH    int
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show credential, project, and recent-activity status",
		Long: longDesc(`
Print a single dashboard:
  - Which provider credentials are stored (~/.roster/credentials.json)
  - Which repos Roster is tracking (cursor files in ~/.roster/cursors/
    and audit files in ~/.roster/audit/)
  - For each repo, a per-status / per-module count over the lookback
    window, plus the latest event and the latest error if any

Use --json for a machine-readable form.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := audit.DefaultBaseDir()
			window := time.Duration(windowH) * time.Hour
			if windowH <= 0 {
				window = 24 * time.Hour
			}
			report, err := buildStatusReport(base, window)
			if err != nil {
				return err
			}
			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			renderStatusReport(os.Stdout, report)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON instead of human text")
	cmd.Flags().IntVar(&windowH, "window-hours", 24, "Lookback window for activity summary (hours)")
	return cmd
}

// statusReport is the structured dashboard.
type statusReport struct {
	BaseDir     string                 `json:"base_dir"`
	GeneratedAt time.Time              `json:"generated_at"`
	WindowHours int                    `json:"window_hours"`
	Credentials map[string]bool        `json:"credentials"`
	Projects    []projectStatus        `json:"projects"`
}

type projectStatus struct {
	Repo            string         `json:"repo"`
	HasCursor       bool           `json:"has_cursor"`
	LastEventID     string         `json:"last_event_id,omitempty"`
	LastPolledAt    *time.Time     `json:"last_polled_at,omitempty"`
	HasAuditFile    bool           `json:"has_audit_file"`
	WindowSummary   audit.Summary  `json:"window_summary"`
	LatestActivity  *time.Time     `json:"latest_activity,omitempty"`
}

// cursorFile mirrors poller.Cursor's persisted JSON. We don't import
// internal/poller here to avoid a forward dep from bootstrap.
type cursorFile struct {
	Repo         string    `json:"repo"`
	LastEventID  string    `json:"last_event_id"`
	LastPolledAt time.Time `json:"last_polled_at"`
}

func buildStatusReport(baseDir string, window time.Duration) (*statusReport, error) {
	now := time.Now().UTC()
	since := now.Add(-window)

	r := &statusReport{
		BaseDir:     baseDir,
		GeneratedAt: now,
		WindowHours: int(window / time.Hour),
		Credentials: map[string]bool{},
	}

	// Credentials.
	store, _ := creds.Load(creds.Path(baseDir))
	if store == nil {
		store = &creds.Store{}
	}
	for _, p := range []string{"github", "jira", "slack", "claude"} {
		r.Credentials[p] = store.Has(p)
	}

	// Inventory: union of audit files and cursor files.
	repos := map[string]struct{}{}
	rec := audit.NewRecorder(baseDir)
	if list, err := rec.ListRepos(); err == nil {
		for _, name := range list {
			repos[name] = struct{}{}
		}
	}
	cursorDir := filepath.Join(baseDir, "cursors")
	if entries, err := os.ReadDir(cursorDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			base := strings.TrimSuffix(e.Name(), ".json")
			if i := strings.IndexByte(base, '_'); i > 0 {
				repos[base[:i]+"/"+base[i+1:]] = struct{}{}
			}
		}
	}

	// Per-repo detail.
	names := make([]string, 0, len(repos))
	for n := range repos {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		ps := projectStatus{Repo: name}

		// Cursor.
		cursorPath := filepath.Join(cursorDir, strings.ReplaceAll(name, "/", "_")+".json")
		if data, err := os.ReadFile(cursorPath); err == nil {
			ps.HasCursor = true
			var c cursorFile
			if json.Unmarshal(data, &c) == nil {
				ps.LastEventID = c.LastEventID
				if !c.LastPolledAt.IsZero() {
					t := c.LastPolledAt
					ps.LastPolledAt = &t
				}
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			// soft-fail
		}

		// Audit.
		entries, err := rec.ReadSince(name, since)
		if err == nil {
			ps.HasAuditFile = true
			ps.WindowSummary = audit.Summarize(name, entries)
			if ps.WindowSummary.LatestEntry != nil {
				t := ps.WindowSummary.LatestEntry.Timestamp
				ps.LatestActivity = &t
			}
		}

		r.Projects = append(r.Projects, ps)
	}

	return r, nil
}

func renderStatusReport(w *os.File, r *statusReport) {
	fmt.Fprintf(w, "Roster status — %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Base dir: %s\n\n", r.BaseDir)

	// Credentials block.
	fmt.Fprintln(w, "Credentials:")
	for _, p := range []string{"github", "jira", "slack", "claude"} {
		mark := "✗ not set"
		if r.Credentials[p] {
			mark = "✓ configured"
		}
		fmt.Fprintf(w, "  %-7s %s\n", p, mark)
	}

	// Projects block.
	if len(r.Projects) == 0 {
		fmt.Fprintf(w, "\nProjects: (none — run 'roster takeover' or 'roster sync-issue' first)\n")
		return
	}
	fmt.Fprintf(w, "\nProjects (%d, last %dh):\n", len(r.Projects), r.WindowHours)
	for _, p := range r.Projects {
		fmt.Fprintf(w, "\n  %s\n", p.Repo)
		if p.HasCursor {
			polled := "never"
			if p.LastPolledAt != nil {
				polled = humanAgo(r.GeneratedAt, *p.LastPolledAt)
			}
			fmt.Fprintf(w, "    cursor       last polled %s, event_id=%s\n", polled, p.LastEventID)
		} else {
			fmt.Fprintf(w, "    cursor       (none — manual-only or pre-takeover)\n")
		}
		if !p.HasAuditFile {
			fmt.Fprintf(w, "    audit        (no entries)\n")
			continue
		}
		s := p.WindowSummary
		if s.Total == 0 {
			fmt.Fprintf(w, "    audit        no activity in last %dh\n", r.WindowHours)
			continue
		}
		fmt.Fprintf(w, "    audit        %d events: %d success, %d partial, %d error, %d skipped\n",
			s.Total, s.Success, s.Partial, s.Errors, s.Skipped)
		modules := make([]string, 0, len(s.ByModule))
		for k := range s.ByModule {
			modules = append(modules, k)
		}
		sort.Strings(modules)
		parts := make([]string, 0, len(modules))
		for _, m := range modules {
			parts = append(parts, fmt.Sprintf("%s=%d", m, s.ByModule[m]))
		}
		fmt.Fprintf(w, "    by module    %s\n", strings.Join(parts, ", "))
		if p.LatestActivity != nil {
			fmt.Fprintf(w, "    last event   %s\n", humanAgo(r.GeneratedAt, *p.LatestActivity))
		}
		if s.Errors > 0 {
			fmt.Fprintf(w, "    last error   %s — %s\n",
				humanAgo(r.GeneratedAt, s.LatestErrorTime),
				truncateString(s.LatestErrorMsg, 120))
		}
	}
}

func humanAgo(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
