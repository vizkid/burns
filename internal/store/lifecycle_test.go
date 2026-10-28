package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/usage"
)

func claim(session string, fg int, source string, at time.Duration) usage.Binding {
	return usage.Binding{Host: "h", Block: "block:b1", RexSession: "session:r1", Provider: usage.Claude,
		Session: session, Source: "hook:" + source, Verified: fg != 0, FgPid: fg, Start: t0.Add(at)}
}

func bind(t *testing.T, s *Store, bs ...usage.Binding) {
	t.Helper()
	for _, b := range bs {
		if err := s.Bind(b); err != nil {
			t.Fatal(err)
		}
	}
}

// open returns the sessions with an open binding on host h.
func openSessions(t *testing.T, s *Store) map[string]bool {
	t.Helper()
	bs, err := s.Bindings("h")
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]bool{}
	for _, b := range bs {
		if b.End == nil {
			m[b.Session] = true
		}
	}
	return m
}

func want(t *testing.T, got map[string]bool, sessions ...string) {
	t.Helper()
	if len(got) != len(sessions) {
		t.Fatalf("open = %v; want %v", got, sessions)
	}
	for _, s := range sessions {
		if !got[s] {
			t.Fatalf("open = %v; want %v", got, sessions)
		}
	}
}

// A nested `claude -p` shares the outer agent's foreground and must not end it.
func TestNestedClaimKeepsOuterBinding(t *testing.T) {
	s := open(t)
	bind(t, s, claim("outer", 100, "startup", 0), claim("nested", 100, "startup", time.Minute))
	want(t, openSessions(t, s), "outer", "nested")
}

func TestClaimWithAnotherForegroundEndsOldBinding(t *testing.T) {
	s := open(t)
	bind(t, s, claim("old", 100, "startup", 0), claim("new", 200, "startup", time.Minute))
	want(t, openSessions(t, s), "new")
}

func TestUnverifiedClaimEndsOtherBindings(t *testing.T) {
	s := open(t)
	bind(t, s, claim("old", 100, "startup", 0), claim("new", 0, "startup", time.Minute))
	want(t, openSessions(t, s), "new")
}

func TestClearEndsSameProcessSession(t *testing.T) {
	s := open(t)
	bind(t, s, claim("before", 100, "startup", 0), claim("after", 100, "clear", time.Minute))
	want(t, openSessions(t, s), "after")
}

func TestVerifiedFieldsRoundTrip(t *testing.T) {
	s := open(t)
	bind(t, s, claim("s1", 100, "startup", 0))
	bs, err := s.Bindings("h")
	if err != nil || len(bs) != 1 || !bs[0].Verified || bs[0].FgPid != 100 {
		t.Fatalf("bindings = %+v, %v", bs, err)
	}
}

func TestEndBlockAndRexSession(t *testing.T) {
	s := open(t)
	other := claim("elsewhere", 300, "startup", 0)
	other.Block, other.RexSession = "block:b2", "session:r2"
	bind(t, s, claim("a", 100, "startup", 0), claim("b", 100, "startup", 0), other)
	if err := s.EndBlock("h", "block:b1", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	want(t, openSessions(t, s), "elsewhere")
	if err := s.EndRexSession("h", "session:r2", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	want(t, openSessions(t, s))
}

func TestForegroundChanged(t *testing.T) {
	s := open(t)
	bind(t, s, claim("agent", 100, "startup", 0))
	end := t0.Add(time.Hour)
	// An editor the agent opened has the agent among its ancestors.
	if err := s.ForegroundChanged("h", "block:b1", 150, []int{100, 50}, end); err != nil {
		t.Fatal(err)
	}
	want(t, openSessions(t, s), "agent")
	if err := s.ForegroundChanged("h", "block:b1", 100, nil, end); err != nil {
		t.Fatal(err)
	}
	want(t, openSessions(t, s), "agent")
	if err := s.ForegroundChanged("h", "block:b1", 50, []int{40}, end); err != nil {
		t.Fatal(err)
	}
	want(t, openSessions(t, s))
}

func TestForegroundChangedLeavesUnknownForegroundUnlessNone(t *testing.T) {
	s := open(t)
	bind(t, s, claim("unverified", 0, "startup", 0))
	if err := s.ForegroundChanged("h", "block:b1", 50, nil, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	want(t, openSessions(t, s), "unverified")
	if err := s.ForegroundChanged("h", "block:b1", 0, nil, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	want(t, openSessions(t, s))
}

func TestOpenBlocks(t *testing.T) {
	s := open(t)
	closed := claim("gone", 100, "startup", 0)
	closed.Block = "block:b2"
	bind(t, s, claim("a", 100, "startup", 0), claim("b", 100, "startup", 0), closed)
	if err := s.EndBlock("h", "block:b2", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	blocks, err := s.OpenBlocks("h")
	if err != nil || len(blocks) != 1 || blocks[0] != "block:b1" {
		t.Fatalf("OpenBlocks = %v, %v; want [block:b1]", blocks, err)
	}
}

// A store from before migration 3 keeps its bindings, which read as unverified.
func TestMigrationKeepsOldBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "smithers.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migrations[:2] {
		if _, err := db.Exec(m); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 2;
		INSERT INTO binding (host, block, rex_session, provider, session, source, start)
		VALUES ('h', 'block:b1', 'session:r1', 'claude', 'old', 'hook:startup', 1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bs, err := s.Bindings("h")
	if err != nil || len(bs) != 1 || bs[0].Session != "old" || bs[0].Verified || bs[0].FgPid != 0 {
		t.Fatalf("bindings = %+v, %v", bs, err)
	}
}
