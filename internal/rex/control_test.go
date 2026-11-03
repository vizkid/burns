package rex

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vizkid/rexkit/rexctl"
	"github.com/vizkid/rexkit/rextest"
)

// The pane list comes over one control connection, so a refresh starts no
// process (peer review, 2026-11-03: a process per session every 15 seconds).
// It runs against the local Rex server in a throwaway session.
func TestControlListsPanesWithSessionNames(t *testing.T) {
	c := rextest.Require(t)
	label := rextest.Label("panes")
	session, first := rextest.Session(t, c, label)
	second := rextest.Split(t, c, session, first, "cap")
	home, _ := os.UserHomeDir()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	ctl := StartControl(ctx, rexctl.LocalEndpoint(os.Getenv, home), "test")
	panes, err := ctl.Panes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := panes[second]; got != (Pane{Session: label, Label: "cap"}) {
		t.Errorf("split pane = %+v", got)
	}
	if got, ok := panes[first]; !ok || got.Session != label {
		t.Errorf("first pane = %+v, %v", got, ok)
	}
	// A session made after the connection attached is listed too.
	later := rextest.Label("later")
	_, block := rextest.Session(t, c, later)
	if panes, err = ctl.Panes(ctx); err != nil || panes[block].Session != later {
		t.Fatalf("later session: %+v, %v", panes[block], err)
	}
}

// With no server to reach, the list fails whole rather than coming back empty,
// which would call every live pane dead.
func TestControlFailsWithoutAServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	ctl := StartControl(ctx, "unix://"+t.TempDir()+"/none.sock", "test")
	if panes, err := ctl.Panes(ctx); err == nil {
		t.Fatalf("panes = %+v; want an error", panes)
	}
}
