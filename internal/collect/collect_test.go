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
