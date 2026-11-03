package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

// Shaped like a live report, with the amounts changed.
const codexReport = `{"units":"credits","data_freshness_ts":"2026-11-02T02:00:00Z","group_by":"day","data":[
 {"date":"2026-10-30","premium_usage_values":{"total_usage_credits":{"cli":200},"credit_usage_credits":{"cli":150.5,"vscode":49.5}}},
 {"date":"2026-10-31","premium_usage_values":{"credit_usage_credits":{"cli":0}}},
 {"date":"2026-11-01","premium_usage_values":{"credit_usage_credits":{"cli":100}}}]}`

// The month holds only its own days. The running total crosses months, so a window can span them.
func TestCodexReport(t *testing.T) {
	days, fresh, err := parseCodex([]byte(codexReport))
	if err != nil {
		t.Fatal(err)
	}
	a := config.Account{Provider: "codex", Name: "alpha", CreditPrice: &config.CreditPrice{Currency: "SGD", Amount: 0.05}}
	b, points := codexBilled(a, days, fresh, now)
	if b.Currency != "SGD" || b.UsedMinor != 500 || b.Source != SourceCodex || !b.ObservedAt.Equal(fresh) { // 100 credits at 5 cents
		t.Fatalf("billed = %+v", b)
	}
	if len(points) != 3 || points[0].UsedMinor != 1000 || points[2].UsedMinor != 1500 {
		t.Fatalf("points = %+v", points)
	}
	a.CreditPrice = nil
	if b, _ = codexBilled(a, days, fresh, now); b.Currency != "credits" || b.UsedMinor != 10000 {
		t.Fatalf("no price = %+v", b)
	}
}

// A report in another shape is no reading, never zero.
func TestCodexUnknownShape(t *testing.T) {
	// A row without credit_usage_credits says nothing about that day. A real idle day lists
	// each surface at 0, so a missing or null map is an unknown shape, not a zero.
	day := `{"units":"credits","data_freshness_ts":"2026-11-02T02:00:00Z","data":[{"date":"2026-11-01"%s}]}`
	for _, body := range []string{`{}`, `{"units":"tokens","data_freshness_ts":"2026-11-02T02:00:00Z","data":[]}`, `{"units":"credits","data":[]}`, `nope`,
		fmt.Sprintf(day, ``), fmt.Sprintf(day, `,"premium_usage_values":{}`), fmt.Sprintf(day, `,"premium_usage_values":{"credit_usage_credits":null}`)} {
		if _, _, err := parseCodex([]byte(body)); err == nil {
			t.Errorf("parsed %s", body)
		}
	}
}

// The poller sends the login and the account, and a restated day replaces the old one.
func TestPollCodex(t *testing.T) {
	var auth, acct string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, acct = r.Header.Get("Authorization"), r.Header.Get("ChatGPT-Account-Id")
		w.Write([]byte(codexReport))
	}))
	defer srv.Close()
	p, st := poller(t, srv.URL, func(string) (string, error) { return "", ErrNoToken })
	p.CodexURL = srv.URL
	p.CodexLogin = func(string) (string, string, error) { return "tok", "acct", nil }
	p.Poll(context.Background())
	p.Poll(context.Background())
	got, err := st.BilledAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Provider != "codex" || got[0].UsedMinor != 10000 || auth != "Bearer tok" || acct != "acct" {
		t.Fatalf("billed = %+v, auth %q, account %q", got, auth, acct)
	}
	h, _ := st.BilledHistory("codex", "alpha", now.Add(-24*time.Hour))
	if len(h) != 2 || h[0].UsedMinor != 20000 || h[1].UsedMinor != 30000 { // The last day before the window, then the day inside it.
		t.Fatalf("history = %+v", h)
	}
}

func TestCodexLoginFile(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := StoredCodexLogin(dir); !errors.Is(err, ErrNoToken) {
		t.Fatalf("no file: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"tok","account_id":"acct"}}`), 0o600)
	if tok, acct, err := StoredCodexLogin(dir); err != nil || tok != "tok" || acct != "acct" {
		t.Fatalf("%q %q %v", tok, acct, err)
	}
}

func itoa(n int64) string {
	b := []byte{}
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
