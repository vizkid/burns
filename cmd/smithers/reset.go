package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"time"

	rex "github.com/vizkid/rexkit/rex"
)

// codexDialogPause lets Codex open its "Where should the new conversation run?"
// dialog before the Enter that answers it.
var codexDialogPause = time.Second

// resetPlan is one `smithers reset` (docs/SPEC.md, "Resetting a pane").
type resetPlan struct {
	Label, Checkpoint, Task, Account string
	Wait                             time.Duration
}

// blockSession is one open binding of a block, as smithersd answers it.
type blockSession struct {
	Provider string `json:"provider"`
	Session  string `json:"session"`
	Account  string `json:"account"`
}

// resetHost holds what a reset asks of Rex, smithersd and the Codex daemon,
// so tests can stand in for each part.
type resetHost struct {
	blocks   func(ctx context.Context) ([]rex.Block, error) // the caller's session
	process  func(ctx context.Context, block string) (rex.Process, error)
	typeIn   func(ctx context.Context, block, text string) error // text, then Enter; "" is Enter alone
	sessions func(ctx context.Context, block string) ([]blockSession, error)
	last     func(ctx context.Context, provider, session string) (*time.Time, error)
	home     func(provider, account string) (string, error)
	// newCodex listens on the daemon, runs clear, and returns the session of the
	// one thread started in the pane's folder, or why there is none.
	newCodex func(ctx context.Context, l codexLaunch, clear func() error) (session, said string, err error)
	now      func() time.Time
	poll     time.Duration
}

func resetPane(args []string, out, errOut io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var p resetPlan
	fs.StringVar(&p.Checkpoint, "checkpoint", "", "the checkpoint the pane resumes from")
	fs.StringVar(&p.Task, "task", "", "the task the pane carries on with")
	fs.StringVar(&p.Account, "account", "", "the pane's account, when smithersd does not know it")
	fs.DurationVar(&p.Wait, "wait", 120*time.Second, "how long to wait for each step")
	label, rest := "", args
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		label, rest = args[0], args[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if label == "" && fs.NArg() > 0 {
		label = fs.Arg(0)
	}
	if p.Label = label; label == "" || p.Checkpoint == "" || p.Task == "" {
		fmt.Fprintln(errOut, "usage: smithers reset PANE --checkpoint PATH --task TEXT [--account NAME] [--wait 120s]")
		return 2
	}
	session := getenv("REX_SESSION")
	if session == "" || getenv("REX_BLOCK") == "" {
		fmt.Fprintln(errOut, "smithers reset runs inside a Rex pane: REX_SESSION and REX_BLOCK are not set")
		return 1
	}
	h, err := liveResetHost(session)
	if err != nil {
		fmt.Fprintln(errOut, "reset:", err)
		return 1
	}
	return h.reset(context.Background(), p, out, errOut)
}

// reset clears the pane, types the resume prompt, and waits for the agent to work.
func (h resetHost) reset(ctx context.Context, p resetPlan, out, errOut io.Writer) int {
	refuse := func(format string, a ...any) int { fmt.Fprintf(errOut, "reset: "+format+"\n", a...); return 1 }
	block, err := h.find(ctx, p.Label)
	if err != nil {
		return refuse("%v", err)
	}
	proc, err := h.process(ctx, block)
	if err != nil {
		return refuse("rex: %v", err)
	}
	if proc.Foreground == nil || (proc.Foreground.Agent() != "claude" && proc.Foreground.Agent() != "codex") {
		return refuse("pane %s runs no agent, so it would take /clear as text", p.Label)
	}
	agent, fg := proc.Foreground.Agent(), proc.Foreground
	before, err := h.sessions(ctx, block)
	if err != nil {
		return refuse("smithersd: %v", err)
	}
	var session string
	if agent == "claude" {
		if err := h.typeIn(ctx, block, "/clear"); err != nil {
			return refuse("rex: %v", err)
		}
		if session = h.newSession(ctx, block, before, p.Wait); session == "" {
			fmt.Fprintf(errOut, "reset: %s not cleared: no new Claude session within %s\n", p.Label, p.Wait)
			return 3
		}
	} else {
		// An explicit --account wins over the one smithersd knows.
		account := p.Account
		for _, b := range before {
			account = cmp.Or(account, b.Account)
		}
		home, err := h.home("codex", account)
		if account == "" || err != nil {
			return refuse("cannot tell pane %s's Codex account; pass --account", p.Label)
		}
		l := codexLaunch{Home: home, Block: block, Session: "", Dir: fg.Cwd, Pid: fg.PID, Source: "reset"}
		// After /new in a git folder, Codex 0.166 asks where the new conversation
		// runs; Enter takes the default, the current folder. Codex ignores an Enter
		// on an empty input, so the second Enter is safe without the dialog.
		clear := func() error {
			if err := h.typeIn(ctx, block, "/new"); err != nil {
				return err
			}
			time.Sleep(codexDialogPause)
			return h.typeIn(ctx, block, "")
		}
		s, said, err := h.newCodex(ctx, l, clear)
		if err != nil {
			return refuse("%v", err)
		}
		if session = s; session == "" {
			fmt.Fprintf(errOut, "reset: %s not cleared: %s\n", p.Label, said)
			return 3
		}
		fmt.Fprintln(out, said)
	}
	prompted := h.now()
	prompt := fmt.Sprintf("Context reset. Load the reset-context skill and run its Resume steps on %s. Task: %s", p.Checkpoint, p.Task)
	if err := h.typeIn(ctx, block, prompt); err != nil {
		return refuse("rex: %v", err)
	}
	if !h.worked(ctx, agent, session, prompted, p.Wait) {
		fmt.Fprintf(errOut, "reset: %s cleared to session %s, but no request came within %s of the prompt\n", p.Label, session, p.Wait)
		return 3
	}
	fmt.Fprintf(out, "reset %s: session %s is working on the resume prompt\n", p.Label, session)
	return 0
}

// find returns the one block labelled label in the caller's session.
func (h resetHost) find(ctx context.Context, label string) (string, error) {
	blocks, err := h.blocks(ctx)
	if err != nil {
		return "", err
	}
	var found []string
	for _, b := range blocks {
		if b.Label == label {
			found = append(found, b.ID)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("your session has no pane labelled %q", label)
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("%d panes in your session are labelled %q; reset will not guess", len(found), label)
}

// newSession waits for an open binding of the block that was not there before.
func (h resetHost) newSession(ctx context.Context, block string, before []blockSession, wait time.Duration) string {
	had := func(s blockSession) bool {
		return slices.ContainsFunc(before, func(b blockSession) bool { return b.Session == s.Session })
	}
	for end := h.now().Add(wait); h.now().Before(end); time.Sleep(h.poll) {
		open, _ := h.sessions(ctx, block)
		for _, s := range open {
			if !had(s) {
				return s.Session
			}
		}
	}
	return ""
}

// worked waits for a request of the session after the prompt.
func (h resetHost) worked(ctx context.Context, provider, session string, after time.Time, wait time.Duration) bool {
	for end := h.now().Add(wait); h.now().Before(end); time.Sleep(h.poll) {
		if t, _ := h.last(ctx, provider, session); t != nil && !t.Before(after) {
			return true
		}
	}
	return false
}
