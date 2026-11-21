package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// tabTimeout bounds each rex call the tab rename makes: the launch waits on it.
const tabTimeout = 2 * time.Second

// tabText is the label a launch gives its tab: `<icon> <folder>`
// (docs/SPEC.md, "Shell launchers", Tab).
func tabText(emoji, dir, userHome string) string {
	return clean(strings.TrimSpace(emoji + " " + folderName(dir, userHome)))
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

// nameTab renames the launch's tab when it has Rex's default label. It returns
// a line for launch.log when a rex call fails, and "" otherwise. Outside Rex it
// does nothing.
func nameTab(run func(args ...string) ([]byte, error), session, block, label string) string {
	if session == "" || block == "" || label == "" {
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
	if _, err := run("window", "rename", "-s", session, window, label); err != nil {
		return block + ": tab not named: rex window rename: " + err.Error()
	}
	return ""
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
