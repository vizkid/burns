package price

import (
	"testing"
	"time"
)

var at = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

// Every figure below is worked by hand from the provider's published rates, per million tokens.
func TestCost(t *testing.T) {
	const m = 1_000_000
	cases := []struct {
		name, basis, model, speed string
		tokens                    Tokens
		want                      float64 // dollars
	}{
		// $4 + $0.20 + $20 + $5 + $8.
		{"opus every rate", "anthropic", "claude-opus-5-5", "", Tokens{Input: m, Cached: m, Output: m, Write: m, WriteLong: m}, 37.2},
		// Fast doubles all of them, cache rates too.
		{"opus fast", "anthropic", "claude-opus-5-5", "fast", Tokens{Input: m, Cached: m, Output: m, Write: m, WriteLong: m}, 74.4},
		// A dated name is the same model.
		{"dated model", "anthropic", "claude-sonnet-4-5-20250929", "", Tokens{Input: m}, 3},
		// Under Haiku's 100,000 prompt limit.
		{"haiku short", "anthropic", "claude-haiku-5-5", "", Tokens{Input: m, Output: m}, 0.6},
		// Over it, the higher rates: $0.50 + $2.50.
		{"haiku long", "anthropic", "claude-haiku-5-5", "", Tokens{Input: m, Output: m, Long: true}, 3},
		// GPT-6.1 Sol: $2 + $0.10 + $10.
		{"sol", "openai", "gpt-6.1-sol", "", Tokens{Input: m, Cached: m, Output: m}, 12.1},
		// Long context: $4 + $0.20 + $15.
		{"sol long", "openai", "gpt-6.1-sol", "", Tokens{Input: m, Cached: m, Output: m, Long: true}, 19.2},
		// Ultrafast is six times standard: $12.
		{"sol ultrafast", "openai", "gpt-6.1-sol", "ultrafast", Tokens{Input: m}, 12},
		// Azure bills its cache writes: $10 + $1 + $50 + $12.50.
		{"azure astra", "azure-global", "gpt-6-astra", "", Tokens{Input: m, Cached: m, Output: m, Write: m}, 73.5},
	}
	for _, c := range cases {
		got, ok := Default().Cost(c.basis, c.model, c.speed, at, c.tokens)
		if !ok || got != int64(c.want*1e6+0.5) {
			t.Errorf("%s: %d, %v; want %.0f micros", c.name, got, ok, c.want*1e6)
		}
	}
}

// An unknown price is never zero: the caller counts the request as unpriced.
func TestUnknownIsNotPriced(t *testing.T) {
	for _, c := range [][3]string{{"openai", "codex-auto-review", ""}, {"anthropic", "<synthetic>", ""}, {"openai", "gpt-6.1-sol", "warp"},
		{"anthropic", "claude-sonnet-5-5", "fast"}, {"nobody", "gpt-6.1-sol", ""}, {"azure-global", "gpt-6-astra", "fast"}} {
		if _, ok := Default().Cost(c[0], c[1], c[2], at, Tokens{Input: 1000}); ok {
			t.Errorf("%v priced", c)
		}
	}
}

// A price change reprices nothing before it. Work before the first row uses the first row.
func TestRatesStartOnTheirDate(t *testing.T) {
	tb := Table{Rates: []Rate{{Basis: "b", Model: "m", From: "2026-10-01", Input: 1}, {Basis: "b", Model: "m", From: "2026-11-01", Input: 2}}}
	for _, c := range []struct {
		at   time.Time
		want int64
	}{{time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), 1_000_000}, {time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC), 1_000_000}, {at, 2_000_000}} {
		if got, ok := tb.Cost("b", "m", "", c.at, Tokens{Input: 1_000_000}); !ok || got != c.want {
			t.Errorf("%s: %d, %v", c.at, got, ok)
		}
	}
}

// Every row says where it came from.
func TestEveryRateHasItsSource(t *testing.T) {
	tb := Default()
	if len(tb.Rates) < 20 {
		t.Fatalf("rates = %d", len(tb.Rates))
	}
	for _, r := range tb.Rates {
		if r.Basis == "" || r.Model == "" || r.AsOf == "" || r.SourceURL == "" || r.Input <= 0 || r.Output <= 0 {
			t.Errorf("incomplete: %+v", r)
		}
	}
}

// A band is long on the provider's own limit. The store assigns bands (store.TestRollUpBandEdges).
func TestLongBands(t *testing.T) {
	tb := Default()
	if !tb.IsLong("anthropic", "claude-haiku-5-5", 1) || tb.IsLong("openai", "gpt-6.1-sol", 1) || !tb.IsLong("openai", "gpt-6.1-sol", 2) || tb.IsLong("anthropic", "claude-opus-5-5", 2) {
		t.Fatal("long bands")
	}
}
