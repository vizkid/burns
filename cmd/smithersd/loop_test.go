package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeScanner struct{}

func (fakeScanner) Scan() error             { return nil }
func (fakeScanner) Interval() time.Duration { return time.Hour }

// fakeUpkeep records calls; its first full roll-up blocks until released, as a
// roll-up of the whole history does on a loaded host.
type fakeUpkeep struct {
	mu      sync.Mutex
	calls   []string
	started chan struct{}
	release chan struct{}
}

func (f *fakeUpkeep) Sweep(time.Time) error { f.note("sweep"); return nil }
func (f *fakeUpkeep) RollUp(from, _ time.Time) error {
	if from.IsZero() {
		f.note("rollup all")
		close(f.started)
		<-f.release
	}
	return nil
}
func (f *fakeUpkeep) note(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }

// The startup sweep and full roll-up run in the background loop, so smithersd
// listens at once. On 2026-11-04 Baroni, at load 55, the roll-up held the
// listener closed for minutes after a restart, and Claude's telemetry was refused.
func TestLoopDoesTheStartupUpkeep(t *testing.T) {
	up := &fakeUpkeep{started: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { loop(ctx, fakeScanner{}, up); close(done) }()
	select {
	case <-up.started:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not start the full roll-up")
	}
	close(up.release)
	cancel()
	<-done
	up.mu.Lock()
	defer up.mu.Unlock()
	if len(up.calls) < 2 || up.calls[0] != "sweep" || up.calls[1] != "rollup all" {
		t.Errorf("calls %q; want sweep, then the full roll-up", up.calls)
	}
}
