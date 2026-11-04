package main

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/vizkid/burns/internal/codexd"
)

func TestCodexPidIsTheLaunchOrItsChild(t *testing.T) {
	// 10 is Rex, 20 the launch (now Codex), 30 a child of it (cop's Codex), 40 elsewhere.
	ps := []byte("10 1\n20 10\n30 20\n40 10\n")
	cases := []struct {
		fg   int
		want bool
	}{{20, true}, {30, true}, {40, false}, {10, false}, {0, false}}
	for _, c := range cases {
		if got := underLaunch(ps, c.fg, 20); got != c.want {
			t.Errorf("foreground %d: got %v", c.fg, got)
		}
	}
}

type roarCall struct {
	out  string
	code int
}

func TestRoarBindRetriesOnceWhenCodexIsNotYetForeground(t *testing.T) {
	calls := []roarCall{{"roar: pid_not_foreground: foreground is 7", 2}, {"bound block:b1", 0}}
	var n int
	run := func(args []string) (string, int) {
		if strings.Join(args, " ") != "bind codex --thread t1 --pid 20 --codex-home /h" {
			t.Fatalf("args %q", args)
		}
		c := calls[n]
		n++
		return c.out, c.code
	}
	if got, ok := roarBind(run, "t1", 20, "/h", 0); got != "roar bound block:b1" || !ok || n != 2 {
		t.Fatalf("got %q after %d calls", got, n)
	}
}

func TestRoarBindDoesNotRetryOtherRefusals(t *testing.T) {
	var n int
	run := func([]string) (string, int) { n++; return "roar: thread_bound_elsewhere\n", 2 }
	if got, ok := roarBind(run, "t1", 20, "/h", 0); got != "roar refused: roar: thread_bound_elsewhere" || ok || n != 1 {
		t.Fatalf("got %q after %d calls", got, n)
	}
}

func TestCodexWatchCommandCarriesTheLaunch(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	cmd := codexWatchCommand("/bin/smithers", codexLaunch{Home: "/h", Block: "block:b1", Session: "session:s1", Dir: "/w/a", Pid: 20}, []string{"PATH=/x"}, w)
	want := "/bin/smithers bind-codex --home /h --block block:b1 --session session:s1 --dir /w/a --pid 20"
	if got := strings.Join(cmd.Args, " "); got != want {
		t.Fatalf("args %q", got)
	}
	if len(cmd.ExtraFiles) != 1 || cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatal("the watcher needs its ready pipe and its own session")
	}
}

// fakeBinder records what a binding would send. Its pane runs Codex as the launch itself.
type fakeBinder struct {
	roarOut   string
	roarCode  int
	noRoar    bool
	rival     string
	roarCalls int
	claims    []map[string]any
}

func (f *fakeBinder) binder() binder {
	b := binder{
		foreground: func(context.Context, string) (int, error) { return 20, nil },
		ps:         func() ([]byte, error) { return []byte("10 1\n20 10\n"), nil },
		rivals:     func(context.Context, codexLaunch) (string, error) { return f.rival, nil },
		claim: func(body []byte) {
			var m map[string]any
			json.Unmarshal(body, &m)
			f.claims = append(f.claims, m)
		},
	}
	if !f.noRoar {
		b.roar = func([]string) (string, int) { f.roarCalls++; return f.roarOut, f.roarCode }
	}
	return b
}

var bindLaunch = codexLaunch{Home: "/h", Block: "block:b1", Session: "session:s1", Dir: "/w/a", Pid: 20}
var bindThreads = []codexd.Thread{{ID: "t1", SessionID: "t1", Cwd: "/w/a", Originator: "codex-tui"}}

func TestABoundThreadIsClaimed(t *testing.T) {
	f := &fakeBinder{roarOut: "bound block:b1"}
	f.binder().bind(context.Background(), bindLaunch, bindThreads, true)
	if len(f.claims) != 1 || f.claims[0]["session"] != "t1" || f.claims[0]["rex_block"] != "block:b1" {
		t.Fatalf("claims %v", f.claims)
	}
}

// Review P1, 2026-11-04: a thread roar says is another pane's must not move that pane's tokens.
func TestAThreadBoundElsewhereIsNotClaimed(t *testing.T) {
	for _, reason := range []string{"thread_bound_elsewhere", "invalid_thread"} {
		f := &fakeBinder{roarOut: "roar: " + reason, roarCode: 2}
		got := f.binder().bind(context.Background(), bindLaunch, bindThreads, true)
		if len(f.claims) != 0 || !strings.Contains(got, reason) {
			t.Errorf("%s: claims %v, said %q", reason, f.claims, got)
		}
	}
}

// A wrapper in the foreground is roar's refusal about the pane, not the thread: the tokens are
// still this pane's.
func TestAWrapperPaneIsStillClaimed(t *testing.T) {
	f := &fakeBinder{roarOut: "roar: not_codex_pane: the pane's foreground program is not codex", roarCode: 2}
	f.binder().bind(context.Background(), bindLaunch, bindThreads, true)
	if len(f.claims) != 1 {
		t.Fatalf("claims %v", f.claims)
	}
}

// Another Codex pane in the same folder that roar has not bound could own the thread.
func TestARivalPaneBindsNothing(t *testing.T) {
	f := &fakeBinder{roarOut: "bound block:b1", rival: "unbound Codex pane block:b2 in /w/a"}
	got := f.binder().bind(context.Background(), bindLaunch, bindThreads, true)
	if f.roarCalls != 0 || len(f.claims) != 0 || !strings.Contains(got, "block:b2") {
		t.Fatalf("roar %d, claims %v, said %q", f.roarCalls, f.claims, got)
	}
}

// Review finding 5: a window the daemon cut short may have missed a second thread.
func TestAnIncompleteWindowBindsNothing(t *testing.T) {
	f := &fakeBinder{roarOut: "bound block:b1"}
	got := f.binder().bind(context.Background(), bindLaunch, bindThreads, false)
	if f.roarCalls != 0 || len(f.claims) != 0 || !strings.Contains(got, "window") {
		t.Fatalf("roar %d, claims %v, said %q", f.roarCalls, f.claims, got)
	}
}

// Without roar there is no rival check, so nothing is bound.
func TestNoRoarBindsNothing(t *testing.T) {
	f := &fakeBinder{noRoar: true}
	f.binder().bind(context.Background(), bindLaunch, bindThreads, true)
	if len(f.claims) != 0 {
		t.Fatalf("claims %v", f.claims)
	}
}

func TestUnboundCodexPanesAreRivals(t *testing.T) {
	panes := []byte(`[{"block":"block:b1","agent":"codex","path":"typed"},
		{"block":"block:b2","agent":"codex","path":"typed"},
		{"block":"block:b3","agent":"codex","path":"codex_native"},
		{"block":"block:b4","agent":"claude","path":"channel"}]`)
	got, err := unboundCodex(panes, "block:b1")
	if err != nil || !slices.Equal(got, []string{"block:b2"}) {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := unboundCodex([]byte("not json"), "block:b1"); err == nil {
		t.Fatal("a list roar did not give must not read as no rivals")
	}
}
