package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/launch"
)

func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func pathVars(env []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return !strings.HasPrefix(kv, "PATH=") })
}

// A process reads the first PATH and exec does not dedupe, so a second entry
// would leave the CLI on the short PATH.
func TestFindReplacesThePathTheShellCorrected(t *testing.T) {
	dir := t.TempDir()
	full := filepath.Join(dir, "full")
	script(t, full, "tool", "")
	shell := script(t, dir, "shell", `printf '\n%s' "`+full+`"`)
	path, env, err := find("tool", []string{"PATH=" + filepath.Join(dir, "short"), "SHELL=" + shell})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"PATH=" + full}; path != filepath.Join(full, "tool") || !slices.Equal(pathVars(env), want) {
		t.Fatalf("path %s, PATH %v", path, pathVars(env))
	}
}

// A background job in an rc file keeps the output pipe open after the shell exits.
func TestFindDoesNotWaitForTheShellsBackgroundJobs(t *testing.T) {
	dir := t.TempDir()
	script(t, dir, "tool", "")
	shell := script(t, dir, "shell", `/bin/sleep 8 & printf '\n%s' "`+dir+`"`)
	start := time.Now()
	if _, _, err := find("tool", []string{"PATH=/none", "SHELL=" + shell}); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("took %s", took)
	}
}

func TestADaemonHoldingThePipeDoesNotStopTheLaunch(t *testing.T) {
	dir := t.TempDir()
	script(t, dir, "daemon", "/bin/sleep 8 &")
	script(t, dir, "tool", "")
	t.Setenv("PATH", dir)
	ran := false
	replace = func(string, []string, []string) error { ran = true; return nil }
	t.Cleanup(func() { replace = nil })
	var errOut bytes.Buffer
	start := time.Now()
	plan := launch.Plan{Provider: "tool", Program: "tool", Env: map[string]string{}, Daemon: []string{"daemon"}}
	if err := startPlan(plan, &errOut, "", ""); err != nil || !ran {
		t.Fatalf("err %v, ran %v", err, ran)
	}
	if took := time.Since(start); took > 5*time.Second || errOut.Len() != 0 {
		t.Fatalf("took %s, stderr %q", took, errOut.String())
	}
}

func TestLaunchClearsAnInheritedHomeOnTheDefaultHome(t *testing.T) {
	user := fakeHost(t)
	os.WriteFile(filepath.Join(user, ".config", "burns", "accounts.json"),
		[]byte(`{"accounts":[{"provider":"claude","name":"alpha","home":".claude","kind":"subscription"}]}`), 0o600)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(user, ".claude-other"))
	var environ []string
	replace = func(_ string, _, e []string) error { environ = e; return nil }
	t.Cleanup(func() { replace = nil })
	var errOut bytes.Buffer
	if code := launchAgent([]string{"--provider", "claude", "--account", "alpha"}, strings.NewReader(""), &errOut, os.Getenv); code != 0 {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
	for _, kv := range environ {
		if strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") {
			t.Fatalf("env keeps %s", kv)
		}
	}
}

func TestLaunchReadsTheXDGConfigFolder(t *testing.T) {
	user := fakeHost(t)
	xdg := filepath.Join(user, "xdg")
	if err := os.Rename(filepath.Join(user, ".config"), xdg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	replace = func(string, []string, []string) error { return nil }
	t.Cleanup(func() { replace = nil })
	var errOut bytes.Buffer
	if code := launchAgent([]string{"--provider", "claude", "--account", "alpha"}, strings.NewReader("\n"), &errOut, os.Getenv); code != 0 {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
}

func TestLaunchCreatesANestedHome(t *testing.T) {
	user := fakeHost(t)
	os.WriteFile(filepath.Join(user, ".config", "burns", "accounts.json"),
		[]byte(`{"accounts":[{"provider":"claude","name":"alpha","home":"agents/claude-alpha","kind":"subscription"}]}`), 0o600)
	replace = func(string, []string, []string) error { return nil }
	t.Cleanup(func() { replace = nil })
	var errOut bytes.Buffer
	if code := launchAgent([]string{"--provider", "claude", "--account", "alpha"}, strings.NewReader("\n"), &errOut, os.Getenv); code != 0 {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
	for _, dir := range []string{"agents", "agents/claude-alpha"} {
		if info, err := os.Stat(filepath.Join(user, dir)); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", dir, info, err)
		}
	}
}

func TestTheWatcherGetsTheResolvedEnvironment(t *testing.T) {
	env := []string{"PATH=/full"}
	cmd := watchCommand("/self", "block:1", []string{"a", "b"}, env)
	if !slices.Equal(cmd.Env, env) || !slices.Contains(cmd.Args, "a,b") {
		t.Fatalf("%+v", cmd)
	}
}

// The watcher's stderr goes nowhere, so a failure must reach launch.log.
func TestAMissingRexIsLogged(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("PATH", user)
	t.Setenv("SHELL", script(t, user, "shell", "exit 1"))
	if code := answerChannels([]string{"--block", "block:1", "--servers", "a"}, io.Discard); code != 1 {
		t.Fatalf("code %d", code)
	}
	log, err := os.ReadFile(filepath.Join(user, ".local", "state", "burns", "launch.log"))
	if err != nil || !strings.Contains(string(log), "block:1") || !strings.Contains(string(log), "rex") {
		t.Fatalf("log %q, %v", log, err)
	}
}
