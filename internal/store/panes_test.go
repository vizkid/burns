package store

import (
	"testing"
	"time"

	"github.com/vizkid/burns/internal/usage"
)

func TestPanesSeparateTheMainThreadFromSubagents(t *testing.T) {
	s := open(t)
	if err := s.Bind(usage.Binding{Host: "h", Block: "block:live", RexSession: "session:r", Provider: usage.Claude, Session: "s1", Source: "startup", Start: t0}); err != nil {
		t.Fatal(err)
	}
	sub := paneReq("sub", "s1", "claude-haiku-5-5", 900, 0, 0, 0, t0.Add(time.Minute))
	sub.Agent = "agent-1"
	other := paneReq("sub2", "s1", "claude-fable-5-1", 0, 0, 0, 0, t0.Add(time.Minute))
	other.Agent = "agent-1"
	third := paneReq("sub3", "s1", "claude-haiku-5-5", 0, 0, 0, 0, t0.Add(time.Minute))
	third.Agent = "agent-2"
	if err := s.AddRequests([]usage.Request{sub, other, third, paneReq("main", "s1", "claude-fable-5-1", 10, 0, 0, 0, t0.Add(time.Minute))}); err != nil {
		t.Fatal(err)
	}
	panes, err := s.Panes()
	if err != nil || len(panes) != 2 {
		t.Fatalf("panes = %+v, err %v", panes, err)
	}
	if p := panes[0]; p.Model != "claude-haiku-5-5" || p.Tokens != 900 || p.MainTokens != 0 {
		t.Errorf("subagent row = %+v", p)
	}
	if p := panes[1]; p.Model != "claude-fable-5-1" || p.Tokens != 10 || p.MainTokens != 10 {
		t.Errorf("main row = %+v", p)
	}
	// The same subagent on two models counts once, on every row of the pane.
	if panes[0].Agents != 2 || panes[1].Agents != 2 {
		t.Errorf("agents = %d and %d; want 2", panes[0].Agents, panes[1].Agents)
	}
}

func TestPanesCountANestedSessionAsASubagent(t *testing.T) {
	s := open(t)
	for i, session := range []string{"outer", "nested"} {
		b := usage.Binding{Host: "h", Block: "block:live", RexSession: "session:r", Provider: usage.Claude, Session: session, Source: "startup", FgPid: 7, Start: t0.Add(time.Duration(i) * time.Minute)}
		if err := s.Bind(b); err != nil {
			t.Fatal(err)
		}
	}
	sub := paneReq("sub", "outer", "claude-fable-5-1", 1, 0, 0, 0, t0.Add(3*time.Minute))
	sub.Agent = "agent-1"
	err := s.AddRequests([]usage.Request{sub, paneReq("main", "outer", "claude-fable-5-1", 50, 0, 0, 0, t0.Add(3*time.Minute)),
		paneReq("inner", "nested", "claude-haiku-5-5", 5000, 0, 0, 0, t0.Add(3*time.Minute))})
	if err != nil {
		t.Fatal(err)
	}
	panes, err := s.Panes()
	if err != nil || len(panes) != 2 {
		t.Fatalf("panes = %+v, err %v", panes, err)
	}
	if p := panes[0]; p.Model != "claude-haiku-5-5" || p.Tokens != 5000 || p.MainTokens != 0 || p.Agents != 2 {
		t.Errorf("nested row = %+v; want no main tokens and 2 agents", p)
	}
	if p := panes[1]; p.Model != "claude-fable-5-1" || p.Tokens != 51 || p.MainTokens != 50 || p.Agents != 2 {
		t.Errorf("outer row = %+v; want 50 main tokens and 2 agents", p)
	}
}

func TestPanesKeepTwoAccountsInOneBlockApart(t *testing.T) {
	s := open(t)
	for i, session := range []string{"a", "b"} {
		b := usage.Binding{Host: "h", Block: "block:live", RexSession: "session:r", Provider: usage.Claude, Session: session, Source: "startup", FgPid: 7, Start: t0.Add(time.Duration(i) * time.Minute)}
		if err := s.Bind(b); err != nil {
			t.Fatal(err)
		}
	}
	one, two := paneReq("1", "a", "claude-opus-5-5", 5, 0, 0, 0, t0.Add(3*time.Minute)), paneReq("2", "b", "claude-opus-5-5", 3, 0, 0, 0, t0.Add(3*time.Minute))
	two.Account = "beta"
	if err := s.AddRequests([]usage.Request{one, two}); err != nil {
		t.Fatal(err)
	}
	if panes, err := s.Panes(); err != nil || len(panes) != 2 || panes[0].Account != "alpha" || panes[1].Account != "beta" {
		t.Fatalf("panes = %+v, err %v", panes, err)
	}
}

func paneReq(key, session, model string, in, cached, write, out int64, at time.Time) usage.Request {
	return usage.Request{Provider: usage.Claude, Key: key, Account: "alpha", Session: session, Turn: "p1", Model: model,
		Input: in, CachedInput: cached, CacheWrite: write, Output: out, Source: "test", At: at}
}

