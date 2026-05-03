package budget

import (
	"math"
	"testing"

	"github.com/45online/roster/internal/api"
)

func TestModelPrice_KnownFamilies(t *testing.T) {
	cases := []struct {
		model    string
		wantIn   float64
		wantOut  float64
		wantHit  bool
	}{
		{"claude-haiku-4-5-20251001", 1.00, 5.00, true},
		{"claude-sonnet-4-6-20250514", 3.00, 15.00, true},
		{"claude-opus-4-7", 15.00, 75.00, true},
		{"gpt-4o", 2.50, 10.00, true},
		{"gpt-4o-mini", 0.15, 0.60, true},
		{"gpt-4.1-mini", 0.40, 1.60, true}, // longest-prefix wins over gpt-4
		{"deepseek-chat", 0.27, 1.10, true},
		{"deepseek-reasoner", 0.55, 2.19, true},
		{"gemini-2.5-flash", 0.075, 0.30, true},
		{"gemini-2.5-pro", 1.25, 10.00, true},
		{"grok-3", 2.00, 10.00, true},
		{"unknown-model-x", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, tc := range cases {
		p, ok := ModelPrice(tc.model)
		if ok != tc.wantHit {
			t.Errorf("ModelPrice(%q): hit=%v, want %v", tc.model, ok, tc.wantHit)
			continue
		}
		if !ok {
			continue
		}
		if p.InputUSD != tc.wantIn {
			t.Errorf("ModelPrice(%q): input=%v, want %v", tc.model, p.InputUSD, tc.wantIn)
		}
		if p.OutputUSD != tc.wantOut {
			t.Errorf("ModelPrice(%q): output=%v, want %v", tc.model, p.OutputUSD, tc.wantOut)
		}
	}
}

func TestModelPrice_DerivedCacheRates(t *testing.T) {
	p, _ := ModelPrice("claude-sonnet-4-6")
	if p.CacheWriteUSD != p.InputUSD*1.25 {
		t.Errorf("CacheWriteUSD = %v, want %v (1.25x input)", p.CacheWriteUSD, p.InputUSD*1.25)
	}
	if p.CacheReadUSD != p.InputUSD*0.10 {
		t.Errorf("CacheReadUSD = %v, want %v (0.10x input)", p.CacheReadUSD, p.InputUSD*0.10)
	}
}

func TestCostForUsage(t *testing.T) {
	// Sonnet: $3 in, $15 out per million.
	usage := api.Usage{
		InputTokens:              1_000_000, // $3.00
		OutputTokens:             100_000,   // $1.50
		CacheCreationInputTokens: 500_000,   // $1.875 (3 * 1.25 / 2)
		CacheReadInputTokens:     2_000_000, // $0.60 (3 * 0.10 * 2)
	}
	got := CostForUsage("claude-sonnet-4-6", usage)
	want := 3.00 + 1.50 + 1.875 + 0.60
	if math.Abs(got-want) > 0.001 {
		t.Errorf("CostForUsage = %v, want %v", got, want)
	}
}

func TestCostForUsage_UnknownModelReturnsZero(t *testing.T) {
	got := CostForUsage("totally-made-up-model", api.Usage{InputTokens: 1_000_000})
	if got != 0 {
		t.Errorf("unknown model should yield 0, got %v", got)
	}
}
