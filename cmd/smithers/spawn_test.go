package main

import (
	"bytes"
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
		{[]string{"--provider", "claude", "--account", "a", "--label", "x"}, nil, "Rex pane"},
		{[]string{"--provider", "claude", "--account", "a"}, map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}, "--label"},
		{[]string{"--provider", "gemini", "--account", "a", "--label", "x"}, map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}, "provider"},
	} {
		var out, errOut bytes.Buffer
		code := run(append([]string{"spawn"}, c.args...), strings.NewReader(""), &out, &errOut, env(c.env))
		if code != 1 && code != 2 || !strings.Contains(errOut.String(), c.want) {
			t.Errorf("%v: code %d, stderr %q; want it to mention %q", c.args, code, errOut.String(), c.want)
		}
	}
}