func TestPanesCountsOpenBindingsByModelWithoutCachedInput(t *testing.T) {
	s := open(t)
	for _, b := range []usage.Binding{
		{Host: "h", Block: "block:live", RexSession: "session:r", Provider: usage.Claude, Session: "s1", Source: "startup", Start: t0},
		{Host: "h", Block: "block:gone", RexSession: "session:r", Provider: usage.Claude, Session: "s2", Source: "startup", Start: t0},
	} {
		if err := s.Bind(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.EndBlock("h", "block:gone", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	err := s.AddRequests([]usage.Request{
		paneReq("before", "s1", "claude-opus-5-5", 1000, 0, 0, 0, t0.Add(-time.Hour)), // Before this pane took the session.
		paneReq("a", "s1", "claude-opus-5-5", 10, 5000, 20, 30, t0.Add(time.Minute)),
		paneReq("b", "s1", "claude-opus-5-5", 1, 5000, 2, 3, t0.Add(2*time.Minute)),
		paneReq("c", "s1", "claude-haiku-5-5", 4, 0, 0, 1, t0.Add(3*time.Minute)),
		paneReq("d", "s1", "<synthetic>", 9, 0, 0, 9, t0.Add(4*time.Minute)),
		paneReq("e", "s2", "claude-opus-5-5", 7, 0, 0, 7, t0.Add(time.Minute)),
	})
	if err != nil {
		t.Fatal(err)
	}
	panes, err := s.Panes()
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 2 {
		t.Fatalf("panes = %+v; want two rows of the live pane", panes)
	}
	if p := panes[0]; p.Block != "block:live" || p.Account != "alpha" || p.Provider != "claude" || p.Model != "claude-opus-5-5" ||
		p.Tokens != 66 || !p.Last.Equal(t0.Add(2*time.Minute)) {
		t.Errorf("largest = %+v; want opus with 66 uncached tokens", p)
	}
	if p := panes[1]; p.Model != "claude-haiku-5-5" || p.Tokens != 5 {
		t.Errorf("second = %+v; want haiku with 5", p)
	}
}

// A pane reports its own session's compactions since it took the session, so
// an agent can ask how often a lane compacted without reading the store. A
// nested session's compactions are a subagent's, and one before the pane took
// the session belongs to an earlier pane.
func TestPanesCountTheirOwnCompactions(t *testing.T) {
	s := open(t)
	for i, session := range []string{"outer", "nested"} {
		b := usage.Binding{Host: "h", Block: "block:live", RexSession: "session:r", Provider: usage.Claude, Session: session, Source: "startup", FgPid: 7, Start: t0.Add(time.Duration(i) * time.Minute)}
		if err := s.Bind(b); err != nil {
			t.Fatal(err)
		}
	}
	err := s.AddRequests([]usage.Request{paneReq("main", "outer", "claude-fable-5-1", 50, 0, 0, 0, t0.Add(3*time.Minute)),
		paneReq("inner", "nested", "claude-haiku-5-5", 5, 0, 0, 0, t0.Add(3*time.Minute))})
	if err != nil {
		t.Fatal(err)
	}
	comp := func(session string, at time.Time) usage.Compaction {
		return usage.Compaction{Provider: usage.Claude, Account: "alpha", Session: session, Trigger: "auto", Source: usage.SourceOTel, At: at}
	}
	if err := s.AddCompactions([]usage.Compaction{comp("outer", t0.Add(-time.Hour)), comp("outer", t0.Add(10*time.Minute)),
		comp("outer", t0.Add(20*time.Minute)), comp("nested", t0.Add(30*time.Minute))}); err != nil {
		t.Fatal(err)
	}
	panes, err := s.Panes()
	if err != nil || len(panes) != 2 {
		t.Fatalf("panes = %+v, err %v", panes, err)
	}
	for _, p := range panes {
		if p.Compactions != 2 || p.LastCompaction == nil || !p.LastCompaction.Equal(t0.Add(20*time.Minute)) {
			t.Errorf("%s: compactions %d, last %v; want 2 at %v", p.Model, p.Compactions, p.LastCompaction, t0.Add(20*time.Minute))
		}
	}
}

// A pane that never compacted says so with a zero count and no time.
func TestPanesWithoutCompactionsHaveNoLastTime(t *testing.T) {
	s := open(t)
	if err := s.Bind(usage.Binding{Host: "h", Block: "block:live", RexSession: "session:r", Provider: usage.Claude, Session: "s1", Source: "startup", Start: t0}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRequests([]usage.Request{paneReq("r", "s1", "claude-opus-5-5", 5, 0, 0, 0, t0.Add(time.Minute))}); err != nil {
		t.Fatal(err)
	}
	panes, err := s.Panes()
	if err != nil || len(panes) != 1 || panes[0].Compactions != 0 || panes[0].LastCompaction != nil {
		t.Fatalf("panes = %+v, err %v", panes, err)
	}
}
