package usage

import (
	"testing"
	"time"
)

// Rule 8: a burn rate needs two samples from one session.
func TestRateNeedsTwoSamples(t *testing.T) {
	t0 := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if _, ok := Rate([]Sample{{At: t0, Tokens: 100}}); ok {
		t.Fatal("one sample must give no rate")
	}
	if _, ok := Rate([]Sample{{At: t0, Tokens: 100}, {At: t0, Tokens: 200}}); ok {
		t.Fatal("samples at one instant must give no rate")
	}
	got, ok := Rate([]Sample{{At: t0, Tokens: 100}, {At: t0.Add(2 * time.Minute), Tokens: 300}})
	if !ok || got != 100 {
		t.Fatalf("rate = %v, %v; want 100 per minute", got, ok)
	}
}
