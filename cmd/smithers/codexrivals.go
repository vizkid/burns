package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vizkid/burns/internal/codexd"
	"github.com/vizkid/burns/internal/rex"
)

// rivalPane names another unbound Codex pane whose foreground runs in the launch
// folder, or returns "". Such a pane could have started the thread the watcher
// saw: the daemon does not say which client started it (review P1, 2026-11-04).
// A pane roar has already bound is no rival: roar refuses its thread as
// thread_bound_elsewhere.
func rivalPane(ctx context.Context, l codexLaunch, roar func([]string) (string, int), r rex.CLI) (string, error) {
	out, code := roar([]string{"panes", "--json"})
	if code != 0 {
		return "", fmt.Errorf("roar panes: exit %d", code)
	}
	blocks, err := unboundCodex([]byte(out), l.Block)
	if err != nil {
		return "", err
	}
	for _, b := range blocks {
		agent, cwd, err := r.ForegroundIn(ctx, b)
		if err != nil {
			return "", err
		}
		if agent == "codex" && codexd.SameDir(cwd, l.Dir) {
			return "unbound Codex pane " + b + " in " + l.Dir, nil
		}
	}
	return "", nil
}

// unboundCodex lists the Codex panes roar has not bound, other than own. An
// answer that is not roar's list is an error, never an empty list.
func unboundCodex(panes []byte, own string) ([]string, error) {
	var list []struct {
		Block string `json:"block"`
		Agent string `json:"agent"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal(panes, &list); err != nil {
		return nil, fmt.Errorf("roar panes: %w", err)
	}
	var out []string
	for _, p := range list {
		if p.Agent == "codex" && p.Path != "codex_native" && p.Block != own {
			out = append(out, p.Block)
		}
	}
	return out, nil
}
