package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vizkid/burns/internal/config"
)

// tabTimeout bounds each rex call the tab rename makes: the launch waits on it.
const tabTimeout = 2 * time.Second

// tabText is the label a launch gives its tab: `<icon> <name>`
// (docs/SPEC.md, "Shell launchers", Tab).
func tabText(emoji, name string) string {
	return clean(strings.TrimSpace(emoji + " " + strings.TrimSpace(name)))
}

// tabToRename returns the window holding block when that window still has
// Rex's default label, "Window " and the last 8 hex digits of its id. Any other
// label was chosen by spawn, the Mac panel or the operator, so it stays.
func tabToRename(blocks, windows []byte, block string) string {
	var b struct {
		Blocks []struct {
			Block  string `json:"block_id"`
			Window string `json:"window_id"`
		} `json:"blocks"`
	}
	var w struct {
		Windows []struct {
			Window string `json:"window_id"`
			Label  string `json:"label"`
		} `json:"windows"`
	}
	if json.Unmarshal(blocks, &b) != nil || json.Unmarshal(windows, &w) != nil {
		return ""
	}
	window := ""
	for _, x := range b.Blocks {
		if x.Block == block {
			window = x.Window
		}
	}
	for _, x := range w.Windows {
		if window != "" && x.Window == window && x.Label == "Window "+lastHex(window, 8) {
			return window
		}
	}
	return ""
}

// lastHex is the last n hex digits of a Rex id, hyphens dropped.
func lastHex(id string, n int) string {
	s := strings.ReplaceAll(id[strings.LastIndex(id, ":")+1:], "-", "")
	if len(s) < n {
		return s
	}
	return s[len(s)-n:]
}

// nameTab names the launch's tab `<icon> <name>` when the tab has Rex's
// default label. The name is flagTab, or else the answer to ask, where an empty
// answer takes folder. A nil ask, as with no terminal, leaves the tab alone. It
// returns a line for launch.log when a rex call fails, and "" otherwise. Outside
// Rex it does nothing.
func nameTab(run func(args ...string) ([]byte, error), session, block, emoji, flagTab, folder string,
	ask func(folder string) (string, bool)) string {
	if session == "" || block == "" {
		return ""
	}
	blocks, err := run("block", "ls", "-s", session, "--json", "--short=false")
	if err != nil {
		return block + ": tab not named: rex block ls: " + err.Error()
	}
	windows, err := run("window", "ls", "-s", session, "--json", "--short=false")
	if err != nil {
		return block + ": tab not named: rex window ls: " + err.Error()
	}
	window := tabToRename(blocks, windows, block)
	if window == "" {
		return ""
	}
	name := strings.TrimSpace(flagTab)
	if name == "" {
		if ask == nil {
			return ""
		}
		answer, ok := ask(folder)
		if !ok {
			return ""
		}
		if name = strings.TrimSpace(answer); name == "" {
			name = folder
		}
	}
	if _, err := run("window", "rename", "-s", session, window, tabText(emoji, name)); err != nil {
		return block + ": tab not named: rex window rename: " + err.Error()
	}
	return ""
}

// askTab prompts once for the tab's name. It reads one byte at a time up to the
// newline, so input typed ahead for the agent stays unread. End of input
// reports false.
func askTab(in io.Reader, out io.Writer) func(folder string) (string, bool) {
	return func(folder string) (string, bool) {
		fmt.Fprintf(out, "Tab name [%s]: ", folder)
		var line []byte
		b := make([]byte, 1)
		for {
			n, err := in.Read(b)
			if n == 1 {
				if b[0] == '\n' {
					return strings.TrimSuffix(string(line), "\r"), true
				}
				line = append(line, b[0])
			}
			if err != nil {
				return string(line), len(line) > 0
			}
		}
	}
}

// labelTab names the launch's tab (docs/SPEC.md, "Shell launchers", Tab). It
// prompts only when in is a terminal.
func labelTab(a config.Account, cwd, user, flagTab string, in io.Reader, out io.Writer, getenv func(string) string) {
	run := rexRunner()
	if run == nil {
		return
	}
	var ask func(string) (string, bool)
	if f, ok := in.(*os.File); ok {
		if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			ask = askTab(in, out)
		}
	}
	if line := nameTab(run, getenv("REX_SESSION"), getenv("REX_BLOCK"), a.Emoji, flagTab, folderName(cwd, user), ask); line != "" {
		logLaunch(line)
	}
}

// rexRunner runs rex with tabTimeout per call. It is nil when rex is not found.
func rexRunner() func(args ...string) ([]byte, error) {
	path, err := exec.LookPath("rex")
	if err != nil {
		return nil
	}
	return func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), tabTimeout)
		defer cancel()
		return exec.CommandContext(ctx, path, args...).Output()
	}
}
