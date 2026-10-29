package collect

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/store"
	"github.com/vizkid/burns/internal/usage"
)

var now = time.Date(2026, 10, 28, 4, 0, 0, 0, time.UTC)

type env struct {
	home, rollout, transcript string
	cfg                       config.Config
	db                        string
}

func setup(t *testing.T) env {
	t.Helper()
	home := t.TempDir()
	e := env{home: home, db: filepath.Join(t.TempDir(), "s.db"),
		rollout:    filepath.Join(home, ".codex-x", "sessions", "2026", "10", "20", "rollout-2026-10-20T00-00-00-t1.jsonl"),
		transcript: filepath.Join(home, ".claude-x", "projects", "-p", "s1.jsonl")}
	for _, p := range []string{e.rollout, e.transcript} {
		os.MkdirAll(filepath.Dir(p), 0o700)
	}
	cfgPath := filepath.Join(home, "accounts.json")
	os.WriteFile(cfgPath, []byte(`{"accounts":[
	 {"provider":"claude","name":"personal","home":".claude-x","kind":"subscription"},
	 {"provider":"codex","name":"work","home":".codex-x","kind":"subscription"}]}`), 0o600)
	cfg, err := config.Load(cfgPath, home)
	if err != nil {
		t.Fatal(err)
	}
	e.cfg = cfg
	return e
}

func (e env) scan(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(e.db)
	if err != nil {
		t.Fatal(err)
	}
	c := &Collector{Store: st, Config: e.cfg, Now: func() time.Time { return now }}
	if err := c.Scan(); err != nil {
		t.Fatal(err)
	}
	return st
}

func appendLines(t *testing.T, p, s string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(s)
	f.Close()
	os.Chtimes(p, now, now)
}

const rec = `{"timestamp":"2026-10-28T03:00:00Z","type":"token_usage_record","payload":{"thread_id":"t1","session_id":"t1","turn_id":"u1","response_id":"%s","usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":10}}}` + "\n"

func record(id string) string { return fmtS(rec, id) }

