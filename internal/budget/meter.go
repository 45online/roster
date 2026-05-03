package budget

import (
	"time"

	"github.com/45online/roster/internal/audit"
)

// MonthlySummary aggregates spend over a calendar month for one repo.
type MonthlySummary struct {
	Repo       string
	Year       int
	Month      time.Month
	TotalUSD   float64
	ByModule   map[string]float64
	CallCount  int
	TokensIn   int
	TokensOut  int
	TokensCache int
}

// SummarizeMonth walks audit entries and accumulates spend for the given
// month (in the entry timestamp's UTC year+month). Entries without a Cost
// or whose timestamp falls outside the window are ignored.
func SummarizeMonth(repo string, entries []audit.Entry, year int, month time.Month) MonthlySummary {
	s := MonthlySummary{
		Repo:     repo,
		Year:     year,
		Month:    month,
		ByModule: map[string]float64{},
	}
	for _, e := range entries {
		ts := e.Timestamp.UTC()
		if ts.Year() != year || ts.Month() != month {
			continue
		}
		if e.CostUSD <= 0 && e.InputTokens == 0 && e.OutputTokens == 0 {
			continue
		}
		s.TotalUSD += e.CostUSD
		s.ByModule[e.Module] += e.CostUSD
		s.CallCount++
		s.TokensIn += e.InputTokens
		s.TokensOut += e.OutputTokens
		s.TokensCache += e.CacheReadTokens + e.CacheCreateTokens
	}
	return s
}

// SummarizeCurrentMonth is a convenience: the calendar month of `now`.
func SummarizeCurrentMonth(repo string, entries []audit.Entry, now time.Time) MonthlySummary {
	t := now.UTC()
	return SummarizeMonth(repo, entries, t.Year(), t.Month())
}
