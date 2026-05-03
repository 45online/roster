package budget

import (
	"math"
	"testing"
	"time"

	"github.com/45online/roster/internal/audit"
)

func TestSummarizeMonth_FiltersByMonth(t *testing.T) {
	may := time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC)
	apr := time.Date(2026, time.April, 30, 23, 59, 0, 0, time.UTC)
	entries := []audit.Entry{
		{Module: "a", Timestamp: may, CostUSD: 1.0, InputTokens: 100},
		{Module: "a", Timestamp: may, CostUSD: 2.0, OutputTokens: 50},
		{Module: "b", Timestamp: may, CostUSD: 0.5},
		{Module: "a", Timestamp: apr, CostUSD: 999.0}, // outside window
	}
	s := SummarizeMonth("foo/bar", entries, 2026, time.May)
	if s.CallCount != 3 {
		t.Errorf("CallCount = %d, want 3", s.CallCount)
	}
	if math.Abs(s.TotalUSD-3.5) > 0.001 {
		t.Errorf("TotalUSD = %v, want 3.5", s.TotalUSD)
	}
	if math.Abs(s.ByModule["a"]-3.0) > 0.001 {
		t.Errorf("ByModule[a] = %v, want 3.0", s.ByModule["a"])
	}
	if math.Abs(s.ByModule["b"]-0.5) > 0.001 {
		t.Errorf("ByModule[b] = %v, want 0.5", s.ByModule["b"])
	}
}

func TestSummarizeMonth_SkipsZeroCostNonAIEntries(t *testing.T) {
	// Module D / mechanical Module A entries have CostUSD=0 and 0 tokens —
	// they're real audit rows but shouldn't add to the call count.
	may := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{Module: "alert_aggregation", Timestamp: may},
		{Module: "issue_to_jira", Timestamp: may, CostUSD: 0.01, InputTokens: 100},
	}
	s := SummarizeMonth("x", entries, 2026, time.May)
	if s.CallCount != 1 {
		t.Errorf("expected 1 paid call, got %d", s.CallCount)
	}
}

func TestSummarizeCurrentMonth(t *testing.T) {
	now := time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC)
	entries := []audit.Entry{
		{Timestamp: now, CostUSD: 1.0},
	}
	s := SummarizeCurrentMonth("x", entries, now)
	if s.Month != time.May || s.Year != 2026 {
		t.Errorf("wrong month: %v %d", s.Month, s.Year)
	}
	if s.TotalUSD != 1.0 {
		t.Errorf("TotalUSD = %v, want 1.0", s.TotalUSD)
	}
}
