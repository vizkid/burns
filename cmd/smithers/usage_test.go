package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Shapes from smithersd's GET /usage and GET /usage/sessions, with synthetic accounts.
const usageBody = `{"host":"h1","accounts":[
{"provider":"claude","account":"alpha","kind":"subscription","limits":[{"limit_id":"claude","windows":{
 "10080":{"percent":22,"state":"live","resets_at":"2026-11-02T10:00:00Z","observed_at":"2026-10-29T11:59:00Z"},
 "300":{"percent":3,"state":"live","resets_at":"2026-10-29T15:20:00Z","observed_at":"2026-10-29T11:59:00Z"}}}]},
{"provider":"claude","account":"beta","kind":"subscription","limits":[]},
{"provider":"codex","account":"alpha","kind":"subscription","limits":[{"limit_id":"codex","windows":{
 "10080":{"percent":null,"state":"unknown","resets_at":"2026-10-28T06:00:00Z","observed_at":"2026-10-21T06:00:00Z"}}}]}]}`

const sessionsBody = `{"host":"h1","sessions":[
{"provider":"claude","account":"alpha","session":"s1","requests":400,"input":900,"cached_input":90000000,"cache_write":1600000,"output":400000,"cost_micros":null,"unknown_cost":400},
{"provider":"claude","account":"alpha","session":"s2","requests":10,"input":100,"cached_input":0,"cache_write":0,"output":1000,"cost_micros":1250000,"unknown_cost":0},
{"provider":"codex","account":"alpha","session":"s3","requests":55,"input":212777,"cached_input":10925568,"cache_write":0,"output":6917,"cost_micros":null,"unknown_cost":55}],"blocks":[]}`

func usageServer(t *testing.T, since *string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/usage":
			w.Write([]byte(usageBody))
		case "/usage/sessions":
			*since = r.URL.Query().Get("since")
			w.Write([]byte(sessionsBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func runUsage(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	now = func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) }
	local = time.UTC
	t.Cleanup(func() { now, local = time.Now, time.Local })
	var out, errOut bytes.Buffer
	code := run(append([]string{"usage"}, args...), strings.NewReader(""), &out, &errOut, env(nil))
	return out.String(), errOut.String(), code
}

func TestUsagePrintsQuotaAndConsumption(t *testing.T) {
	var since string
	ts := usageServer(t, &since)
	out, errOut, code := runUsage(t, "--url", ts.URL, "--since", "6h")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if since != "6h" {
		t.Errorf("since = %q; want 6h passed through", since)
	}
	for _, want := range []string{
		"5h 3% resets 15:20",      // same day: time only
		"7d 22% resets Mon 10:00", // within a week: weekday
		"seen 1m ago",             // newest observation
		"7d unknown",              // a passed reset is unknown, not 0%
		"Last 6h",                 // the window heading
		"$1.25 + 400 unknown",     // known cost plus the count without one
		"unknown (55)",            // no known cost is never $0
		"91.6M",                   // cached input plus cache writes
		"none seen",               // an account with no sessions
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if l := lineOf(out, "claude beta"); !strings.Contains(l, "unknown") {
		t.Errorf("claude beta quota = %q; no limits is unknown, not 0%%", l)
	}
	if strings.Contains(out, "$0") {
		t.Errorf("output shows $0 for unknown cost:\n%s", out)
	}
	// Accounts print in settings order, quota before consumption.
	if a, b := strings.Index(out, "claude alpha"), strings.Index(out, "codex alpha"); a < 0 || b < a {
		t.Errorf("accounts out of settings order:\n%s", out)
	}
}

func TestUsageFailsWhenSmithersdIsDown(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	ts.Close()
	out, errOut, code := runUsage(t, "--url", ts.URL)
	if code != 1 || out != "" || !strings.Contains(errOut, "smithersd") {
		t.Fatalf("exit %d, stdout %q, stderr %q; want 1, nothing, and a message naming smithersd", code, out, errOut)
	}
}

func TestUsageRejectsBadSince(t *testing.T) {
	_, errOut, code := runUsage(t, "--since", "soon")
	if code != 2 || !strings.Contains(errOut, "since") {
		t.Fatalf("exit %d, stderr %q; want 2 and a message about --since", code, errOut)
	}
}

func lineOf(out, prefix string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}
