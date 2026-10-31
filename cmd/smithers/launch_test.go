package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func fakeHost(t *testing.T) string {
	t.Helper()
	user := t.TempDir()
	t.Setenv("HOME", user)
	bin := filepath.Join(user, "bin")
	os.MkdirAll(bin, 0o700)
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"), 0o700)
	t.Setenv("PATH", bin)
	dir := filepath.Join(user, ".config", "burns", "tools.d")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(filepath.Dir(dir), "accounts.json"),
		[]byte(`{"accounts":[{"provider":"claude","name":"alpha","home":".claude-alpha","kind":"subscription","launch":{"effort":"low"}}]}`), 0o600)
	return user
}

func TestLaunchCreatesTheHomeAndBecomesTheCLI(t *testing.T) {
	user := fakeHost(t)
	var path string
	var argv, environ []string
	replace = func(p string, a, e []string) error { path, argv, environ = p, a, e; return nil }
	t.Cleanup(func() { replace = nil })
	var errOut bytes.Buffer
	code := launchAgent([]string{"--provider", "claude", "--account", "alpha", "--effort", "high"}, strings.NewReader(""), &errOut, os.Getenv)
	if code != 0 {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
	home := filepath.Join(user, ".claude-alpha")
	if info, err := os.Stat(home); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("home: %v %v", info, err)
	}
	if filepath.Base(path) != "claude" || !slices.Equal(argv, []string{"claude", "--effort", "high"}) {
		t.Fatalf("path %s argv %v", path, argv)
	}
	if !slices.Contains(environ, "CLAUDE_CONFIG_DIR="+home) || !slices.Contains(environ, "BURNS_ACCOUNT=alpha") {
		t.Fatalf("env %v", environ)
	}
}

func TestLaunchRefusesWhatItCannotPass(t *testing.T) {
	fakeHost(t)
	replace = func(string, []string, []string) error { return errors.New("must not run") }
	t.Cleanup(func() { replace = nil })
	for _, args := range [][]string{
		{"--provider", "claude", "--account", "nobody"},
		{"--provider", "claude", "--account", "alpha", "--model", "--evil"},
	} {
		var errOut bytes.Buffer
		if code := launchAgent(args, strings.NewReader("\n"), &errOut, os.Getenv); code != 1 || strings.Contains(errOut.String(), "must not run") {
			t.Fatalf("%v: code %d: %s", args, code, errOut.String())
		}
	}
}
