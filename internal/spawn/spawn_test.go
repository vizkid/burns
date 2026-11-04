package spawn

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vizkid/rexkit/rex"
)

// fakeRex answers like one Rex server and records every command it ran.
type fakeRex struct {
	host     string
	sessions []rex.Session
	blocks   map[string][]rex.Block // by session ID
	out      map[string]string      // canned JSON by the command's first word or two
	agent    string                 // what the new block's foreground becomes
	after    int                    // process reads before the agent shows
	reads    int
	ran      [][]string
	fail     string // a command that fails, by its first word
}

func (f *fakeRex) Whoami(context.Context) (rex.Whoami, error) { return rex.Whoami{HostID: f.host}, nil }
func (f *fakeRex) Sessions(context.Context) ([]rex.Session, error) {
	return f.sessions, nil
}
func (f *fakeRex) Blocks(_ context.Context, session string) ([]rex.Block, error) {
	return f.blocks[session], nil
}
func (f *fakeRex) Run(_ context.Context, args ...string) ([]byte, error) {
	f.ran = append(f.ran, args)
	if args[0] == f.fail {
		return nil, errors.New("rex " + args[0] + ": killed")
	}
	for _, key := range []string{strings.Join(args[:min(2, len(args))], " "), args[0]} {
		if out, ok := f.out[key]; ok {
			return []byte(out), nil
		}
	}
	return nil, nil
}
func (f *fakeRex) Process(context.Context, string, string) (rex.Process, error) {
	f.reads++
	if f.agent == "" || f.reads <= f.after {
		return rex.Process{Foreground: &rex.Proc{Name: "smithers", Argv0: "smithers"}}, nil
	}
	return rex.Process{Foreground: &rex.Proc{Name: f.agent, InvokedPath: "/usr/local/bin/" + f.agent, Argv0: f.agent}}, nil
}

const (
	newWindow = `{"window_id":"window:w2","block_ids":["block:new"],"revision":2}`
	newSess   = `{"session_id":"session:made","initial_windows":[{"window_id":"window:w1","block_ids":["block:new"]}]}`
	split     = `{"anchor_id":"block:caller","block_ids":["block:new"],"split_ids":["split:s"]}`
)

func local() *fakeRex {
	return &fakeRex{host: "h1", sessions: []rex.Session{{ID: "session:mine", Label: "proj"}},
		blocks: map[string][]rex.Block{"session:mine": {{ID: "block:caller", Label: "cap"}}},
		out:    map[string]string{"window new": newWindow, "new": newSess, "split": split}, agent: "claude", after: 2}
}

func request() Request {
	return Request{Provider: "claude", Account: "alpha", Label: "helper", Tab: "helper-cap", Emoji: "🟢", Dir: "dev/proj",
		CallerSession: "session:mine", CallerBlock: "block:caller", Wait: time.Second, Every: time.Millisecond}
}

var launchCmd = []string{"--", "smithers", "launch", "--provider", "claude", "--account", "alpha", "--dir", "dev/proj"}

// A new tab in the caller's own session, named for roar, then a wait until the
// agent is the pane's foreground process.
func TestSpawnOpensATabInTheCallersSession(t *testing.T) {
	l := local()
	res, err := Spawn(t.Context(), l, l, request())
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{"window", "new", "-s", "session:mine", "--focus=false", "--json", "🟢 helper-cap"}, launchCmd...)
	if !slices.Equal(l.ran[0], want) {
		t.Errorf("ran %q\nwant %q", l.ran[0], want)
	}
	if !slices.Equal(l.ran[1], []string{"block", "rename", "-s", "session:mine", "block:new", "helper"}) {
		t.Errorf("rename = %q", l.ran[1])
	}
	if res != (Result{SessionID: "session:mine", SessionLabel: "proj", Block: "block:new", Label: "helper", Agent: "claude"}) {
		t.Errorf("result = %+v", res)
	}
}

// Viz, 2026-11-03: a host without the caller's session gets one, labelled the same.
func TestSpawnCreatesTheSessionOnAHostWithoutIt(t *testing.T) {
	l, other := local(), local()
	other.host, other.sessions, other.blocks = "h2", nil, nil
	res, err := Spawn(t.Context(), l, other, request())
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{"new", "proj", "--window", "🟢 helper-cap", "--json"}, launchCmd...)
	if !slices.Equal(other.ran[0], want) {
		t.Errorf("ran %q\nwant %q", other.ran[0], want)
	}
	if !res.Created || res.SessionID != "session:made" || res.Block != "block:new" {
		t.Errorf("result = %+v", res)
	}
	if len(l.ran) != 0 {
		t.Errorf("the caller's host ran %q", l.ran)
	}
}

// Two sessions with one label: picking one would be a guess.
func TestSpawnRefusesTwoSessionsWithTheCallersLabel(t *testing.T) {
	l, other := local(), local()
	other.host = "h2"
	other.sessions = []rex.Session{{ID: "session:a", Label: "proj"}, {ID: "session:b", Label: "proj"}}
	_, err := Spawn(t.Context(), l, other, request())
	if err == nil || !strings.Contains(err.Error(), "session:a") || !strings.Contains(err.Error(), "session:b") || len(other.ran) != 0 {
		t.Fatalf("err %v, ran %q", err, other.ran)
	}
}

