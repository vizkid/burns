package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/quota"
	"github.com/vizkid/burns/internal/tail"
	"github.com/vizkid/burns/internal/usage"
)

var t0 = time.Date(2026, 10, 28, 4, 0, 0, 0, time.UTC)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "smithers.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func i64(v int64) *int64 { return &v }

func req(key, source string, out int64) usage.Request {
	return usage.Request{Provider: usage.Claude, Key: key, Account: "personal", Session: "s1", Turn: "p1",
		Input: 1, CachedInput: 100, Output: out, Source: source, At: t0}
}

func one(t *testing.T) func([]Group, error) Group {
	return func(gs []Group, err error) Group {
		t.Helper()
		if err != nil || len(gs) != 1 {
			t.Fatalf("groups = %+v, err %v; want one", gs, err)
		}
		return gs[0]
	}
}

// Re-ingesting the same records changes nothing.
func TestReingestIsIdempotent(t *testing.T) {
	s := open(t)
	rs := []usage.Request{req("r1", usage.SourceTranscript, 10), req("r2", usage.SourceTranscript, 5)}
	for i := 0; i < 3; i++ {
		if err := s.AddRequests(rs); err != nil {
			t.Fatal(err)
		}
	}
	g := one(t)(s.Sessions(t0.Add(-time.Hour)))
	if g.Requests != 2 || g.Output != 15 {
		t.Fatalf("totals = %+v", g.Totals)
	}
	if g.CostMicros != nil || g.UnknownCost != 2 {
		t.Fatalf("rule 9: no known cost must read null, not 0: %+v", g.Totals)
	}
}

// OTel and transcript copies of one request are stored once; OTel's cost is kept.
func TestOTelAndBackfillStoreOnce(t *testing.T) {
	s := open(t)
	o := req("r1", usage.SourceOTel, 10)
	o.CostMicros, o.Block = i64(4553), "block:b1"
	if err := s.AddRequests([]usage.Request{req("r1", usage.SourceTranscript, 10)}); err != nil {
		t.Fatal(err)
	}
	s.AddRequests([]usage.Request{o})
	s.AddRequests([]usage.Request{req("r1", usage.SourceTranscript, 10)})
	g := one(t)(s.Sessions(t0.Add(-time.Hour)))
	if g.Requests != 1 || g.CostMicros == nil || *g.CostMicros != 4553 || g.UnknownCost != 0 {
		t.Fatalf("totals = %+v", g.Totals)
	}
	b := one(t)(s.Blocks(t0.Add(-time.Hour)))
	if b.Block != "block:b1" {
		t.Fatalf("block = %q", b.Block)
	}
}

func TestCompactionsDedupeAndFlag(t *testing.T) {
	s := open(t)
	cr := req("rc", usage.SourceOTel, 1074)
	s.AddRequests([]usage.Request{req("r1", usage.SourceOTel, 10), cr})
	otel := usage.Compaction{Provider: usage.Claude, Account: "personal", Session: "s1", Turn: "p2", Trigger: "manual",
		PreTokens: i64(35988), Source: usage.SourceOTel, At: t0.Add(13596 * time.Millisecond)}
	tr := otel
	tr.Source, tr.PreTokens, tr.At = usage.SourceTranscript, i64(66481), t0.Add(13532*time.Millisecond)
	later := tr
	later.At = t0.Add(time.Hour)
	for _, batch := range [][]usage.Compaction{{otel}, {tr}, {otel}, {later}} {
		if err := s.AddCompactions(batch); err != nil {
			t.Fatal(err)
		}
	}
	codexReq := usage.Request{Provider: usage.Codex, Key: "t1/rc", Account: "work", Session: "t1", Input: 5, Output: 5,
		Source: usage.SourceRollout, At: t0}
	s.AddRequests([]usage.Request{codexReq})
	cc := usage.Compaction{Provider: usage.Codex, Key: "t1/rc", RequestKey: "t1/rc", Account: "work", Session: "t1",
		Source: usage.SourceRollout, At: t0}
	s.AddCompactions([]usage.Compaction{cc})
	s.AddCompactions([]usage.Compaction{cc})

	gs, err := s.Sessions(t0.Add(-time.Hour))
	if err != nil || len(gs) != 2 {
		t.Fatalf("sessions = %+v, %v", gs, err)
	}
	for _, g := range gs {
		switch g.Provider {
		case usage.Claude:
			if g.Compactions != 2 {
				t.Errorf("claude compactions = %d; want 2 (OTel and boundary are one)", g.Compactions)
			}
		case usage.Codex:
			if g.Compactions != 1 || g.CompactionTokens != 10 {
				t.Errorf("codex = %+v; want 1 compaction of 10 tokens", g.Totals)
			}
		}
	}
}

// A Claude compact request is flagged at ingest; its tokens count as compaction tokens.
func TestFlaggedRequestCountsAsCompactionTokens(t *testing.T) {
	s := open(t)
	cr := req("rc", usage.SourceOTel, 1074)
	cr.Compaction = true
	s.AddRequests([]usage.Request{cr, req("r1", usage.SourceOTel, 10)})
	g := one(t)(s.Sessions(t0.Add(-time.Hour)))
	if g.CompactionTokens != cr.Total() {
		t.Fatalf("compaction tokens = %d; want %d", g.CompactionTokens, cr.Total())
	}
}

func TestTurnsAndUnattributedBlocks(t *testing.T) {
	s := open(t)
	a, b := req("r1", usage.SourceTranscript, 10), req("r2", usage.SourceTranscript, 20)
	b.Turn = "p2"
	s.AddRequests([]usage.Request{a, b})
	ts, err := s.Turns(usage.Claude, "s1")
	if err != nil || len(ts) != 2 || ts[0].Turn != "p1" || ts[1].Output != 20 {
		t.Fatalf("turns = %+v, %v", ts, err)
	}
	if g := one(t)(s.Blocks(t0.Add(-time.Hour))); g.Block != "" || g.Requests != 2 {
		t.Fatalf("unattributed = %+v", g)
	}
}

