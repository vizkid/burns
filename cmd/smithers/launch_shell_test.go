package main

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/vizkid/burns/internal/config"
)

// docs/SPEC.md, "Shell launchers".
func TestTitleNamesIconFolderAndAccount(t *testing.T) {
	a := config.Account{Provider: "claude", Name: "nfx", Emoji: "◆"}
	for _, c := range []struct{ dir, emoji, want string }{
		{"/home/op/dev/burns", "◆", "\x1b]0;◆ burns · claude nfx\x1b\\"},
		{"/home/op", "◆", "\x1b]0;◆ ~ · claude nfx\x1b\\"},
		{"/home/op/dev/burns", "", "\x1b]0;burns · claude nfx\x1b\\"},
		{"/home/op/dev/b\x07ad\x1b", "", "\x1b]0;bad · claude nfx\x1b\\"},
	} {
		a.Emoji = c.emoji
		if got := title(a, c.dir, "/home/op"); got != c.want {
			t.Errorf("%q: got %q, want %q", c.dir, got, c.want)
		}
	}
}

func TestArgumentsAfterTheMarkerReachTheAgent(t *testing.T) {
	fakeHost(t)
	var argv []string
	replace = func(_ string, a, _ []string) error { argv = a; return nil }
	t.Cleanup(func() { replace = nil })
	var errOut bytes.Buffer
	code := launchAgent([]string{"--no-pause", "--provider", "claude", "--account", "alpha", "--", "-p", "--model x"}, strings.NewReader(""), &errOut, os.Getenv)
	if code != 0 || !slices.Equal(argv, []string{"claude", "--effort", "low", "-p", "--model x"}) {
		t.Fatalf("code %d argv %q: %s", code, argv, errOut.String())
	}
}

// A shell launcher stays in its shell, so a failure returns at once.
func TestNoPauseExitsWithoutWaiting(t *testing.T) {
	fakeHost(t)
	replace = func(string, []string, []string) error { return errors.New("must not run") }
	t.Cleanup(func() { replace = nil })
	in := &countingReader{}
	var errOut bytes.Buffer
	code := launchAgent([]string{"--no-pause", "--provider", "claude", "--account", "nobody"}, in, &errOut, os.Getenv)
	if code != 1 || in.reads != 0 || strings.Contains(errOut.String(), "Press Return") {
		t.Fatalf("code %d, reads %d: %s", code, in.reads, errOut.String())
	}
}

type countingReader struct{ reads int }

func (r *countingReader) Read(p []byte) (int, error) { r.reads++; p[0] = '\n'; return 1, nil }