// --beside splits the caller's own tab, with the label set at creation.
func TestSpawnBesideSplitsTheCallersTab(t *testing.T) {
	l := local()
	req := request()
	req.Beside = true
	if _, err := Spawn(t.Context(), l, l, req); err != nil {
		t.Fatal(err)
	}
	want := append([]string{"split", "-s", "session:mine", "-b", "block:caller", "--label", "helper", "--focus=false", "--json"}, launchCmd...)
	if len(l.ran) != 1 || !slices.Equal(l.ran[0], want) {
		t.Errorf("ran %q\nwant %q", l.ran, want)
	}
}

// The caller's block exists only on its own host.
func TestSpawnBesideRefusesAnotherHost(t *testing.T) {
	l, other := local(), local()
	other.host = "h2"
	req := request()
	req.Beside = true
	if _, err := Spawn(t.Context(), l, other, req); err == nil || len(other.ran) != 0 {
		t.Fatalf("err %v, ran %q", err, other.ran)
	}
}

// Roar finds a pane by label, so the label must name one pane.
func TestSpawnRefusesALabelInUse(t *testing.T) {
	l := local()
	req := request()
	req.Label = "cap"
	if _, err := Spawn(t.Context(), l, l, req); err == nil || len(l.ran) != 0 {
		t.Fatalf("err %v, ran %q", err, l.ran)
	}
}

// The wait runs out: the pane exists, so the result names it, and the error says so.
func TestSpawnReportsAnAgentThatNeverStarted(t *testing.T) {
	l := local()
	l.agent = ""
	req := request()
	req.Wait = 20 * time.Millisecond
	res, err := Spawn(t.Context(), l, l, req)
	if !errors.Is(err, ErrNotStarted) || res.Block != "block:new" || res.Agent != "" {
		t.Fatalf("res %+v, err %v", res, err)
	}
}

// Values become arguments, never shell code; the model is passed through.
func TestSpawnPassesTheModelAsOneArgument(t *testing.T) {
	l := local()
	req := request()
	req.Model = "claude-haiku-5-5; rm -rf ~"
	if _, err := Spawn(t.Context(), l, l, req); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(l.ran[0], "claude-haiku-5-5; rm -rf ~") {
		t.Errorf("ran %q", l.ran[0])
	}
}

// Outside a Rex pane there is no caller session to spawn into.
func TestSpawnNeedsTheCallersSession(t *testing.T) {
	l := local()
	req := request()
	req.CallerSession = "session:gone"
	if _, err := Spawn(t.Context(), l, l, req); err == nil {
		t.Fatal("spawned without a caller session")
	}
}

// Claude's folder-trust dialog waits for the operator. Claude is already the
// foreground process, so only the screen shows it (Viz, 2026-11-03: spawn may
// read the new pane's screen to verify this).
func TestSpawnReportsTheFolderTrustDialog(t *testing.T) {
	l := local()
	l.out["capture -s"] = " ~/dev/proj\n Quick safety check: Is this a project you created or one you trust?\n\n ❯ No, exit\n   Yes, I trust this folder\n\n Enter to confirm · Esc to cancel\n"
	req := request()
	req.TrustWatch = 20 * time.Millisecond
	res, err := Spawn(t.Context(), l, l, req)
	if !errors.Is(err, ErrTrust) || res.Agent != "claude" || res.Block != "block:new" {
		t.Fatalf("res %+v, err %v", res, err)
	}
}

// A screen without the dialog is a running agent, after the watch.
func TestSpawnWatchesTheScreenOnlyBriefly(t *testing.T) {
	l := local()
	l.out["capture -s"] = " Claude Code\n ❯ \n"
	req := request()
	req.TrustWatch = 20 * time.Millisecond
	if _, err := Spawn(t.Context(), l, l, req); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(l.ran, func(a []string) bool { return a[0] == "capture" }) {
		t.Error("the screen was never read")
	}
}

// Captures that fail throughout leave the dialog unchecked: spawn says so
// rather than reporting a clean start (peer review, 2026-11-03).
func TestSpawnSaysWhenTheTrustCheckCouldNotRun(t *testing.T) {
	l := local()
	l.fail = "capture"
	req := request()
	req.TrustWatch = 20 * time.Millisecond
	res, err := Spawn(t.Context(), l, l, req)
	if !errors.Is(err, ErrTrustUnchecked) || res.Agent != "claude" || res.Block != "block:new" {
		t.Fatalf("res %+v, err %v", res, err)
	}
}

// A new tab is named like the lanes' own: the account's emoji, then the tab
// label (Viz, 2026-11-03). With no emoji, the label alone.
func TestSpawnNamesTheTabWithoutAnEmoji(t *testing.T) {
	l := local()
	req := request()
	req.Emoji = ""
	if _, err := Spawn(t.Context(), l, l, req); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(l.ran[0], "helper-cap") || slices.Contains(l.ran[0], " helper-cap") {
		t.Errorf("ran %q", l.ran[0])
	}
}

// A new tab needs its label; --beside opens none.
func TestSpawnNeedsATabLabelForANewTab(t *testing.T) {
	l := local()
	req := request()
	req.Tab = ""
	if _, err := Spawn(t.Context(), l, l, req); err == nil || len(l.ran) != 0 {
		t.Fatalf("err %v, ran %q", err, l.ran)
	}
	req.Beside = true
	if _, err := Spawn(t.Context(), l, l, req); err != nil {
		t.Fatalf("beside: %v", err)
	}
}
