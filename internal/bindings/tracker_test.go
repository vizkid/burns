package bindings

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/rex"
	"github.com/vizkid/burns/internal/store"
	"github.com/vizkid/burns/internal/usage"
)

var t0 = time.Date(2026, 10, 28, 4, 0, 0, 0, time.UTC)

type fakeRex map[string]struct {
	pid int
	err error
}

func (f fakeRex) Foreground(_ context.Context, block string) (int, error) {
	a, ok := f[block]
	if !ok {
		return 0, rex.ErrNoBlock
	}
	return a.pid, a.err
}

func tracker(t *testing.T, r fakeRex, blocks ...string) *Tracker {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for i, b := range blocks {
		err := st.Bind(usage.Binding{Host: "h", Block: b, RexSession: "session:r1", Provider: usage.Claude,
			Session: "s-" + b, Source: "hook:startup", Verified: true, FgPid: 100 + i, Start: t0})
		if err != nil {
			t.Fatal(err)
		}
	}
	return &Tracker{Store: st, Rex: r, Host: "h", Now: func() time.Time { return t0.Add(time.Hour) }}
}

func open(t *testing.T, tr *Tracker) []string {
	t.Helper()
	blocks, err := tr.Store.OpenBlocks("h")
	if err != nil {
		t.Fatal(err)
	}
	return blocks
}

func TestHandleEndsBindings(t *testing.T) {
	tr := tracker(t, nil, "block:b0", "block:b1", "block:b2")
	events := []rex.Event{
		{Kind: rex.BlockClosed, Session: "session:r1", Block: "block:b0"},
		{Kind: rex.Foreground, Session: "session:r1", Block: "block:b1", Pid: 101}, // same agent: stays
		{Kind: rex.Foreground, Session: "session:r1", Block: "block:b2", Pid: 0},   // program gone
	}
	for _, e := range events {
		if err := tr.Handle(e); err != nil {
			t.Fatal(err)
		}
	}
	if got := open(t, tr); len(got) != 1 || got[0] != "block:b1" {
		t.Fatalf("open = %v; want [block:b1]", got)
	}
	if err := tr.Handle(rex.Event{Kind: rex.SessionDestroyed, Session: "session:r1"}); err != nil {
		t.Fatal(err)
	}
	if got := open(t, tr); len(got) != 0 {
		t.Fatalf("open = %v after session_destroyed; want none", got)
	}
}

func TestReconcile(t *testing.T) {
	r := fakeRex{
		"block:b1": {pid: 101},                        // agent still in front
		"block:b2": {pid: 0},                          // no foreground
		"block:b3": {err: errors.New("rex: timeout")}, // unknown: leave alone
		"block:b4": {pid: 555},                        // other program: next claim decides
	}
	tr := tracker(t, r, "block:b0", "block:b1", "block:b2", "block:b3", "block:b4") // b0 is gone
	if err := tr.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := open(t, tr)
	want := []string{"block:b1", "block:b3", "block:b4"}
	if len(got) != len(want) {
		t.Fatalf("open = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("open = %v; want %v", got, want)
		}
	}
}
