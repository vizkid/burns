package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/theme"
)

// themeTimeout bounds the set_theme call: colour must never hold up a launch.
const themeTimeout = 5 * time.Second

// themeFile is this host's Rex themes file, or "" without the Rex app.
func themeFile(userHome string) string { return theme.Locate(theme.Dirs(userHome)) }

// applyTheme sets a launch's own block to its account's theme (docs/SPEC.md,
// "Pane themes"). It returns the line for launch.log, or "" when the account
// has no theme. Nothing here fails the launch.
func applyTheme(a config.Account, file string, call func(args ...string) error, session, block string) string {
	if a.Theme == "" {
		return ""
	}
	if file == "" {
		return block + ": theme " + a.Theme + " left to spawn: this host has no Rex app"
	}
	c, err := theme.Find(file, a.Theme)
	if err == nil {
		var args []byte
		if args, err = json.Marshal(c); err == nil {
			err = call("block", "call", "-s", session, "-b", block, "set_theme", string(args))
		}
	}
	if err != nil {
		return fmt.Sprintf("%s: theme %s not applied: %v", block, a.Theme, err)
	}
	return block + ": theme " + a.Theme + " applied"
}

// themeOwnPane runs applyTheme for `smithers launch` and logs the outcome.
func themeOwnPane(a config.Account, userHome, session, block string) {
	if block == "" {
		return
	}
	call := func(args ...string) error {
		path, env, err := find("rex", os.Environ())
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), themeTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Env, cmd.WaitDelay = env, pipeGrace
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w: %s", err, out)
		}
		return nil
	}
	if line := applyTheme(a, themeFile(userHome), call, session, block); line != "" {
		logLaunch(line)
	}
}

// accountLook is an account's emoji and resolved theme on this host, for
// spawn. A theme this host cannot resolve is a warning, never a refusal.
func accountLook(home, provider, name string, errOut io.Writer) (string, *theme.Colours) {
	cfg, err := config.Load(filepath.Join(config.Dir(home), "accounts.json"), home)
	if err != nil {
		return "", nil
	}
	a, _ := cfg.Find(provider, name)
	if a.Theme == "" {
		return a.Emoji, nil
	}
	file := themeFile(home)
	if file == "" {
		fmt.Fprintf(errOut, "warning: theme %s not applied: this host has no Rex app\n", a.Theme)
		return a.Emoji, nil
	}
	c, err := theme.Find(file, a.Theme)
	if err != nil {
		fmt.Fprintf(errOut, "warning: theme %s not applied: %v\n", a.Theme, err)
		return a.Emoji, nil
	}
	return a.Emoji, &c
}