func totals(t *testing.T, st *store.Store) map[string]store.Totals {
	t.Helper()
	gs, err := st.Sessions(now.Add(-30 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]store.Totals{}
	for _, g := range gs {
		out[g.Provider+"/"+g.Account] = g.Totals
	}
	return out
}

func TestScanIngestsAndResumes(t *testing.T) {
	e := setup(t)
	appendLines(t, e.rollout, `{"timestamp":"2026-10-28T02:59:00Z","type":"session_meta","payload":{"id":"t1","session_id":"t1"}}`+"\n"+record("r1"))
	appendLines(t, e.transcript, `{"type":"user","promptId":"p1","sessionId":"s1","timestamp":"2026-10-28T03:00:00Z"}`+"\n"+
		`{"type":"assistant","requestId":"q1","sessionId":"s1","timestamp":"2026-10-28T03:00:01Z","message":{"id":"m1","usage":{"input_tokens":1,"output_tokens":2}}}`+"\n")
	st := e.scan(t)
	got := totals(t, st)
	if got["codex/work"].Requests != 1 || got["claude/personal"].Requests != 1 {
		t.Fatalf("first scan = %+v", got)
	}
	st.Close()

	// A restarted collector resumes from stored cursors and decoder state.
	appendLines(t, e.rollout, record("r2"))
	st = e.scan(t)
	defer st.Close()
	got = totals(t, st)
	if got["codex/work"].Requests != 2 || got["codex/work"].Input != 200 {
		t.Fatalf("after resume = %+v", got["codex/work"])
	}
	gs, _ := st.Sessions(now.Add(-time.Hour))
	for _, g := range gs {
		if g.Provider == usage.Codex && g.Session != "t1" {
			t.Fatalf("decoder state lost: session %q", g.Session)
		}
	}
}

func TestScanSkipsOldFiles(t *testing.T) {
	e := setup(t)
	appendLines(t, e.rollout, record("r1"))
	old := now.Add(-store.Retention - time.Hour)
	os.Chtimes(e.rollout, old, old)
	st := e.scan(t)
	defer st.Close()
	if got := totals(t, st); got["codex/work"].Requests != 0 {
		t.Fatalf("old file must be skipped: %+v", got)
	}
}

func TestRateLimitsReachTheStore(t *testing.T) {
	e := setup(t)
	reset := now.Add(48 * time.Hour).Unix()
	appendLines(t, e.rollout, fmtS(`{"timestamp":"2026-10-28T03:00:00Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"limit_id":"codex","primary":{"used_percent":28,"window_minutes":10080,"resets_at":%s},"credits":{"has_credits":true,"unlimited":false,"balance":"5"}}}}`+"\n", fmtI(reset)))
	st := e.scan(t)
	defer st.Close()
	ls, err := st.Limits()
	if err != nil || len(ls) != 1 || ls[0].Account != "work" || ls[0].Credits == nil {
		t.Fatalf("limits = %+v, %v", ls, err)
	}
	if v, ok := ls[0].Windows[10080].Value(now); !ok || v != 28 {
		t.Fatalf("weekly = %v %v", v, ok)
	}
}

func fmtS(format, a string) string {
	for i := 0; i+1 < len(format); i++ {
		if format[i] == '%' && format[i+1] == 's' {
			return format[:i] + a + format[i+2:]
		}
	}
	return format
}

func fmtI(v int64) string {
	if v == 0 {
		return "0"
	}
	s := ""
	for ; v > 0; v /= 10 {
		s = string(rune('0'+v%10)) + s
	}
	return s
}

// A resumed old file is decoded for state, but its expired rows are not stored.
func TestExpiredRowsInRecentFileAreDropped(t *testing.T) {
	e := setup(t)
	old := `{"timestamp":"2026-10-01T00:00:00Z","type":"token_usage_record","payload":{"thread_id":"t1","session_id":"t1","response_id":"old","usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"
	appendLines(t, e.rollout, old+record("r1"))
	st := e.scan(t)
	defer st.Close()
	if got := totals(t, st); got["codex/work"].Requests != 1 {
		t.Fatalf("requests = %d; want 1 (the 2026-10-01 row is past retention)", got["codex/work"].Requests)
	}
}

// An unchanged file costs a stat, not a database read.
func TestUnchangedFilesSkipTheStore(t *testing.T) {
	e := setup(t)
	appendLines(t, e.rollout, record("r1"))
	st, err := store.Open(e.db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c := &Collector{Store: st, Config: e.cfg, Now: func() time.Time { return now }}
	if err := c.Scan(); err != nil {
		t.Fatal(err)
	}
	loads, changed := c.cursorLoads, c.LastChange
	if err := c.Scan(); err != nil {
		t.Fatal(err)
	}
	if c.cursorLoads != loads {
		t.Fatalf("second scan loaded %d cursors; want 0", c.cursorLoads-loads)
	}
	if changed.IsZero() || !c.LastChange.Equal(changed) {
		t.Fatalf("LastChange = %v then %v; want set once", changed, c.LastChange)
	}
	appendLines(t, e.rollout, record("r2"))
	later := now.Add(time.Minute)
	c.Now = func() time.Time { return later }
	c.Scan()
	if !c.LastChange.Equal(later) {
		t.Fatalf("LastChange = %v; want %v after an append", c.LastChange, later)
	}
}

func TestIntervalAdapts(t *testing.T) {
	c := &Collector{Now: func() time.Time { return now }}
	if got := c.Interval(); got != SlowScan {
		t.Fatalf("idle interval = %v; want %v", got, SlowScan)
	}
	c.LastChange = now.Add(-time.Minute)
	if got := c.Interval(); got != FastScan {
		t.Fatalf("active interval = %v; want %v", got, FastScan)
	}
}

// A default home signed in to the same account is scanned as that account.
func TestScanReadsExtraHomes(t *testing.T) {
	e := setup(t)
	cfgPath := filepath.Join(e.home, "accounts.json")
	os.WriteFile(cfgPath, []byte(`{"accounts":[
	 {"provider":"claude","name":"personal","home":".claude-x","extra_homes":[".claude"],"kind":"subscription"}]}`), 0o600)
	cfg, err := config.Load(cfgPath, e.home)
	if err != nil {
		t.Fatal(err)
	}
	e.cfg = cfg
	extra := filepath.Join(e.home, ".claude", "projects", "-p", "s2.jsonl")
	os.MkdirAll(filepath.Dir(extra), 0o700)
	appendLines(t, extra, `{"type":"assistant","requestId":"q2","sessionId":"s2","timestamp":"2026-10-28T03:00:01Z","message":{"id":"m2","usage":{"input_tokens":1,"output_tokens":2}}}`+"\n")
	appendLines(t, e.transcript, `{"type":"assistant","requestId":"q1","sessionId":"s1","timestamp":"2026-10-28T03:00:01Z","message":{"id":"m1","usage":{"input_tokens":1,"output_tokens":2}}}`+"\n")
	st := e.scan(t)
	defer st.Close()
	gs, err := st.Sessions(now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	accounts := map[string]int64{}
	for _, g := range gs {
		accounts[g.Provider+"/"+g.Account] += g.Requests
	}
	if accounts["claude/personal"] != 2 || len(accounts) != 1 {
		t.Fatalf("requests by account = %v; want both homes under claude/personal", accounts)
	}
}
