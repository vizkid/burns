package main

import (
	"os"
	"strings"
	"testing"
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
	if got := roarBind(run, "t1", 20, "/h", 0); got != "roar bound block:b1" || n != 2 {
		t.Fatalf("got %q after %d calls", got, n)
	}
}

func TestRoarBindDoesNotRetryOtherRefusals(t *testing.T) {
	var n int
	run := func([]string) (string, int) { n++; return "roar: thread_bound_elsewhere\n", 2 }
	if got := roarBind(run, "t1", 20, "/h", 0); got != "roar refused: roar: thread_bound_elsewhere" || n != 1 {
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