// A hook binding attributes requests by time; a new session in the block ends the old binding.
func TestBindingAttributesByInterval(t *testing.T) {
	s := open(t)
	bind := func(session string, at time.Time) {
		if err := s.Bind(usage.Binding{Host: "h", Block: "block:b1", RexSession: "session:r1", Provider: usage.Claude,
			Session: session, Source: usage.SourceHook, Start: at}); err != nil {
			t.Fatal(err)
		}
	}
	bind("s1", t0.Add(-time.Minute))
	bind("s2", t0.Add(time.Minute))
	a := req("r1", usage.SourceTranscript, 10)
	late := req("r2", usage.SourceTranscript, 10)
	late.At = t0.Add(2 * time.Minute) // s1 is no longer in b1
	s2 := req("r3", usage.SourceTranscript, 10)
	s2.Session, s2.At = "s2", t0.Add(2*time.Minute)
	s.AddRequests([]usage.Request{a, late, s2})
	gs, err := s.Blocks(t0.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, g := range gs {
		got[g.Block] += g.Requests
	}
	if got["block:b1"] != 2 || got[""] != 1 {
		t.Fatalf("blocks = %v; want b1:2 unattributed:1", got)
	}
}

func TestLimitsApplyRulesAndCredits(t *testing.T) {
	s := open(t)
	p := func(v float64) *float64 { return &v }
	rep := quota.Report{Provider: usage.Codex, Windows: quota.Windows{
		300:   {Percent: p(10), ResetsAt: t0.Add(time.Hour), ObservedAt: t0},
		10080: {Percent: p(30), ResetsAt: t0.Add(48 * time.Hour), ObservedAt: t0}}}
	s.ApplyReports("work", "codex", []quota.Report{rep}, t0)
	s.ApplyReports("work", "premium", []quota.Report{{Provider: usage.Codex}}, t0)
	s.ApplyReports("work", "codex", []quota.Report{{Provider: usage.Codex, Windows: quota.Windows{
		10080: {Percent: p(31), ResetsAt: t0.Add(48 * time.Hour), ObservedAt: t0.Add(time.Minute)}}}}, t0.Add(time.Minute))
	s.SetCredits(usage.Codex, "work", "codex", Credits{HasCredits: true, Balance: "12.5", ObservedAt: t0})
	ls, err := s.Limits()
	if err != nil {
		t.Fatal(err)
	}
	var codex *Limit
	for i := range ls {
		if ls[i].LimitID == "codex" {
			codex = &ls[i]
		}
	}
	if codex == nil {
		t.Fatalf("limits = %+v", ls)
	}
	if _, ok := codex.Windows[300]; ok {
		t.Fatal("rule 5: omitted codex five-hour window must be cleared")
	}
	if v, ok := codex.Windows[10080].Value(t0.Add(time.Minute)); !ok || v != 31 {
		t.Fatalf("weekly = %v, %v", v, ok)
	}
	if codex.Credits == nil || codex.Credits.Balance != "12.5" {
		t.Fatalf("credits = %+v", codex.Credits)
	}
}

func TestHistoryBuckets(t *testing.T) {
	s := open(t)
	a, b, c := req("r1", usage.SourceOTel, 10), req("r2", usage.SourceOTel, 10), req("r3", usage.SourceOTel, 10)
	b.At, c.At = t0.Add(30*time.Second), t0.Add(90*time.Second)
	s.AddRequests([]usage.Request{a, b, c})
	bs, err := s.History(t0, t0.Add(20*time.Minute))
	if err != nil || len(bs) != 2 || bs[0].Tokens != 2*a.Total() || !bs[1].Minute.Equal(t0.Add(time.Minute)) {
		t.Fatalf("history = %+v, %v", bs, err)
	}
}

func TestCursorRoundTripAndSweep(t *testing.T) {
	s := open(t)
	f := filepath.Join(t.TempDir(), "r.jsonl")
	os.WriteFile(f, []byte("x\n"), 0o600)
	c := tail.Cursor{Path: f, Inode: 7, Offset: 2, Size: 2, ModTime: t0, HeadHash: "h", TailHash: "t"}
	if err := s.SaveCursor(c, json.RawMessage(`{"thread":"t1"}`)); err != nil {
		t.Fatal(err)
	}
	got, st, ok, err := s.Cursor(f)
	if err != nil || !ok || got != c || string(st) != `{"thread":"t1"}` {
		t.Fatalf("cursor = %+v %s %v %v", got, st, ok, err)
	}
	old := req("old", usage.SourceOTel, 1)
	old.At = t0.Add(-8 * 24 * time.Hour)
	s.AddRequests([]usage.Request{old, req("r1", usage.SourceOTel, 1)})
	os.Remove(f)
	if err := s.Sweep(t0); err != nil {
		t.Fatal(err)
	}
	if g := one(t)(s.Sessions(t0.Add(-30 * 24 * time.Hour))); g.Requests != 1 {
		t.Fatalf("sweep left %d requests", g.Requests)
	}
	if _, _, ok, _ := s.Cursor(f); ok {
		t.Fatal("sweep must drop cursors of deleted files")
	}
}

// The store holds usage history: only its owner may read it.
func TestStoreFilesArePrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.AddRequests([]usage.Request{req("r1", usage.SourceOTel, 1)})
	defer s.Close()
	for _, p := range []string{path, path + "-wal"} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("%s mode = %o; want 600", filepath.Base(p), mode)
		}
	}
}
