package launch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/config"
)

const user = "/home/op"

func has(args []string, want ...string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		if reflect.DeepEqual(args[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

func TestClaudeUsesOverridesToolsAndTelemetry(t *testing.T) {
	a := config.Account{Provider: "claude", Name: "alpha", Launch: &config.Launch{Model: "m1", Effort: "high", PermissionMode: "auto"}}
	p := Build(Input{Account: a, Home: user + "/.claude-alpha", UserHome: user, Channels: []string{"b", "a", "b"}, Block: "block:1", Session: "session:1"})
	if p.Program != "claude" || p.Env["CLAUDE_CONFIG_DIR"] != user+"/.claude-alpha" || p.Env["BURNS_ACCOUNT"] != "alpha" {
		t.Fatalf("plan = %+v", p)
	}
	if !has(p.Args, "--model", "m1") || !has(p.Args, "--effort", "high") || !has(p.Args, "--permission-mode", "auto") {
		t.Fatalf("args = %v", p.Args)
	}
	if !has(p.Args, "--dangerously-load-development-channels=server:a", "server:b") {
		t.Fatalf("channels = %v", p.Args)
	}
	if p.Env["OTEL_EXPORTER_OTLP_ENDPOINT"] != "http://localhost:9766" || p.Env["OTEL_RESOURCE_ATTRIBUTES"] != "rex.block=block:1,rex.session=session:1" {
		t.Fatalf("env = %v", p.Env)
	}
	if !reflect.DeepEqual(p.Channels, []string{"a", "b"}) {
		t.Fatalf("channels = %v", p.Channels)
	}
}

// With nothing set, the CLI's own defaults stand.
func TestNoOverridesAddNoFlags(t *testing.T) {
	p := Build(Input{Account: config.Account{Provider: "claude", Name: "alpha"}, Home: user + "/.claude-alpha", UserHome: user})
	if len(p.Args) != 0 {
		t.Fatalf("args = %v", p.Args)
	}
}

// Claude files its login in the Keychain under the home's path, so naming the
// default home would lose the login that is already there.
func TestDefaultClaudeHomeIsNotNamed(t *testing.T) {
	p := Build(Input{Account: config.Account{Provider: "claude", Name: "alpha"}, Home: user + "/.claude", UserHome: user})
	if _, ok := p.Env["CLAUDE_CONFIG_DIR"]; ok {
		t.Fatalf("env = %v", p.Env)
	}
}

func TestCodexStaysOnItsDaemon(t *testing.T) {
	a := config.Account{Provider: "codex", Name: "alpha", Launch: &config.Launch{Model: "m1"}}
	p := Build(Input{Account: a, Home: user + "/.codex-alpha", UserHome: user, Channels: []string{"ignored"}})
	if p.Program != "codex" || !reflect.DeepEqual(p.Args, []string{"-m", "m1"}) || p.Env["CODEX_HOME"] != user+"/.codex-alpha" {
		t.Fatalf("plan = %+v", p)
	}
	if !reflect.DeepEqual(p.Daemon, []string{"app-server", "daemon", "start"}) || p.Channels != nil {
		t.Fatalf("plan = %+v", p)
	}
}

func TestCredentialGoesThroughCop(t *testing.T) {
	a := config.Account{Provider: "codex", Name: "cloud", Kind: config.Azure, Launch: &config.Launch{
		Model: "m1", Credential: &config.Credential{Account: "team.example", Ref: "op://v/i/f", Env: "API_KEY"}}}
	p := Build(Input{Account: a, Home: user + "/.codex-cloud", UserHome: user})
	want := []string{"run", "--account", "team.example", "--ref", "op://v/i/f", "--env", "API_KEY", "--", "codex", "-m", "m1"}
	if p.Program != "cop" || !reflect.DeepEqual(p.Args, want) {
		t.Fatalf("plan = %+v", p)
	}
}

func TestToolsMergeAndABadFileIsReported(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "one.json"), []byte(`{"claude":{"channel_servers":["a"]}}`), 0o600)
	os.WriteFile(filepath.Join(dir, "two.json"), []byte(`{"claude":{"channel_servers":["b"]},"codex":{}}`), 0o600)
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{`), 0o600)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(`x`), 0o600)
	tools, errs := LoadTools(dir)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "bad.json") {
		t.Fatalf("errs = %v", errs)
	}
	if got := Channels(tools, "claude"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("channels = %v", got)
	}
}

func TestMissingToolsFolderIsNoTools(t *testing.T) {
	tools, errs := LoadTools(filepath.Join(t.TempDir(), "none"))
	if len(tools) != 0 || len(errs) != 0 {
		t.Fatalf("tools = %v errs = %v", tools, errs)
	}
}

const dialog = `WARNING: Loading development channels

--dangerously-load-development-channels is for local channel development only.

Channels: server:a server:b

❯ 1. I am using this for local development
  2. Exit
`

func TestReadsTheDialog(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		want   []string
		got    State
	}{
		{"exact", dialog, []string{"b", "a"}, Answer},
		{"another server", dialog, []string{"a"}, Foreign},
		{"cursor elsewhere", strings.Replace(strings.Replace(dialog, "❯ 1", "  1", 1), "  2", "❯ 2", 1), []string{"a", "b"}, Foreign},
		{"wrapped list", strings.Replace(dialog, "server:a server:b", "server:a\n          server:b", 1), []string{"a", "b"}, Answer},
		{"trust", "Do you trust this folder?\n❯ No, exit\n  Yes, I trust this folder", []string{"a"}, Trust},
		{"session", "Welcome to Claude Code\n> ", []string{"a"}, Nothing},
	}
	for _, c := range cases {
		if got := Read(c.screen, c.want); got != c.got {
			t.Errorf("%s: got %v, want %v", c.name, got, c.got)
		}
	}
}

type pane struct {
	screens []string
	entered int
	fail    error
}

func (p *pane) Capture(context.Context, string) (string, error) {
	if p.fail != nil {
		return "", p.fail
	}
	s := p.screens[0]
	if len(p.screens) > 1 {
		p.screens = p.screens[1:]
	}
	return s, nil
}

func (p *pane) Enter(context.Context, string) error { p.entered++; return nil }

func watch(p *pane, want []string) string {
	var clock time.Time
	return Watch(context.Background(), p, "block:1", want, WatchTiming{
		Wait: 3 * time.Second, Step: time.Second, Limit: 30 * time.Second,
		Now: func() time.Time { return clock }, Sleep: func(d time.Duration) { clock = clock.Add(d) },
	})
}

func TestWatchAnswersOnce(t *testing.T) {
	p := &pane{screens: []string{"starting", dialog}}
	if got := watch(p, []string{"a", "b"}); !strings.Contains(got, "answered") || p.entered != 1 {
		t.Fatalf("got %q, entered %d", got, p.entered)
	}
}

func TestWatchLeavesAForeignDialog(t *testing.T) {
	p := &pane{screens: []string{dialog}}
	if got := watch(p, []string{"a"}); !strings.Contains(got, "left for the operator") || p.entered != 0 {
		t.Fatalf("got %q, entered %d", got, p.entered)
	}
}

// The operator's trust answer can come long after launch, and the channels
// dialog may follow it, so the wait restarts while the trust dialog shows.
func TestWatchWaitsOutTheTrustDialog(t *testing.T) {
	trust := "Yes, I trust this folder"
	p := &pane{screens: []string{trust, trust, trust, trust, trust, trust, dialog}}
	if got := watch(p, []string{"a", "b"}); !strings.Contains(got, "answered") {
		t.Fatalf("got %q", got)
	}
}

func TestWatchGivesUpQuietly(t *testing.T) {
	if got := watch(&pane{screens: []string{"Welcome"}}, []string{"a"}); !strings.Contains(got, "no dialog") {
		t.Fatalf("got %q", got)
	}
	if got := watch(&pane{fail: errors.New("gone")}, []string{"a"}); !strings.Contains(got, "no dialog") {
		t.Fatalf("got %q", got)
	}
}
