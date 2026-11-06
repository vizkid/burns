package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/usage"
)

func roarFile(t *testing.T, dir, block, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, block+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Roar's Codex bindings bind when Rex confirms the pid, and only then.
func TestSyncRoarBindsConfirmedFiles(t *testing.T) {
	_, s := server(t)
	dir := t.TempDir()
	roarFile(t, dir, "block:live", `{"thread":"t1","session":"session:r","codex_home":"/h/.codex","pid":40,"at":"2026-10-28T03:00:00Z"}`)
	roarFile(t, dir, "block:stale", `{"thread":"t2","session":"session:r","pid":41,"at":"2026-10-28T03:00:00Z"}`)
	roarFile(t, dir, "block:junk", `not json`)
	s.Rex = rexFunc(func(_ context.Context, block string) (int, error) {
		return map[string]int{"block:live": 40, "block:stale": 99}[block], nil
	})
	if n := s.SyncRoar(dir); n != 1 {
		t.Fatalf("bound %d; want 1", n)
	}
	bs, err := s.Store.Bindings("host-1")
	if err != nil || len(bs) != 1 || bs[0].Block != "block:live" || bs[0].Session != "t1" || bs[0].Provider != usage.Codex ||
		!bs[0].Verified || bs[0].FgPid != 40 || bs[0].Source != "roar" {
		t.Fatalf("bindings = %+v, %v", bs, err)
	}
	if n := s.SyncRoar(dir); n != 0 {
		t.Errorf("second pass bound %d; an open binding is not bound again", n)
	}
}

// A file older than the block's newest binding names an old thread: a reset
// moved the pane on, and the file must not move it back.
func TestSyncRoarSkipsOlderFile(t *testing.T) {
	_, s := server(t)
	dir := t.TempDir()
	newer := usage.Binding{Host: "host-1", Block: "block:live", Provider: usage.Codex, Session: "t9", Source: "launch",
		Verified: true, FgPid: 40, Start: time.Date(2026, 10, 28, 4, 0, 0, 0, time.UTC)}
	if err := s.Store.Bind(newer); err != nil {
		t.Fatal(err)
	}
	roarFile(t, dir, "block:live", `{"thread":"t1","session":"session:r","pid":40,"at":"2026-10-28T03:00:00Z"}`)
	s.Rex = foreground(40, nil)
	if n := s.SyncRoar(dir); n != 0 {
		t.Fatalf("bound %d; want the older file skipped", n)
	}
}

// When Rex fails, a file is skipped, never stored unverified.
func TestSyncRoarSkipsWhenRexFails(t *testing.T) {
	_, s := server(t)
	dir := t.TempDir()
	roarFile(t, dir, "block:live", `{"thread":"t1","session":"session:r","pid":40,"at":"2026-10-28T03:00:00Z"}`)
	s.Rex = foreground(0, errors.New("rex: server unavailable"))
	if n := s.SyncRoar(dir); n != 0 {
		t.Fatalf("bound %d", n)
	}
	if bs, _ := s.Store.Bindings("host-1"); len(bs) != 0 {
		t.Fatalf("bindings = %+v; want none", bs)
	}
}
