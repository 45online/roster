package budget

import (
	"sync"
	"time"

	"github.com/45online/roster/internal/audit"
)

// Threshold guards a single repository's monthly Claude spend. The takeover
// daemon queries it before each event dispatch and stops itself when the
// configured cap is reached.
//
// Reads are served from a small TTL cache (default 30s) to avoid re-parsing
// the audit JSONL on every poll-tick event; this trades a few seconds of
// staleness for not making budget enforcement a hot path.
type Threshold struct {
	Repo       string
	MonthlyUSD float64       // 0 → unlimited (Threshold is effectively a no-op)
	OnExceed   string        // "stop" (only mode currently); "downgrade" reserved
	CacheTTL   time.Duration // 0 → 30s

	rec       *audit.Recorder
	mu        sync.RWMutex
	lastCheck time.Time
	cachedMTD float64
}

// Decision is what Threshold.Check returns.
type Decision struct {
	MTDUSD     float64 // current month-to-date spend
	Limit      float64 // configured cap (0 → unlimited)
	Exceeded   bool    // MTDUSD >= Limit, and Limit > 0
	ShouldStop bool    // Exceeded && OnExceed == "stop"
}

// NewThreshold returns a checker that consults the recorder for the given
// repo. When monthlyUSD <= 0 the returned Threshold is a no-op (safe to
// call Check on it).
func NewThreshold(rec *audit.Recorder, repo string, monthlyUSD float64, onExceed string) *Threshold {
	return &Threshold{
		Repo:       repo,
		MonthlyUSD: monthlyUSD,
		OnExceed:   onExceed,
		CacheTTL:   30 * time.Second,
		rec:        rec,
	}
}

// Check returns the current Decision, refreshing from audit if the cache
// is stale. Fails open: an audit read error keeps the previous cache,
// rather than freezing the whole daemon.
func (t *Threshold) Check(now time.Time) Decision {
	if t == nil || t.MonthlyUSD <= 0 {
		return Decision{}
	}
	now = now.UTC()
	if t.cacheTooOld(now) {
		t.refresh(now)
	}
	t.mu.RLock()
	cost := t.cachedMTD
	t.mu.RUnlock()

	d := Decision{MTDUSD: cost, Limit: t.MonthlyUSD}
	if cost >= t.MonthlyUSD {
		d.Exceeded = true
		// Default behaviour when on_exceed is unset: stop. We err on the
		// side of preventing runaway spend.
		if t.OnExceed == "" || t.OnExceed == "stop" {
			d.ShouldStop = true
		}
	}
	return d
}

// MarkSpend lets a fast path bump the cached MTD without re-reading the
// audit file (saves a stat+scan on every event). It's optional — the
// 30s TTL refresh would catch up anyway.
func (t *Threshold) MarkSpend(usd float64) {
	if t == nil || usd <= 0 {
		return
	}
	t.mu.Lock()
	t.cachedMTD += usd
	t.mu.Unlock()
}

func (t *Threshold) cacheTooOld(now time.Time) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.lastCheck.IsZero() {
		return true
	}
	ttl := t.CacheTTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return now.Sub(t.lastCheck) >= ttl
}

func (t *Threshold) refresh(now time.Time) {
	if t.rec == nil {
		return
	}
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	entries, err := t.rec.ReadSince(t.Repo, monthStart)
	if err != nil {
		return // fail open
	}
	s := SummarizeMonth(t.Repo, entries, now.Year(), now.Month())

	t.mu.Lock()
	t.cachedMTD = s.TotalUSD
	t.lastCheck = now
	t.mu.Unlock()
}
