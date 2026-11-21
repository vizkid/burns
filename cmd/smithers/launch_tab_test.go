package main

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

// Rex's JSON as `rex block ls --json` and `rex window ls --json` print it.
const (
	tabBlocks = `{"blocks":[
		{"block_id":"block:01a1-other","window_id":"window:01a1fbf1-7c98-724d-8ea3-ea7900000000"},
		{"block_id":"block:01a1-mine","window_id":"window:01a1fbf1-7c98-724d-8ea3-ea79783affdd"}]}`
	tabWindowsDefault = `{"windows":[
		{"window_id":"window:01a1fbf1-7c98-724d-8ea3-ea7900000000","label":"Window 00000000"},
		{"window_id":"window:01a1fbf1-7c98-724d-8ea3-ea79783affdd","label":"Window 783affdd"}]}`
	tabWindowsNamed = `{"windows":[
		{"window_id":"window:01a1fbf1-7c98-724d-8ea3-ea79783affdd","label":"🎱 editor-cap"}]}`
)

// docs/SPEC.md, "Shell launchers", Tab.
func TestTabRenameOnlyReplacesRexDefault(t *testing.T) {
	for _, c := range []struct {
		name, windows, block, wantWindow string
	}{
		{"default label", tabWindowsDefault, "block:01a1-mine", "window:01a1fbf1-7c98-724d-8ea3-ea79783affdd"},
		{"named tab", tabWindowsNamed, "block:01a1-mine", ""},
		{"unknown block", tabWindowsDefault, "block:01a1-gone", ""},
		{"bad json", `{`, "block:01a1-mine", ""},
	} {
		if got := tabToRename([]byte(tabBlocks), []byte(c.windows), c.block); got != c.wantWindow {
			t.Errorf("%s: got %q, want %q", c.name, got, c.wantWindow)
		}
	}
}

func TestTabLabelIsIconAndName(t *testing.T) {
	for _, c := range []struct{ emoji, name, want string }{
		{"⬢", "stance.ai", "⬢ stance.ai"},
		{"", "stance.ai", "stance.ai"},
		{"◆", "  my tab ", "◆ my tab"},
		{"◆", "b\x07ad", "◆ bad"},
	} {
		if got := tabText(c.emoji, c.name); got != c.want {
			t.Errorf("%q: got %q, want %q", c.name, got, c.want)
		}
	}
}

// fakeRex answers the two listings with the given windows and records calls.
func fakeRex(windows string, calls *[][]string, renameErr error) func(args ...string) ([]byte, error) {
	return func(args ...string) ([]byte, error) {
		*calls = append(*calls, args)
		switch {
		case args[0] == "block":
			return []byte(tabBlocks), nil
		case args[1] == "ls":
			return []byte(windows), nil
		}
		return nil, renameErr
	}
}

func renameOf(calls [][]string) string {
	for _, c := range calls {
		if len(c) == 6 && c[1] == "rename" {
			return c[5]
		}
	}
	return ""
}

// docs/SPEC.md, "Shell launchers", Tab: --tab, the prompt, and named tabs.
func TestNameTabChoosesTheName(t *testing.T) {
	asked := 0
	answer := func(reply string) func(string) (string, bool) {
		return func(folder string) (string, bool) { asked++; return reply, true }
	}
	for _, c := range []struct {
		name, windows, flag string
		ask                 func(string) (string, bool)
		wantLabel           string
		wantAsked           int
	}{
		{"flag names it", tabWindowsDefault, "review", answer("ignored"), "⬢ review", 0},
		{"prompt answer", tabWindowsDefault, "", answer("mine"), "⬢ mine", 1},
		{"return takes folder", tabWindowsDefault, "", answer(""), "⬢ stance.ai", 1},
		{"no terminal", tabWindowsDefault, "", nil, "", 0},
		{"named tab, flag", tabWindowsNamed, "review", answer("x"), "", 0},
		{"named tab, prompt", tabWindowsNamed, "", answer("x"), "", 0},
	} {
		asked = 0
		var calls [][]string
		line := nameTab(fakeRex(c.windows, &calls, nil), "stance", "block:01a1-mine", "⬢", c.flag, "stance.ai", c.ask)
		if got := renameOf(calls); got != c.wantLabel || asked != c.wantAsked || line != "" {
			t.Errorf("%s: label %q asked %d line %q; want %q asked %d", c.name, got, asked, line, c.wantLabel, c.wantAsked)
		}
	}
}

// A failure is reported for launch.log and never stops the launch; outside Rex
// nothing runs.
func TestNameTabSurvivesFailure(t *testing.T) {
	var calls [][]string
	line := nameTab(fakeRex(tabWindowsDefault, &calls, errors.New("refused")), "stance", "block:01a1-mine", "⬢", "x", "f", nil)
	want := []string{"window", "rename", "-s", "stance", "window:01a1fbf1-7c98-724d-8ea3-ea79783affdd", "⬢ x"}
	if line == "" || len(calls) != 3 || !slices.Equal(calls[2], want) {
		t.Fatalf("line %q calls %q", line, calls)
	}
	calls = nil
	if line := nameTab(fakeRex(tabWindowsDefault, &calls, nil), "", "block:01a1-mine", "⬢", "x", "f", nil); line != "" || len(calls) != 0 {
		t.Fatalf("outside Rex: line %q, calls %q", line, calls)
	}
}

// The prompt reads one line, byte by byte, so typed-ahead input after it stays
// for the agent.
func TestAskTabReadsOneLine(t *testing.T) {
	in := strings.NewReader("my tab\nhello agent")
	var out strings.Builder
	name, ok := askTab(in, &out)("stance.ai")
	rest, _ := io.ReadAll(in)
	if !ok || name != "my tab" || out.String() != "Tab name [stance.ai]: " || string(rest) != "hello agent" {
		t.Fatalf("name %q ok %v prompt %q rest %q", name, ok, out.String(), rest)
	}
	if _, ok := askTab(strings.NewReader(""), &out)("f"); ok {
		t.Fatal("end of input must skip naming")
	}
}
