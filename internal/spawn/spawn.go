// Package spawn starts an agent session for another agent (docs/SPEC.md,
// "Agent-started launches"). The new pane joins the caller's own Rex session,
// on the caller's host or another, and runs `smithers launch` there.
package spawn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vizkid/rexkit/rex"
)

// ErrNotStarted means the pane exists but its agent did not start within the wait.
var ErrNotStarted = errors.New("the agent did not start")

// Rex is one Rex server. rexkit's rex.Client satisfies it.
type Rex interface {
	Whoami(ctx context.Context) (rex.Whoami, error)
	Sessions(ctx context.Context) ([]rex.Session, error)
	Blocks(ctx context.Context, session string) ([]rex.Block, error)
	Run(ctx context.Context, args ...string) ([]byte, error)
	Process(ctx context.Context, session, block string) (rex.Process, error)
}

// Request is one spawn. Every value becomes an argument, never shell code.
type Request struct {
	Provider, Account, Model string
	Label                    string // the new pane's label, which roar addresses
	Emoji                    string // the new tab's label: the account's emoji
	Dir                      string // passed to `smithers launch --dir`
	Beside                   bool   // split the caller's tab instead of opening one
	CallerSession            string
	CallerBlock              string
	Wait, Every              time.Duration
}

// Result names the new pane. Agent is the provider once it runs, else empty.
type Result struct {
	SessionID, SessionLabel string
	Block, Label            string
	Agent                   string
	Created                 bool // the session was created on the target host
}

// Spawn opens the pane on target, names it, and waits for the agent. local is
// the caller's server; it may be the same as target.
func Spawn(ctx context.Context, local, target Rex, req Request) (Result, error) {
	label, err := sessionLabel(ctx, local, req.CallerSession)
	if err != nil {
		return Result{}, err
	}
	res := Result{SessionLabel: label, Label: req.Label}
	if req.Beside {
		if err := sameServer(ctx, local, target); err != nil {
			return Result{}, err
		}
		if err := labelFree(ctx, target, req.CallerSession, req.Label); err != nil {
			return Result{}, err
		}
		res.SessionID = req.CallerSession
		res.Block, err = created(target.Run(ctx, append([]string{"split", "-s", req.CallerSession, "-b", req.CallerBlock,
			"--label", req.Label, "--focus=false", "--json"}, command(req)...)...))
	} else {
		res, err = openTab(ctx, target, req, res)
	}
	if err != nil {
		return res, err
	}
	res.Agent, err = wait(ctx, target, res.SessionID, res.Block, req)
	return res, err
}

// openTab adds a tab to the target's session with the caller's label, or
// creates that session, then labels the new block.
func openTab(ctx context.Context, target Rex, req Request, res Result) (Result, error) {
	sessions, err := target.Sessions(ctx)
	if err != nil {
		return res, err
	}
	var ids []string
	for _, s := range sessions {
		if s.Label == res.SessionLabel {
			ids = append(ids, s.ID)
		}
	}
	switch len(ids) {
	case 0:
		args := []string{"new", res.SessionLabel}
		if req.Emoji != "" {
			args = append(args, "--window", req.Emoji)
		}
		out, err := target.Run(ctx, append(append(args, "--json"), command(req)...)...)
		if err != nil {
			return res, err
		}
		var s struct {
			ID      string `json:"session_id"`
			Windows []struct {
				Blocks []string `json:"block_ids"`
			} `json:"initial_windows"`
		}
		if err := json.Unmarshal(out, &s); err != nil || s.ID == "" || len(s.Windows) == 0 || len(s.Windows[0].Blocks) == 0 {
			return res, fmt.Errorf("rex new printed %q", out)
		}
		res.SessionID, res.Block, res.Created = s.ID, s.Windows[0].Blocks[0], true
	case 1:
		res.SessionID = ids[0]
		if err := labelFree(ctx, target, res.SessionID, req.Label); err != nil {
			return res, err
		}
		args := []string{"window", "new", "-s", res.SessionID, "--focus=false", "--json"}
		if req.Emoji != "" {
			args = append(args, req.Emoji)
		}
		if res.Block, err = created(target.Run(ctx, append(args, command(req)...)...)); err != nil {
			return res, err
		}
	default:
		return res, fmt.Errorf("%d sessions are labelled %q on that host (%s); spawn will not guess", len(ids), res.SessionLabel, strings.Join(ids, ", "))
	}
	_, err = target.Run(ctx, "block", "rename", "-s", res.SessionID, res.Block, req.Label)
	return res, err
}

// command is what the new pane runs.
func command(req Request) []string {
	args := []string{"--", "smithers", "launch", "--provider", req.Provider, "--account", req.Account}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	return append(args, "--dir", req.Dir)
}

// created reads the new block from `rex window new` or `rex split`.
func created(out []byte, err error) (string, error) {
	if err != nil {
		return "", err
	}
	var w struct {
		Blocks []string `json:"block_ids"`
	}
	if err := json.Unmarshal(out, &w); err != nil || len(w.Blocks) == 0 {
		return "", fmt.Errorf("rex printed no new block: %q", out)
	}
	return w.Blocks[0], nil
}

func sessionLabel(ctx context.Context, r Rex, id string) (string, error) {
	sessions, err := r.Sessions(ctx)
	if err != nil {
		return "", err
	}
	for _, s := range sessions {
		if s.ID == id && s.Label != "" {
			return s.Label, nil
		}
	}
	return "", fmt.Errorf("the caller's session %s has no label on this host", id)
}

func sameServer(ctx context.Context, local, target Rex) error {
	a, err := local.Whoami(ctx)
	if err != nil {
		return err
	}
	b, err := target.Whoami(ctx)
	if err != nil {
		return err
	}
	if a.HostID != b.HostID {
		return errors.New("--beside splits the caller's own tab, which exists only on the caller's host")
	}
	return nil
}

func labelFree(ctx context.Context, r Rex, session, label string) error {
	blocks, err := r.Blocks(ctx, session)
	if err != nil {
		return err
	}
	for _, b := range blocks {
		if b.Label == label {
			return fmt.Errorf("a pane in that session is already labelled %q, and roar would not know which one you mean", label)
		}
	}
	return nil
}

// wait polls the new block until its foreground process is the agent.
func wait(ctx context.Context, r Rex, session, block string, req Request) (string, error) {
	deadline := time.Now().Add(req.Wait)
	for {
		p, err := r.Process(ctx, session, block)
		if err != nil {
			return "", fmt.Errorf("reading the new pane: %w", err)
		}
		if p.Foreground != nil && p.Foreground.Agent() == req.Provider {
			return req.Provider, nil
		}
		if time.Now().After(deadline) {
			return "", ErrNotStarted
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(req.Every):
		}
	}
}
