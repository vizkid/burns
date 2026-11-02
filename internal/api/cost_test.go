package api

import (
	"testing"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/store"
	"github.com/vizkid/burns/internal/usage"
)

type costDoc struct {
	Hours  []CostHour     `json:"hours"`
	Billed []store.Billed `json:"billed"`
}

// A priced request has its list price. A model with no price is counted, not free.
func TestCostRoute(t *testing.T) {
	ts, s := server(t)
	at := now.Add(-time.Hour)
	if err := s.Store.AddRequests([]usage.Request{
		{Provider: usage.Claude, Key: "r1", Account: "personal", Session: "s", Model: "claude-opus-5-5", Input: 1_000_000, Output: 1_000_000, Source: usage.SourceOTel, At: at},
		{Provider: usage.Codex, Key: "r2", Account: "azure", Session: "s", Model: "gpt-6-astra", Input: 100_000, CacheWrite: 100_000, Source: usage.SourceRollout, At: at},
		{Provider: usage.Codex, Key: "r3", Account: "azure", Session: "s", Model: "codex-auto-review", Input: 500, Source: usage.SourceRollout, At: at},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.RollUp(now.Add(-store.Retention), now); err != nil {
		t.Fatal(err)
	}
	var doc costDoc
	getJSON(t, ts.URL+"/usage/cost", &doc)
	if len(doc.Hours) != 3 {
		t.Fatalf("hours = %+v", doc.Hours)
	}
	got := map[string]CostHour{}
	for _, h := range doc.Hours {
		got[h.Model] = h
	}
	if got["claude-opus-5-5"].Micros != 24_000_000 { // $4 + $20
		t.Errorf("opus = %+v", got["claude-opus-5-5"])
	}
	if got["gpt-6-astra"].Micros != 2_250_000 { // Azure: $1 + $1.25 write
		t.Errorf("astra = %+v", got["gpt-6-astra"])
	}
	if a := got["codex-auto-review"]; a.Micros != 0 || a.Unpriced != 1 || a.Tokens != 500 {
		t.Errorf("unpriced = %+v", a)
	}
}

// Falls add up, a reload is not negative spend, and the account's price turns credits into money.
func TestCreditSpend(t *testing.T) {
	a := config.Account{Provider: "codex", Name: "alpha", CreditPrice: &config.CreditPrice{Currency: "SGD", Amount: 0.05}}
	b := []store.CreditBalance{{Balance: 1000}, {Balance: 800}, {Balance: 5000}, {Balance: 4900, ObservedAt: now}}
	got, ok := creditSpend(a, b)
	if h := got.History; len(h) != 3 || h[0].UsedMinor != 0 || h[1].UsedMinor != 1000 || h[2].UsedMinor != 1500 {
		t.Fatalf("history = %+v", h)
	}
	if !ok || got.Currency != "SGD" || got.UsedMinor != 1500 || got.Exponent != 2 || !got.ObservedAt.Equal(now) { // 300 credits at 5 cents
		t.Fatalf("spend = %+v", got)
	}
	a.CreditPrice = nil
	if got, _ = creditSpend(a, b); got.Currency != "credits" || got.UsedMinor != 30000 {
		t.Fatalf("no price = %+v", got)
	}
}

// One reading shows no fall, so it is no reading, not zero spend.
func TestCreditSpendNeedsTwoReadings(t *testing.T) {
	if _, ok := creditSpend(config.Account{}, []store.CreditBalance{{Balance: 10}}); ok {
		t.Fatal("one reading gave spend")
	}
}
