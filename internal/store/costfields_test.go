package store

import (
	"testing"
	"time"

	"github.com/vizkid/burns/internal/usage"
)

// One request can arrive from telemetry and from a transcript. Telemetry's tokens win, and the
// transcript still supplies the cache lifetime and speed that older telemetry leaves out.
func TestRequestKeepsCacheLifetimeAndSpeed(t *testing.T) {
	s := open(t)
	at := time.Unix(1793160000, 0).UTC()
	if err := s.AddRequests([]usage.Request{{Provider: usage.Claude, Key: "r1", Session: "s1", CacheWrite: 900,
		Source: usage.SourceOTel, At: at}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRequests([]usage.Request{{Provider: usage.Claude, Key: "r1", Session: "s1", CacheWrite: 900, CacheWrite1h: 600,
		Speed: "fast", Source: usage.SourceTranscript, At: at}}); err != nil {
		t.Fatal(err)
	}
	var oneHour int64
	var speed, source string
	if err := s.rdb.QueryRow(`SELECT cache_write_1h, speed, source FROM request WHERE key = 'r1'`).Scan(&oneHour, &speed, &source); err != nil {
		t.Fatal(err)
	}
	if oneHour != 600 || speed != "fast" || source != usage.SourceOTel {
		t.Fatalf("1h %d, speed %q, source %q", oneHour, speed, source)
	}
}

// Codex usually sends a null balance. That says nothing about the balance, so the last real one stays.
func TestNullBalanceKeepsTheLastBalance(t *testing.T) {
	s := open(t)
	t0 := time.Unix(1793160000, 0).UTC()
	s.SetCredits(usage.Codex, "work", "codex", Credits{HasCredits: true, Balance: "2052.5", ObservedAt: t0})
	s.SetCredits(usage.Codex, "work", "codex", Credits{HasCredits: true, Balance: "", ObservedAt: t0.Add(time.Hour)})
	limits, err := s.Limits()
	if err != nil {
		t.Fatal(err)
	}
	if len(limits) != 1 || limits[0].Credits == nil || limits[0].Credits.Balance != "2052.5" {
		t.Fatalf("limits = %+v", limits)
	}
}

// Each change of balance is kept in time order, and a repeat or a null adds nothing. Spend is
// worked out from these, and a rise is a reload.
func TestBalanceHistoryKeepsChanges(t *testing.T) {
	s := open(t)
	t0 := time.Unix(1793160000, 0).UTC()
	for i, b := range []string{"100", "100", "", "60", "500"} {
		if err := s.SetCredits(usage.Codex, "work", "codex", Credits{HasCredits: true, Balance: b, ObservedAt: t0.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.CreditBalances(usage.Codex, "work", "codex", t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Balance != 100 || got[1].Balance != 60 || got[2].Balance != 500 || !got[1].ObservedAt.Equal(t0.Add(3*time.Hour)) {
		t.Fatalf("balances = %+v", got)
	}
}

// A provider reports a month's total, so its changes are kept. A repeat adds nothing, and the
// history starts from the last amount before the window asked for.
func TestBilledHistory(t *testing.T) {
	s := open(t)
	t0 := time.Unix(1793160000, 0).UTC()
	for i, used := range []int64{100, 100, 250, 900} {
		if err := s.SetBilled(Billed{Provider: "claude", Account: "a", Source: "x", Currency: "SGD", UsedMinor: used, Exponent: 2,
			ObservedAt: t0.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.BilledHistory("claude", "a", t0.Add(150*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].UsedMinor != 250 || got[1].UsedMinor != 900 {
		t.Fatalf("history = %+v", got)
	}
}

// A store from before this migration keeps its rows: no lifetime split is known, and the speed is standard.
func TestOldRequestsMigrate(t *testing.T) {
	s := open(t)
	if _, err := s.db.Exec(`INSERT INTO request (provider, key, account, session, agent, turn, model, input, cached_input, cache_write,
		output, reasoning, compaction, block, rex_session, source, at) VALUES ('claude','old','a','s','','','m',1,2,3,4,0,0,'','','otel',1)`); err != nil {
		t.Fatal(err)
	}
	var oneHour int64
	var speed string
	if err := s.rdb.QueryRow(`SELECT cache_write_1h, speed FROM request WHERE key = 'old'`).Scan(&oneHour, &speed); err != nil {
		t.Fatal(err)
	}
	if oneHour != 0 || speed != "" {
		t.Fatalf("1h %d, speed %q", oneHour, speed)
	}
}
