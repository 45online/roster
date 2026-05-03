package budget

import (
	"testing"
	"time"

	"github.com/45online/roster/internal/audit"
)

func TestThreshold_NilOrZeroLimit_NeverExceeds(t *testing.T) {
	now := time.Now().UTC()
	var nilT *Threshold
	if d := nilT.Check(now); d.Exceeded {
		t.Error("nil Threshold should not be exceeded")
	}
	zero := NewThreshold(nil, "x/y", 0, "stop")
	if d := zero.Check(now); d.Exceeded {
		t.Error("zero-limit Threshold should not be exceeded")
	}
}

func TestThreshold_BelowLimit_NotExceeded(t *testing.T) {
	dir := t.TempDir()
	rec := audit.NewRecorder(dir)
	now := time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC)
	rec.Record(audit.Entry{Repo: "x/y", Timestamp: now, CostUSD: 1.0})

	th := NewThreshold(rec, "x/y", 5.0, "stop")
	d := th.Check(now)
	if d.Exceeded {
		t.Errorf("$1 of $5 cap should not exceed, got %+v", d)
	}
	if d.MTDUSD != 1.0 || d.Limit != 5.0 {
		t.Errorf("MTD/Limit wrong: got %+v", d)
	}
}

func TestThreshold_AtOrAboveLimit_StopsOnDefault(t *testing.T) {
	dir := t.TempDir()
	rec := audit.NewRecorder(dir)
	now := time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC)
	rec.Record(audit.Entry{Repo: "x/y", Timestamp: now, CostUSD: 5.0})
	rec.Record(audit.Entry{Repo: "x/y", Timestamp: now, CostUSD: 5.01})

	cases := []struct {
		onExceed        string
		shouldStop      bool
		shouldDowngrade bool
	}{
		{"stop", true, false},
		{"", true, false},                // default → stop
		{"downgrade", false, true},       // explicit downgrade path
		{"unknown-mode", true, false},    // unrecognised → conservative stop
	}
	for _, tc := range cases {
		th := NewThreshold(rec, "x/y", 10.0, tc.onExceed)
		d := th.Check(now)
		if !d.Exceeded {
			t.Errorf("on_exceed=%q: expected Exceeded=true, got %+v", tc.onExceed, d)
		}
		if d.ShouldStop != tc.shouldStop {
			t.Errorf("on_exceed=%q: ShouldStop=%v, want %v", tc.onExceed, d.ShouldStop, tc.shouldStop)
		}
		if d.ShouldDowngrade != tc.shouldDowngrade {
			t.Errorf("on_exceed=%q: ShouldDowngrade=%v, want %v", tc.onExceed, d.ShouldDowngrade, tc.shouldDowngrade)
		}
		// Stop and Downgrade are mutually exclusive — never both true.
		if d.ShouldStop && d.ShouldDowngrade {
			t.Errorf("on_exceed=%q: stop and downgrade should be mutually exclusive", tc.onExceed)
		}
	}
}

func TestThreshold_CachesAcrossCheckCalls(t *testing.T) {
	dir := t.TempDir()
	rec := audit.NewRecorder(dir)
	now := time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC)
	rec.Record(audit.Entry{Repo: "x/y", Timestamp: now, CostUSD: 2.0})

	th := NewThreshold(rec, "x/y", 100.0, "stop")
	th.CacheTTL = 1 * time.Hour

	// First check populates the cache.
	if d := th.Check(now); d.MTDUSD != 2.0 {
		t.Fatalf("first check: %+v", d)
	}
	// Add another entry; second check within TTL should NOT see it yet.
	rec.Record(audit.Entry{Repo: "x/y", Timestamp: now, CostUSD: 3.0})
	if d := th.Check(now.Add(1 * time.Second)); d.MTDUSD != 2.0 {
		t.Errorf("cache should hold for TTL window, got %v", d.MTDUSD)
	}
	// Past TTL, it refreshes.
	if d := th.Check(now.Add(2 * time.Hour)); d.MTDUSD != 5.0 {
		t.Errorf("after TTL refresh: %v, want 5.0", d.MTDUSD)
	}
}

func TestThreshold_MarkSpend_BumpsCacheImmediately(t *testing.T) {
	dir := t.TempDir()
	rec := audit.NewRecorder(dir)
	now := time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC)
	rec.Record(audit.Entry{Repo: "x/y", Timestamp: now, CostUSD: 1.0})

	th := NewThreshold(rec, "x/y", 10.0, "stop")
	th.CacheTTL = 1 * time.Hour

	_ = th.Check(now)        // populate
	th.MarkSpend(2.5)        // pretend a module just spent $2.5
	d := th.Check(now.Add(1 * time.Second))
	if d.MTDUSD != 3.5 {
		t.Errorf("MarkSpend should add to cached MTD; got %v, want 3.5", d.MTDUSD)
	}
}
