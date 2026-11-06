package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	rex "github.com/vizkid/rexkit/rex"
)

// resetFake stands in for Rex, smithersd and the Codex daemon. Typing /clear
// opens a new session; typing the prompt makes it work, unless idle.
type resetFake struct {
	agent, account string
	typed          []string
	open           []blockSession
	worked         map[string]time.Time
	idle, stuck    bool
	clock          time.Time
}

func (f *resetFake) host() resetHost {
	return resetHost{
		blocks: func(context.Context) ([]rex.Block, error) {
			return []rex.Block{{ID: "block:1", Label: "cap"}, {ID: "block:2", Label: "nav"}}, nil
		},
		process: func(_ context.Context, block string) (rex.Process, error) {
			if f.agent == "" {
				return rex.Process{Foreground: &rex.Proc{PID: 7, Name: "zsh", Path: "/bin/zsh", Cwd: "/w"}}, nil
			}
			return rex.Process{Foreground: &rex.Proc{PID: 7, Name: f.agent, Path: "/x/" + f.agent, InvokedPath: "/x/" + f.agent, Cwd: "/w"}}, nil
		},
		typeIn: func(_ context.Context, block, text string) error {
			f.typed = append(f.typed, block+" "+text)
			if text == "/clear" && !f.stuck {
				f.open = []blockSession{{Provider: "claude", Session: "s2", Account: "viz"}}
			}
			if strings.HasPrefix(text, "Context reset.") && !f.idle {
				f.worked["s2"] = f.clock.Add(time.Second)
			}
			return nil
		},
		sessions: func(context.Context, string) ([]blockSession, error) { return f.open, nil },
		last: func(_ context.Context, _, session string) (*time.Time, error) {
			if t, ok := f.worked[session]; ok {
				return &t, nil
			}
			return nil, nil
		},
		home: func(provider, account string) (string, error) {
			if account == "" {
				return "", errors.New("no account")
			}
			return "/h/.codex-" + account, nil
		},
		newCodex: func(_ context.Context, l codexLaunch, clear func() error) (string, string, error) {
			if l.Home != "/h/.codex-viz" || l.Dir != "/w" || l.Pid != 7 || l.Block != "block:2" {
				return "", "", errors.New("wrong launch")
			}
			if err := clear(); err != nil {
				return "", "", err
			}
			if f.stuck {
				return "", "no Codex thread started in /w", nil
			}
			return "s2", "codex thread s2 bound", nil
		},
		now:  func() time.Time { f.clock = f.clock.Add(100 * time.Millisecond); return f.clock },
		poll: time.Millisecond,
	}
}

func newFake(agent string) *resetFake {
	return &resetFake{agent: agent, worked: map[string]time.Time{}, clock: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC),
		open: []blockSession{{Provider: agent, Session: "s1", Account: "viz"}}}
}

func resetWith(f *resetFake, label string) (int, string) {
	codexDialogPause = 0
	var out, errOut bytes.Buffer
	p := resetPlan{Label: label, Checkpoint: "/w/ck.md", Task: "finish lint", Wait: 5 * time.Second}
	code := f.host().reset(context.Background(), p, &out, &errOut)
	return code, out.String() + errOut.String()
}

// Claude: /clear, wait for the new session, type the prompt, wait for work.
func TestResetClaudeClearsAndResumes(t *testing.T) {
	f := newFake("claude")
	code, out := resetWith(f, "cap")
	want := []string{"block:1 /clear",
		"block:1 Context reset. Load the reset-context skill and run its Resume steps on /w/ck.md. Task: finish lint"}
	if code != 0 || strings.Join(f.typed, "|") != strings.Join(want, "|") {
		t.Fatalf("code %d, typed %q, out %s", code, f.typed, out)
	}
	if !strings.Contains(out, "s2") {
		t.Errorf("out names no new session: %s", out)
	}
}

// Codex: the daemon watch wraps /new and names the new session.
func TestResetCodexUsesDaemonWatch(t *testing.T) {
	f := newFake("codex")
	f.open = []blockSession{{Provider: "codex", Session: "s1", Account: "viz"}}
	code, out := resetWith(f, "nav")
	// The bare Enter takes Codex's "Where should the new conversation run?"
	// default, the current folder. Without the dialog Codex ignores it.
	if code != 0 || len(f.typed) != 3 || f.typed[0] != "block:2 /new" || f.typed[1] != "block:2 " {
		t.Fatalf("code %d, typed %q, out %s", code, f.typed, out)
	}
}

// Refusals and timeouts name what went wrong, and nothing is typed on a refusal.
func TestResetRefusesAndTimesOut(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(*resetFake)
		label string
		code  int
		says  string
	}{
		{"no such pane", func(*resetFake) {}, "ghost", 1, "no pane labelled"},
		{"not an agent", func(f *resetFake) { f.agent = "" }, "cap", 1, "runs no agent"},
		{"codex without account", func(f *resetFake) { f.agent, f.open = "codex", nil }, "nav", 1, "--account"},
		{"not cleared", func(f *resetFake) { f.stuck = true }, "cap", 3, "not cleared"},
		{"no work", func(f *resetFake) { f.idle = true }, "cap", 3, "no request"},
	} {
		f := newFake("claude")
		c.setup(f)
		code, out := resetWith(f, c.label)
		if code != c.code || !strings.Contains(out, c.says) {
			t.Errorf("%s: code %d, out %q; want %d and %q", c.name, code, out, c.code, c.says)
		}
		if c.code == 1 && len(f.typed) > 0 {
			t.Errorf("%s: typed %q on a refusal", c.name, f.typed)
		}
	}
}

// The command checks its arguments before it touches anything.
func TestResetArguments(t *testing.T) {
	pane := map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}
	for _, args := range [][]string{
		{"reset"},
		{"reset", "cap", "--task", "t"},
		{"reset", "cap", "--checkpoint", "/c"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, strings.NewReader(""), &out, &errOut, env(pane)); code != 2 {
			t.Errorf("%v: code %d; want 2", args, code)
		}
	}
}
