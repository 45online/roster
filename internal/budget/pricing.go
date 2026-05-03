// Package budget tracks Claude API spend per project, derived from the
// audit log. Every module that calls Claude is expected to record token
// usage on its audit Entry; this package does the price lookup and the
// per-month aggregation.
package budget

import (
	"strings"

	"github.com/45online/roster/internal/api"
)

// Price is a per-million-tokens dollar rate for a model.
type Price struct {
	// InputUSD is the rate for plain input tokens.
	InputUSD float64
	// OutputUSD is the rate for output tokens.
	OutputUSD float64
	// CacheWriteUSD is the rate for cache_creation_input_tokens.
	// Defaults to InputUSD * 1.25 if zero.
	CacheWriteUSD float64
	// CacheReadUSD is the rate for cache_read_input_tokens.
	// Defaults to InputUSD * 0.10 if zero.
	CacheReadUSD float64
}

// pricesByModel maps a model identifier (or substring prefix) to its Price.
// Keys are matched longest-first by ModelPrice; this lets dated variants
// like "claude-haiku-4-5-20251001" fall back to the family rate.
//
// Prices are USD per million tokens, last refreshed early 2026. Cache
// rates default to InputUSD * 1.25 / 0.10 if zero (Anthropic-style).
// For OpenAI-compatible providers without a cache concept, we leave the
// derived fields at the same fallback — the rate is academic when no
// cache tokens are reported.
var pricesByModel = map[string]Price{
	// ── Anthropic (Claude 4.x family) ────────────────────────────
	"claude-opus-4":   {InputUSD: 15.00, OutputUSD: 75.00},
	"claude-sonnet-4": {InputUSD: 3.00, OutputUSD: 15.00},
	"claude-haiku-4":  {InputUSD: 1.00, OutputUSD: 5.00},

	// ── OpenAI ────────────────────────────────────────────────────
	"gpt-4o-mini": {InputUSD: 0.15, OutputUSD: 0.60},
	"gpt-4o":      {InputUSD: 2.50, OutputUSD: 10.00},
	"gpt-4.1-mini": {InputUSD: 0.40, OutputUSD: 1.60},
	"gpt-4.1":     {InputUSD: 2.00, OutputUSD: 8.00},

	// ── DeepSeek (cheapest among capable open models) ────────────
	"deepseek-chat":     {InputUSD: 0.27, OutputUSD: 1.10},
	"deepseek-reasoner": {InputUSD: 0.55, OutputUSD: 2.19},

	// ── Google Gemini (OpenAI-compat endpoint) ───────────────────
	"gemini-2.5-flash": {InputUSD: 0.075, OutputUSD: 0.30},
	"gemini-2.0-flash": {InputUSD: 0.10, OutputUSD: 0.40},
	"gemini-2.5-pro":   {InputUSD: 1.25, OutputUSD: 10.00},

	// ── xAI Grok ─────────────────────────────────────────────────
	"grok-3": {InputUSD: 2.00, OutputUSD: 10.00},
	"grok-2": {InputUSD: 2.00, OutputUSD: 10.00},
}

// ModelPrice returns the Price entry for a model name, with caching rates
// derived from input rate when not explicitly provided. Returns the zero
// Price{} (and ok=false) if no match — callers should treat that as an
// unknown model whose cost is zero (rather than guessing).
func ModelPrice(model string) (Price, bool) {
	if model == "" {
		return Price{}, false
	}
	model = strings.ToLower(model)

	// Longest-prefix match: order keys by length desc.
	type kv struct {
		k string
		v Price
	}
	candidates := make([]kv, 0, len(pricesByModel))
	for k, v := range pricesByModel {
		candidates = append(candidates, kv{k, v})
	}
	// Bubble-sort by len desc — small N (a handful of families).
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if len(candidates[j].k) > len(candidates[i].k) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	for _, c := range candidates {
		if strings.HasPrefix(model, c.k) {
			p := c.v
			if p.CacheWriteUSD == 0 {
				p.CacheWriteUSD = p.InputUSD * 1.25
			}
			if p.CacheReadUSD == 0 {
				p.CacheReadUSD = p.InputUSD * 0.10
			}
			return p, true
		}
	}
	return Price{}, false
}

// CostForUsage computes the dollar cost of a single Claude call given the
// model and the Usage block from the response. Unknown models return 0
// (rather than a guess) so an out-of-date pricing table never overstates.
func CostForUsage(model string, u api.Usage) float64 {
	p, ok := ModelPrice(model)
	if !ok {
		return 0
	}
	return CostForTokens(p,
		u.InputTokens,
		u.OutputTokens,
		u.CacheCreationInputTokens,
		u.CacheReadInputTokens,
	)
}

// CostForTokens applies a Price to raw token counts.
func CostForTokens(p Price, in, out, cacheCreate, cacheRead int) float64 {
	const million = 1_000_000.0
	cost := 0.0
	cost += float64(in) * p.InputUSD / million
	cost += float64(out) * p.OutputUSD / million
	cost += float64(cacheCreate) * p.CacheWriteUSD / million
	cost += float64(cacheRead) * p.CacheReadUSD / million
	return cost
}
