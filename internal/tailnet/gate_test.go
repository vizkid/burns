package tailnet

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This node's own address is its owner's: the gate learned both at startup, so
// the Mac app reading its own host costs no lookup. Under load a lookup can
// take longer than the app waits (seen 2026-11-02 at load 57 on 18 cores).
func TestGateAllowsThisNodeWithoutALookup(t *testing.T) {
	at := t0
	w := &fakeWhois{err: errors.New("slow")}
	g := &Gate{Whois: w, Login: "owner@example.com", Self: "100.64.0.1", TTL: 5 * time.Minute, Now: func() time.Time { return at }}
	h := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	if code := get(h, "100.64.0.1:5000"); code != http.StatusOK || w.calls != 0 {
		t.Fatalf("code %d, calls %d; want 200 and no lookup", code, w.calls)
	}
}

// A failed refresh keeps the last answer for Grace: the peer behind a tailnet
// address does not change because tailscaled was slow to answer.
func TestGateKeepsTheLastAnswerWhenARefreshFails(t *testing.T) {
	at := t0
	w := &fakeWhois{ids: map[string]Identity{"100.64.0.2": {Login: "owner@example.com"}}}
	g := &Gate{Whois: w, Login: "owner@example.com", TTL: 5 * time.Minute, Grace: time.Hour, Now: func() time.Time { return at }}
	h := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	get(h, "100.64.0.2:1")
	w.err = errors.New("killed")
	at = t0.Add(30 * time.Minute)
	if code := get(h, "100.64.0.2:1"); code != http.StatusOK {
		t.Fatalf("code %d within grace; want 200", code)
	}
	at = t0.Add(2 * time.Hour)
	if code := get(h, "100.64.0.2:1"); code != http.StatusForbidden {
		t.Fatalf("code %d past grace; want 403", code)
	}
}

// slowWhois holds every lookup until released and counts them.
type slowWhois struct {
	calls   atomic.Int32
	release chan struct{}
}

func (s *slowWhois) Whois(ctx context.Context, _ string) (Identity, error) {
	s.calls.Add(1)
	<-s.release
	return Identity{Login: "owner@example.com"}, nil
}

// Requests from one caller that arrive together share one lookup: each started
// its own process, and six were killed at once.
func TestGateSharesOneLookupAmongConcurrentRequests(t *testing.T) {
	w := &slowWhois{release: make(chan struct{})}
	g := &Gate{Whois: w, Login: "owner@example.com", TTL: 5 * time.Minute, Now: func() time.Time { return t0 }}
	h := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	var wg sync.WaitGroup
	codes := make([]int, 6)
	for i := range codes {
		wg.Go(func() { codes[i] = get(h, "100.64.0.2:1") })
	}
	for w.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // let the others queue behind the first
	close(w.release)
	wg.Wait()
	if n := w.calls.Load(); n != 1 {
		t.Errorf("lookups = %d; want 1", n)
	}
	for i, c := range codes {
		if c != http.StatusOK {
			t.Errorf("request %d: code %d", i, c)
		}
	}
}
