package billing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/store"
)

var now = time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)

// Taken from a live response, with the amounts changed.
const live = `{"seven_day":{"utilization":47.0},"extra_usage":{"is_enabled":true,"monthly_limit":200000,"used_credits":1234.0,
 "currency":"SGD","decimal_places":2,"disabled_reason":null},
 "spend":{"used":{"amount_minor":1234,"currency":"SGD","exponent":2},"limit":{"amount_minor":200000,"currency":"SGD","exponent":2},"percent":0}}`

func TestParseSpend(t *testing.T) {
	b, err := parse([]byte(live))
	if err != nil {
		t.Fatal(err)
	}
	if b.UsedMinor != 1234 || b.Currency != "SGD" || b.Exponent != 2 || b.LimitMinor == nil || *b.LimitMinor != 200000 || !b.Enabled {
		t.Fatalf("billed = %+v", b)
	}
}

// Usage credits off: no limit, and the reason is kept.
func TestParseDisabled(t *testing.T) {
	b, err := parse([]byte(`{"extra_usage":{"is_enabled":false,"disabled_reason":"out_of_credits"},
	 "spend":{"used":{"amount_minor":0,"currency":"SGD","exponent":2},"limit":null}}`))
	if err != nil || b.LimitMinor != nil || b.Enabled || b.Reason != "out_of_credits" {
		t.Fatalf("billed = %+v, %v", b, err)
	}
}

// An undocumented shape can change. A response without spend is no reading, never zero.
func TestUnknownShapeIsNoReading(t *testing.T) {
	for _, body := range []string{`{}`, `{"spend":null}`, `{"spend":{"used":{"amount_minor":0}},"extra_usage":{}}`, `not json`} {
		if _, err := parse([]byte(body)); err == nil {
			t.Errorf("parsed %s", body)
		}
	}
}

func poller(t *testing.T, url string, token func(string) (string, error)) (*Poller, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := config.Config{Accounts: []config.Account{
		{Provider: "claude", Name: "alpha", Home: "/h/.claude-alpha", Kind: "subscription"},
		{Provider: "codex", Name: "alpha", Home: "/h/.codex-alpha", Kind: "subscription"}}}
	return &Poller{Store: st, Config: cfg, Client: http.DefaultClient, URL: url, Token: token, Now: func() time.Time { return now }}, st
}

// The poller sends the login to the endpoint, and stores what comes back for Claude accounts only.
func TestPollStoresTheReading(t *testing.T) {
	var auth, beta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, beta = r.Header.Get("Authorization"), r.Header.Get("anthropic-beta")
		w.Write([]byte(live))
	}))
	defer srv.Close()
	p, st := poller(t, srv.URL, func(string) (string, error) { return "tok", nil })
	p.Poll(context.Background())
	got, err := st.BilledAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Provider != "claude" || got[0].Account != "alpha" || got[0].UsedMinor != 1234 || !got[0].ObservedAt.Equal(now) {
		t.Fatalf("billed = %+v", got)
	}
	if auth != "Bearer tok" || beta != oauthBeta {
		t.Fatalf("auth %q, beta %q", auth, beta)
	}
}

// A refused read leaves the last reading alone: its age shows it is old, and it does not fall to zero.
func TestFailedPollKeepsTheLastReading(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(live))
	}))
	defer srv.Close()
	p, st := poller(t, srv.URL, func(string) (string, error) { return "tok", nil })
	p.Poll(context.Background())
	status = http.StatusTooManyRequests
	p.Now = func() time.Time { return now.Add(time.Hour) }
	p.Poll(context.Background())
	got, _ := st.BilledAll()
	if len(got) != 1 || !got[0].ObservedAt.Equal(now) || got[0].UsedMinor != 1234 {
		t.Fatalf("billed = %+v", got)
	}
}

// A host with no login for an account stores nothing and calls nobody.
func TestNoLoginMakesNoRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	p, st := poller(t, srv.URL, func(string) (string, error) { return "", ErrNoToken })
	p.Poll(context.Background())
	got, _ := st.BilledAll()
	if called || len(got) != 0 {
		t.Fatalf("called %v, billed %+v", called, got)
	}
}

func TestAccessToken(t *testing.T) {
	soon := now.Add(time.Hour).UnixMilli()
	raw := func(ms int64) []byte {
		return []byte(`{"claudeAiOauth":{"accessToken":"tok","expiresAt":` + itoa(ms) + `}}`)
	}
	if tok, err := accessToken(raw(soon), now); err != nil || tok != "tok" {
		t.Fatalf("%q, %v", tok, err)
	}
	if _, err := accessToken(raw(now.Add(-time.Hour).UnixMilli()), now); !errors.Is(err, ErrNoToken) {
		t.Fatalf("expired: %v", err)
	}
	if _, err := accessToken([]byte(`{}`), now); !errors.Is(err, ErrNoToken) {
		t.Fatalf("empty: %v", err)
	}
}

// Each home has its own keychain entry, and the default home has no suffix. The hash is
// Claude Code's own, checked against a real keychain on 2026-11-02.
func TestKeychainService(t *testing.T) {
	if got := keychainService("/Users/u/.claude", "/Users/u"); got != "Claude Code-credentials" {
		t.Fatalf("default = %q", got)
	}
	got := keychainService("/Users/u/.claude-work", "/Users/u")
	if !strings.HasPrefix(got, "Claude Code-credentials-") || len(got) != len("Claude Code-credentials-")+8 {
		t.Fatalf("named = %q", got)
	}
}

func itoa(n int64) string {
	b := []byte{}
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
