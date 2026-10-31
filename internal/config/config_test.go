package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, body string) (Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "accounts.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(p, "/home/u")
}

const good = `{"accounts":[
 {"provider":"claude","name":"personal","home":".claude-personal","kind":"subscription"},
 {"provider":"codex","name":"work","home":"/srv/codex-work","kind":"subscription"},
 {"provider":"codex","name":"azure","home":".codex-azure","kind":"azure"}]}`

func TestLoadAndLookup(t *testing.T) {
	c, err := load(t, good)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Accounts) != 3 || c.Accounts[1].Name != "work" {
		t.Fatalf("accounts = %+v", c.Accounts)
	}
	if got := c.HomeDir(c.Accounts[0]); got != "/home/u/.claude-personal" {
		t.Fatalf("home = %q", got)
	}
	a, ok := c.ForPath("/home/u/.claude-personal/projects/p/s.jsonl")
	if !ok || a.Name != "personal" {
		t.Fatalf("ForPath = %+v, %v", a, ok)
	}
	if _, ok := c.ForPath("/home/u/.claude-personal2/projects/p/s.jsonl"); ok {
		t.Fatal("a home must match as a whole path component")
	}
	if a, ok := c.ForPath("/srv/codex-work/sessions/r.jsonl"); !ok || a.Name != "work" {
		t.Fatalf("absolute home = %+v, %v", a, ok)
	}
	if a, ok := c.Find("codex", "azure"); !ok || a.Kind != Azure {
		t.Fatalf("Find = %+v, %v", a, ok)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field": `{"accounts":[],"extra":1}`,
		"bad provider":  `{"accounts":[{"provider":"gemini","name":"a","home":".g","kind":"subscription"}]}`,
		"bad kind":      `{"accounts":[{"provider":"claude","name":"a","home":".c","kind":"free"}]}`,
		"azure claude":  `{"accounts":[{"provider":"claude","name":"a","home":".c","kind":"azure"}]}`,
		"duplicate":     `{"accounts":[{"provider":"claude","name":"a","home":".c","kind":"subscription"},{"provider":"claude","name":"a","home":".d","kind":"subscription"}]}`,
		"bad name":      `{"accounts":[{"provider":"claude","name":"A b","home":".c","kind":"subscription"}]}`,
		"empty home":    `{"accounts":[{"provider":"claude","name":"a","home":"","kind":"subscription"}]}`,
		"two documents": `{"accounts":[]}{"accounts":[]}`,
		"empty extra":   `{"accounts":[{"provider":"claude","name":"a","home":".c","extra_homes":[" "],"kind":"subscription"}]}`,
		"home twice":    `{"accounts":[{"provider":"claude","name":"a","home":".c","kind":"subscription"},{"provider":"claude","name":"b","home":".d","extra_homes":[".c"],"kind":"subscription"}]}`,
	} {
		if _, err := load(t, body); err == nil {
			t.Errorf("%s: want error", name)
		} else if strings.TrimSpace(err.Error()) == "" {
			t.Errorf("%s: empty error", name)
		}
	}
}

// Another home signed in to the same account counts as that account: its
// files are scanned and its paths belong to the account.
func TestExtraHomesBelongToTheAccount(t *testing.T) {
	c, err := load(t, `{"accounts":[
 {"provider":"claude","name":"personal","home":".claude-personal","extra_homes":[".claude","/srv/claude"],"kind":"subscription"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/home/u/.claude-personal", "/home/u/.claude", "/srv/claude"}
	if got := c.HomeDirs(c.Accounts[0]); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("HomeDirs = %v; want %v", got, want)
	}
	for _, p := range []string{"/home/u/.claude/projects/p/s.jsonl", "/srv/claude/projects/p/s.jsonl"} {
		if a, ok := c.ForPath(p); !ok || a.Name != "personal" {
			t.Fatalf("ForPath(%s) = %+v, %v", p, a, ok)
		}
	}
	if a, ok := c.ForPath("/home/u/.claude-personal/projects/p/s.jsonl"); !ok || a.Name != "personal" {
		t.Fatalf("main home = %+v, %v", a, ok)
	}
}

// The documented example must stay loadable.
func TestExampleLoads(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "docs", "accounts.example.json"), "/home/u")
	if err != nil || len(c.Accounts) == 0 {
		t.Fatalf("example: %v", err)
	}
}

func TestLaunchOverrides(t *testing.T) {
	cases := []struct {
		name, body, wantErr string
	}{
		{"claude", `"provider":"claude","launch":{"model":"m-1","effort":"high","permission_mode":"auto"}`, ""},
		{"flag", `"provider":"claude","launch":{"model":"--evil"}`, "launch settings"},
		{"codex effort", `"provider":"codex","launch":{"effort":"high"}`, "no effort"},
		{"credential", `"provider":"codex","launch":{"credential":{"account":"a.example","ref":"op://v/i/f","env":"KEY"}}`, ""},
		{"bad ref", `"provider":"codex","launch":{"credential":{"account":"a.example","ref":"plain","env":"KEY"}}`, "credential"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(`{"accounts":[{"name":"alpha","home":".h","kind":"subscription",`+c.body+`}]}`), "/home/op")
		if c.wantErr == "" && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%s: got %v", c.name, err)
		}
	}
}
