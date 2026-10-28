package rex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Shapes captured from `rex events --all --json` on 2026-10-28, with synthetic IDs.
func TestParseEvent(t *testing.T) {
	cases := []struct {
		line string
		want Event
		ok   bool
	}{
		{`{"type":"session_event","payload":{"session_id":"session:s1","event":{"type":"block_closed","payload":{"block_id":"block:b1"}}}}`,
			Event{Kind: BlockClosed, Session: "session:s1", Block: "block:b1"}, true},
		{`{"type":"session_event","payload":{"session_id":"session:s1","event":{"type":"block_event","payload":{"block_id":"block:b2","creator_name":"com.superlogical.terminal","name":"process_changed","payload":{"generation":6,"foreground":null}}}}}`,
			Event{Kind: Foreground, Session: "session:s1", Block: "block:b2"}, true},
		{`{"type":"session_event","payload":{"session_id":"session:s1","event":{"type":"block_event","payload":{"block_id":"block:b3","name":"process_changed","payload":{"generation":43,"foreground":{"pid":700,"name":"node"},"ancestors":[{"pid":600,"name":"zsh"},{"pid":500,"name":"login"}]}}}}}`,
			Event{Kind: Foreground, Session: "session:s1", Block: "block:b3", Pid: 700, Ancestors: []int{600, 500}}, true},
		{`{"type":"session_destroyed","payload":{"session_id":"session:s9"}}`,
			Event{Kind: SessionDestroyed, Session: "session:s9"}, true},
		{`{"type":"session_event","payload":{"session_id":"session:s1","event":{"type":"block_event","payload":{"block_id":"block:b1","name":"title_changed","payload":{}}}}}`, Event{}, false},
		{`{"type":"client_joined","payload":{"client":{}}}`, Event{}, false},
		{`Error: interrupted`, Event{}, false},
		{``, Event{}, false},
	}
	for _, c := range cases {
		got, ok := ParseEvent([]byte(c.line))
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseEvent(%.60s) = %+v, %v; want %+v, %v", c.line, got, ok, c.want, c.ok)
		}
	}
}

// fakeRex writes a stand-in rex CLI that answers `block call --block B process`.
func fakeRex(t *testing.T, script string) CLI {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return CLI{Path: path}
}

func TestForeground(t *testing.T) {
	rex := fakeRex(t, `[ "$1 $2 $3 $4 $5" = "block call --block block:b1 process" ] || exit 9
echo '{"child":{"pid":10},"foreground":{"pid":42,"name":"2.1.312"},"last_exit":null}'`)
	pid, err := rex.Foreground(context.Background(), "block:b1")
	if err != nil || pid != 42 {
		t.Fatalf("Foreground = %d, %v; want 42", pid, err)
	}
}

func TestForegroundNone(t *testing.T) {
	rex := fakeRex(t, `echo '{"child":{"pid":10},"foreground":null}'`)
	if pid, err := rex.Foreground(context.Background(), "block:b1"); err != nil || pid != 0 {
		t.Fatalf("Foreground = %d, %v; want 0 for no foreground", pid, err)
	}
}

func TestForegroundMissingBlock(t *testing.T) {
	rex := fakeRex(t, `echo 'Error: block target "block:b1" not found' >&2; exit 3`)
	if _, err := rex.Foreground(context.Background(), "block:b1"); !errors.Is(err, ErrNoBlock) {
		t.Fatalf("err = %v; want ErrNoBlock", err)
	}
}

func TestForegroundOtherFailureIsNotMissing(t *testing.T) {
	rex := fakeRex(t, `echo 'Error: server unavailable' >&2; exit 1`)
	if _, err := rex.Foreground(context.Background(), "block:b1"); err == nil || errors.Is(err, ErrNoBlock) {
		t.Fatalf("err = %v; want an error that is not ErrNoBlock", err)
	}
}

func TestWatchDeliversEventsUntilExit(t *testing.T) {
	rex := fakeRex(t, `[ "$*" = "events --all --json" ] || exit 9
echo '{"type":"session_destroyed","payload":{"session_id":"session:s1"}}'
echo 'noise'
echo '{"type":"session_event","payload":{"session_id":"session:s1","event":{"type":"block_closed","payload":{"block_id":"block:b1"}}}}'`)
	var got []Kind
	err := Watch(context.Background(), rex.Path, func(e Event) { got = append(got, e.Kind) })
	if err != nil || !reflect.DeepEqual(got, []Kind{SessionDestroyed, BlockClosed}) {
		t.Fatalf("Watch = %v, events %v", err, got)
	}
}

// Inside a Rex pane, REX_SESSION scopes block lookups to that session, so other
// sessions' blocks read "not found". The CLI must not pass the pane's identity on.
func TestCLIDropsPaneIdentity(t *testing.T) {
	t.Setenv("REX_SESSION", "session:mine")
	t.Setenv("REX_BLOCK", "block:mine")
	rex := fakeRex(t, `[ -z "$REX_SESSION$REX_BLOCK" ] || { echo 'Error: block target not found' >&2; exit 3; }
echo '{"foreground":{"pid":7}}'`)
	if pid, err := rex.Foreground(context.Background(), "block:other"); err != nil || pid != 7 {
		t.Fatalf("Foreground = %d, %v; want 7", pid, err)
	}
}
