package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The folder travels relative to home, because each host has its own home.
func TestSpawnDirIsRelativeToHome(t *testing.T) {
	for _, c := range []struct{ home, cwd, want string }{
		{"/Users/u", "/Users/u/dev/proj", "dev/proj"},
		{"/Users/u", "/Users/u", "."},
		{"/Users/u", "/opt/shared", "/opt/shared"},
		{"/Users/u", "/Users/user2/x", "/Users/user2/x"},
	} {
		if got := homeRelative(c.home, c.cwd); got != c.want {
			t.Errorf("homeRelative(%q, %q) = %q; want %q", c.home, c.cwd, got, c.want)
		}
	}
}

// launch --dir resolves a relative folder against this host's home.
func TestLaunchDirResolvesAgainstHome(t *testing.T) {
	if got := launchDir("/home/u", "dev/proj"); got != "/home/u/dev/proj" {
		t.Errorf("relative = %q", got)
	}
	if got := launchDir("/home/u", "/opt/x"); got != "/opt/x" {
		t.Errorf("absolute = %q", got)
	}
}

// Roar's address: pane@session, with host: in front for another host.
func TestSpawnAddress(t *testing.T) {
	if got := roarAddress("", "helper", "proj"); got != "helper@proj" {
		t.Errorf("local = %q", got)
	}
	if got := roarAddress("host-2", "helper", "proj"); got != "host-2:helper@proj" {
		t.Errorf("remote = %q", got)
	}
}

// Spawn refuses before touching Rex when it is not in a pane or lacks a label.
func TestSpawnRefusesBeforeRex(t *testing.T) {
	for _, c := range []struct {
		args []string
		env  map[string]string
		want string
	}{
		{[]string{"--provider", "claude", "--account", "a", "--label", "x", "--tab", "t"}, nil, "Rex pane"},
		{[]string{"--provider", "claude", "--account", "a", "--label", "x"}, map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}, "--tab"},
		{[]string{"--provider", "claude", "--account", "a"}, map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}, "--label"},
		{[]string{"--provider", "gemini", "--account", "a", "--label", "x", "--tab", "t"}, map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}, "provider"},
		{[]string{"--provider", "claude", "--account", "a", "--label", "x", "--tab", "t", "--anchor", "nav"}, map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}, "--tab"},
		{[]string{"--provider", "claude", "--account", "a", "--label", "x", "--tab", "t", "--beside"}, map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}, "--tab"},
	} {
		var out, errOut bytes.Buffer
		code := run(append([]string{"spawn"}, c.args...), strings.NewReader(""), &out, &errOut, env(c.env))
		if code != 1 && code != 2 || !strings.Contains(errOut.String(), c.want) {
			t.Errorf("%v: code %d, stderr %q; want it to mention %q", c.args, code, errOut.String(), c.want)
		}
	}
}

// A subscription with no quota windows is unknown, which warns like a spent one.
// Azure has no windows by design and stays quiet (peer review, 2026-11-03).
func TestSpawnWarnsOnUnknownQuota(t *testing.T) {
	body := `{"host":"h1","accounts":[
{"provider":"claude","account":"fresh","kind":"subscription","limits":[]},
{"provider":"claude","account":"ok","kind":"subscription","limits":[{"limit_id":"claude","windows":{"10080":{"percent":40,"resets_at":"2026-11-02T10:00:00Z","observed_at":"2026-10-29T11:59:00Z"}}}]},
{"provider":"claude","account":"spent","kind":"subscription","limits":[{"limit_id":"claude","windows":{"300":{"percent":100,"resets_at":"2026-11-02T10:00:00Z","observed_at":"2026-10-29T11:59:00Z"}}}]},
{"provider":"codex","account":"azure","kind":"azure","limits":[]}]}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	t.Cleanup(ts.Close)
	for _, c := range []struct {
		provider, name, want string
	}{{"claude", "fresh", "unknown"}, {"claude", "ok", ""}, {"claude", "spent", "at 100%"}, {"codex", "azure", ""}} {
		var errOut bytes.Buffer
		warnQuota(ts.URL, c.provider, c.name, &errOut)
		got := errOut.String()
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s %s: warned %q; want %q", c.provider, c.name, got, c.want)
		}
	}
}
