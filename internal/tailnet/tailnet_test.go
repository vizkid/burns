package tailnet

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeWhois struct {
	ids   map[string]Identity
	err   error
	calls int
}

func (f *fakeWhois) Whois(_ context.Context, ip string) (Identity, error) {
	f.calls++
	if f.err != nil {
		return Identity{}, f.err
	}
	id, ok := f.ids[ip]
	if !ok {
		return Identity{}, errors.New("no such peer")
	}
	return id, nil
}

var t0 = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func gate(w Whois, at *time.Time) http.Handler {
	g := &Gate{Whois: w, Login: "owner@example.com", TTL: 5 * time.Minute, Now: func() time.Time { return *at }}
	return g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
}

func get(h http.Handler, remote string) int {
	r := httptest.NewRequest("GET", "/usage", nil)
	r.RemoteAddr = remote
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestGateAllowsTheOwnersOtherDevices(t *testing.T) {
	at := t0
	h := gate(&fakeWhois{ids: map[string]Identity{"100.64.0.2": {Login: "owner@example.com"}}}, &at)
	if code := get(h, "100.64.0.2:51000"); code != http.StatusOK {
		t.Fatalf("code = %d; want 200", code)
	}
}

func TestGateRefusesOthers(t *testing.T) {
	at := t0
	w := &fakeWhois{ids: map[string]Identity{
		"100.64.0.3": {Login: "friend@example.com"},
		"100.64.0.4": {Login: "owner@example.com", Tagged: true},
	}}
	h := gate(w, &at)
	for _, remote := range []string{"100.64.0.3:1", "100.64.0.4:1", "100.64.0.9:1", "not-an-address"} {
		if code := get(h, remote); code != http.StatusForbidden {
			t.Errorf("%s: code = %d; want 403", remote, code)
		}
	}
	if code := get(gate(&fakeWhois{err: errors.New("tailscaled down")}, &at), "100.64.0.2:1"); code != http.StatusForbidden {
		t.Errorf("lookup failure: code = %d; want 403", code)
	}
}

// One lookup per caller address within the TTL, keyed by IP, not port.
func TestGateCachesLookups(t *testing.T) {
	at := t0
	w := &fakeWhois{ids: map[string]Identity{"100.64.0.2": {Login: "owner@example.com"}}}
	h := gate(w, &at)
	get(h, "100.64.0.2:1")
	get(h, "100.64.0.2:2")
	if w.calls != 1 {
		t.Fatalf("calls = %d; want 1", w.calls)
	}
	at = t0.Add(6 * time.Minute)
	get(h, "100.64.0.2:3")
	if w.calls != 2 {
		t.Fatalf("calls = %d after the TTL; want 2", w.calls)
	}
}

// A refusal is not cached: a lookup that failed while tailscaled restarted is retried.
func TestGateRetriesFailedLookups(t *testing.T) {
	at := t0
	w := &fakeWhois{err: errors.New("down")}
	h := gate(w, &at)
	get(h, "100.64.0.2:1")
	w.err, w.ids = nil, map[string]Identity{"100.64.0.2": {Login: "owner@example.com"}}
	if code := get(h, "100.64.0.2:1"); code != http.StatusOK {
		t.Fatalf("code = %d; want 200 once the lookup works", code)
	}
}

func fakeTailscale(t *testing.T, script string) CLI {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tailscale")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return CLI{Path: path}
}

func TestCLIWhois(t *testing.T) {
	c := fakeTailscale(t, `[ "$1 $2 $3" = "whois --json 100.64.0.2" ] || exit 1
echo '{"Node":{"Name":"phone.example.ts.net.","Tags":null},"UserProfile":{"LoginName":"owner@example.com"}}'`)
	id, err := c.Whois(context.Background(), "100.64.0.2")
	if err != nil || id.Login != "owner@example.com" || id.Tagged {
		t.Fatalf("id = %+v, %v", id, err)
	}
	tagged := fakeTailscale(t, `echo '{"Node":{"Tags":["tag:server"]},"UserProfile":{"LoginName":"tagged-devices"}}'`)
	if id, err := tagged.Whois(context.Background(), "100.64.0.5"); err != nil || !id.Tagged {
		t.Fatalf("tagged id = %+v, %v", id, err)
	}
}

func TestCLISelf(t *testing.T) {
	c := fakeTailscale(t, `case "$1" in
ip) echo 100.64.0.1 ;;
whois) [ "$3" = 100.64.0.1 ] && echo '{"Node":{},"UserProfile":{"LoginName":"owner@example.com"}}' ;;
esac`)
	ip, login, err := c.Self(context.Background())
	if err != nil || ip != "100.64.0.1" || login != "owner@example.com" {
		t.Fatalf("self = %q %q %v", ip, login, err)
	}
	down := fakeTailscale(t, `echo "Tailscale is stopped." >&2; exit 1`)
	if _, _, err := down.Self(context.Background()); err == nil {
		t.Fatal("Self with Tailscale stopped: want an error")
	}
}
