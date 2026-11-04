package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vizkid/burns/internal/codexd"
	"github.com/vizkid/burns/internal/rex"
)

// A Codex launch binds its thread to its pane by watching the daemon for the
// thread the TUI starts (docs/SPEC.md, "Binding a Codex launch to its pane").
const (
	// codexWindow is how long the watcher collects thread/started. The TUI
	// started its thread 2.3 s after launch in the 2026-11-04 test.
	codexWindow = 10 * time.Second
	// readyWait bounds the launch's wait for the watcher to connect. A launch
	// never waits longer for a binding.
	readyWait = 3 * time.Second
	// roarRetry is the wait before the one retry of pid_not_foreground.
	roarRetry = time.Second
	// claimTimeout bounds the post to smithersd, which checks the claim with Rex.
	claimTimeout = 5 * time.Second
)

// codexLaunch is what the watcher needs from the launch.
type codexLaunch struct {
	Home, Block, Session, Dir string
	Pid                       int
}

// startCodexWatch leaves a watcher connected to the daemon, then returns, so
// the TUI's thread/started cannot come before the watcher listens.
func startCodexWatch(l codexLaunch, env []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	defer r.Close()
	cmd := codexWatchCommand(self, l, env, w)
	err = cmd.Start()
	w.Close()
	if err != nil {
		return err
	}
	cmd.Process.Release()
	r.SetReadDeadline(time.Now().Add(readyWait))
	line := make([]byte, 16)
	n, err := r.Read(line)
	if string(line[:n]) != "ready\n" {
		return fmt.Errorf("watcher did not connect: %v", err)
	}
	return nil
}

// codexWatchCommand passes the ready pipe as fd 3, not stdout: a write to a
// pipe the launch has closed then fails instead of killing the watcher.
func codexWatchCommand(self string, l codexLaunch, env []string, ready *os.File) *exec.Cmd {
	cmd := exec.Command(self, "bind-codex", "--home", l.Home, "--block", l.Block, "--session", l.Session,
		"--dir", l.Dir, "--pid", strconv.Itoa(l.Pid))
	cmd.Env, cmd.ExtraFiles = env, []*os.File{ready}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

func bindCodex(args []string, errOut io.Writer) int {
	fl := flag.NewFlagSet("bind-codex", flag.ContinueOnError)
	fl.SetOutput(errOut)
	var l codexLaunch
	fl.StringVar(&l.Home, "home", "", "Codex home")
	fl.StringVar(&l.Block, "block", "", "Rex block")
	fl.StringVar(&l.Session, "session", "", "Rex session")
	fl.StringVar(&l.Dir, "dir", "", "launch folder")
	fl.IntVar(&l.Pid, "pid", 0, "launch process")
	if fl.Parse(args) != nil || l.Home == "" || l.Block == "" || l.Dir == "" || l.Pid == 0 {
		return 2
	}
	ready := os.NewFile(3, "ready")
	ctx := context.Background()
	conn, err := codexd.Dial(ctx, l.Home)
	if err != nil {
		ready.Close()
		logLaunch(fmt.Sprintf("%s: codex binding left to roar's first message: daemon: %v", l.Block, err))
		return 1
	}
	ready.WriteString("ready\n")
	ready.Close()
	threads := conn.Started(ctx, codexWindow)
	conn.Close()
	logLaunch(l.Block + ": " + bindThread(ctx, l, threads))
	return 0
}

// bindThread tells roar and smithersd which thread runs in the pane, and says
// what happened. It binds nothing on a guess.
func bindThread(ctx context.Context, l codexLaunch, threads []codexd.Thread) string {
	t, why := codexd.Pick(threads, l.Dir)
	if why != "" {
		return "codex binding left to roar's first message: " + why
	}
	pid, why := codexPid(ctx, l)
	if why != "" {
		return "codex thread " + t.ID + " not bound: " + why
	}
	said := "roar not found"
	if path, _, err := find("roar", os.Environ()); err == nil {
		said = roarBind(runner(path), t.ID, pid, l.Home, roarRetry)
	}
	body, _ := json.Marshal(map[string]any{"provider": "codex", "session": t.Session(), "source": "launch",
		"rex_block": l.Block, "rex_session": l.Session, "pids": []int{pid}})
	post(DefaultURL+"/bind", body, claimTimeout)
	return fmt.Sprintf("codex thread %s, pid %d: %s; claim sent to smithersd", t.ID, pid, said)
}

// codexPid is the pane's foreground, which must be the launch itself (exec kept
// its pid) or run under it, as Codex does under cop.
func codexPid(ctx context.Context, l codexLaunch) (int, string) {
	path, _, err := find("rex", os.Environ())
	if err != nil {
		return 0, "rex not found"
	}
	ctx, cancel := context.WithTimeout(ctx, claimTimeout)
	defer cancel()
	fg, err := rex.CLI{Path: path}.Foreground(ctx, l.Block)
	if err != nil {
		return 0, "rex: " + err.Error()
	}
	ps, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		return 0, "ps: " + err.Error()
	}
	if !underLaunch(ps, fg, l.Pid) {
		return 0, fmt.Sprintf("the pane's foreground %d is not this launch (%d)", fg, l.Pid)
	}
	return fg, ""
}

func underLaunch(ps []byte, fg, launch int) bool {
	return fg > 0 && (fg == launch || slices.Contains(chain(ps, fg), launch))
}

func runner(path string) func([]string) (string, int) {
	return func(args []string) (string, int) {
		out, err := exec.Command(path, args...).CombinedOutput()
		var ee *exec.ExitError
		switch {
		case err == nil:
			return string(out), 0
		case errors.As(err, &ee):
			return string(out), ee.ExitCode()
		default:
			return err.Error(), -1
		}
	}
}

// roarBind runs `roar bind codex`. Only pid_not_foreground is retried, once: it
// can mean Codex has not yet replaced its launcher.
func roarBind(run func([]string) (string, int), thread string, pid int, home string, wait time.Duration) string {
	args := []string{"bind", "codex", "--thread", thread, "--pid", strconv.Itoa(pid), "--codex-home", home}
	out, code := run(args)
	if code != 0 && strings.Contains(out, "pid_not_foreground") {
		time.Sleep(wait)
		out, code = run(args)
	}
	out = strings.TrimSpace(out)
	if code != 0 {
		return "roar refused: " + out
	}
	return "roar " + out
}
