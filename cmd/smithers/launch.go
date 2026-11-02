package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/launch"
	"github.com/vizkid/burns/internal/rex"
)

// daemonTimeout bounds `codex app-server daemon start`, which returns at once
// when a daemon runs.
const daemonTimeout = 20 * time.Second

// shellTimeout bounds asking the interactive shell for its PATH.
const shellTimeout = 10 * time.Second

// pipeGrace bounds the wait for an output pipe after a command exits. A forked
// daemon or an rc file's background job keeps the pipe open, and only WaitDelay
// ends that wait: the context's timeout kills the direct child alone.
const pipeGrace = 2 * time.Second

// replace becomes the CLI. It is a variable so tests stop short of it.
var replace = syscall.Exec

// launchAgent runs in the new Rex tab. A failure holds the tab open until a key
// press: an exit would close the tab and lose the reason.
func launchAgent(args []string, in io.Reader, errOut io.Writer, getenv func(string) string) int {
	if err := startAgent(args, errOut, getenv); err != nil {
		fmt.Fprintln(errOut, "burns:", err)
		fmt.Fprintln(errOut, "Press Return to close.")
		io.ReadFull(in, make([]byte, 1))
		return 1
	}
	return 0
}

func startAgent(args []string, errOut io.Writer, getenv func(string) string) error {
	fl := flag.NewFlagSet("launch", flag.ContinueOnError)
	fl.SetOutput(errOut)
	provider := fl.String("provider", "", "claude or codex")
	name := fl.String("account", "", "account name")
	model := fl.String("model", "", "model, replacing the account's")
	effort := fl.String("effort", "", "effort, replacing the account's")
	permission := fl.String("permission-mode", "", "permission mode, replacing the account's")
	if err := fl.Parse(args); err != nil {
		return err
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := config.Dir(user)
	cfg, err := config.Load(filepath.Join(dir, "accounts.json"), user)
	if err != nil {
		return err
	}
	a, ok := cfg.Find(*provider, *name)
	if !ok {
		return fmt.Errorf("no %s account named %q on this host", *provider, *name)
	}
	if err := override(&a, *model, *effort, *permission); err != nil {
		return err
	}
	home := cfg.HomeDir(a)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	tools, problems := launch.LoadTools(filepath.Join(dir, "tools.d"))
	for _, p := range problems {
		fmt.Fprintln(errOut, "burns:", p)
	}
	plan := launch.Build(launch.Input{
		Account: a, Home: home, UserHome: user, Channels: launch.Channels(tools, a.Provider),
		Block: getenv("REX_BLOCK"), Session: getenv("REX_SESSION"),
	})
	return startPlan(plan, errOut, getenv("REX_BLOCK"))
}

// override applies the launch window's choices, then checks them as the accounts
// file is checked: they become arguments.
func override(a *config.Account, model, effort, permission string) error {
	l := config.Launch{}
	if a.Launch != nil {
		l = *a.Launch
	}
	for _, s := range []struct {
		field *string
		value string
	}{{&l.Model, model}, {&l.Effort, effort}, {&l.PermissionMode, permission}} {
		if s.value != "" {
			*s.field = s.value
		}
	}
	a.Launch = &l
	if problem := config.CheckLaunch(*a); problem != "" {
		return errors.New(problem)
	}
	return nil
}

func startPlan(plan launch.Plan, errOut io.Writer, block string) error {
	env := plan.Environ(os.Environ())
	// The CLI may sit where only the interactive shell looks, even under cop.
	if _, full, err := find(plan.Provider, env); err == nil {
		env = full
	}
	if len(plan.Daemon) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), daemonTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, resolved(plan.Daemon[0], env), plan.Daemon[1:]...)
		cmd.Env, cmd.WaitDelay = env, pipeGrace
		// ErrWaitDelay means the command succeeded and left a daemon holding the pipe.
		if out, err := cmd.CombinedOutput(); err != nil && !errors.Is(err, exec.ErrWaitDelay) {
			fmt.Fprintf(errOut, "burns: codex daemon: %v: %s\n", err, strings.TrimSpace(string(out)))
		}
	}
	path, env, err := find(plan.Program, env)
	if err != nil {
		return err
	}
	if len(plan.Channels) > 0 && block != "" {
		if err := startWatch(block, plan.Channels, env); err != nil {
			fmt.Fprintln(errOut, "burns: channels dialog will wait for you:", err)
		}
	}
	return replace(path, append([]string{plan.Program}, plan.Args...), env)
}

// find locates a program. Rex runs commands without the interactive shell, where
// version managers add their folders, so a miss asks that shell for its PATH.
func find(program string, env []string) (string, []string, error) {
	if path, err := lookIn(program, pathOf(env)); err == nil {
		return path, env, nil
	}
	shell := valueOf(env, "SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-ic", `printf '\n%s' "$PATH"`)
	cmd.Env, cmd.WaitDelay = env, pipeGrace
	out, err := cmd.Output()
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return "", env, fmt.Errorf("%s not found: %w", program, err)
	}
	lines := strings.Split(string(out), "\n")
	full := lines[len(lines)-1]
	path, err := lookIn(program, full)
	if err != nil {
		return "", env, err
	}
	return path, withVar(env, "PATH", full), nil
}

// withVar sets a variable once. A process reads the first value and exec does
// not dedupe, so appending would leave the old value in charge.
func withVar(env []string, name, value string) []string {
	out := slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, name+"=") })
	return append(out, name+"="+value)
}

func lookIn(program, list string) (string, error) {
	for _, dir := range filepath.SplitList(list) {
		path := filepath.Join(dir, program)
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s not found on PATH", program)
}

func pathOf(env []string) string { return valueOf(env, "PATH") }

// valueOf reads the first value of a variable, the one getenv returns.
func valueOf(env []string, name string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return v
		}
	}
	return ""
}

func resolved(program string, env []string) string {
	if path, err := lookIn(program, pathOf(env)); err == nil {
		return path
	}
	return program
}

// startWatch leaves a detached watcher behind, since this process is about to
// become the CLI. It runs on the host, so a sleeping Mac does not stop it.
func startWatch(block string, servers, env []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := watchCommand(self, block, servers, env)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// watchCommand gets the launch's resolved environment: the watcher's own PATH
// may lack rex for the same reason it may lack the CLI.
func watchCommand(self, block string, servers, env []string) *exec.Cmd {
	cmd := exec.Command(self, "answer-channels", "--block", block, "--servers", strings.Join(servers, ","))
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

func answerChannels(args []string, errOut io.Writer) int {
	fl := flag.NewFlagSet("answer-channels", flag.ContinueOnError)
	fl.SetOutput(errOut)
	block := fl.String("block", "", "Rex block")
	servers := fl.String("servers", "", "comma-separated channel servers")
	if fl.Parse(args) != nil || *block == "" || *servers == "" {
		return 2
	}
	path, _, err := find("rex", os.Environ())
	if err != nil {
		// This process's stderr goes nowhere, so the log is the only record.
		logLaunch(fmt.Sprintf("%s: channels dialog left for the operator: %v", *block, err))
		fmt.Fprintln(errOut, "burns:", err)
		return 1
	}
	line := launch.Watch(context.Background(), rex.CLI{Path: path}, *block, strings.Split(*servers, ","), launch.Timing())
	logLaunch(line)
	return 0
}

// logLaunch records every answer. A log failure must not touch the session.
func logLaunch(line string) {
	user, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := config.StateDir(user)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "launch.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, time.Now().Format(time.RFC3339), line)
}
