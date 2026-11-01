package rex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func script(t *testing.T, body string) CLI {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rex")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return CLI{Path: p}
}

func TestPanesListsBlocksWithSessionNames(t *testing.T) {
	c := script(t, `case "$1" in
session) echo '{"sessions":[{"session_id":"session:1","label":"work"}]}' ;;
block) echo '{"blocks":[{"block_id":"block:a","label":"cap"},{"block_id":"block:b"}]}' ;;
esac`)
	got, err := c.Panes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got["block:a"] != (Pane{Session: "work", Label: "cap"}) || got["block:b"] != (Pane{Session: "work"}) || len(got) != 2 {
		t.Fatalf("panes = %+v", got)
	}
}

func TestPanesFailsWholeWhenOneSessionFails(t *testing.T) {
	c := script(t, `case "$1" in
session) echo '{"sessions":[{"session_id":"session:1","label":"work"}]}' ;;
block) exit 1 ;;
esac`)
	if got, err := c.Panes(context.Background()); err == nil {
		t.Fatalf("panes = %+v; want an error", got)
	}
}

func TestPaneCacheAnswersWithoutCallingRexAndExpires(t *testing.T) {
	now := time.Unix(1000, 0)
	calls := 0
	var fail error
	c := &PaneCache{Now: func() time.Time { return now }, MaxAge: time.Minute, List: func(context.Context) (map[string]Pane, error) {
		calls++
		return map[string]Pane{"block:a": {Session: "work"}}, fail
	}}
	if _, err := c.Panes(context.Background()); err == nil {
		t.Fatal("an empty cache must not claim an answer")
	}
	c.Refresh(context.Background())
	for range 3 {
		if got, err := c.Panes(context.Background()); err != nil || len(got) != 1 {
			t.Fatalf("panes = %+v, err %v", got, err)
		}
	}
	if calls != 1 {
		t.Fatalf("Rex calls = %d; want 1", calls)
	}
	// A failed refresh keeps the last list until it is too old to trust.
	fail = errors.New("no rex")
	c.Refresh(context.Background())
	if _, err := c.Panes(context.Background()); err != nil {
		t.Fatalf("a recent list must survive one failure: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := c.Panes(context.Background()); err == nil {
		t.Fatal("a stale list must count as no answer")
	}
}
