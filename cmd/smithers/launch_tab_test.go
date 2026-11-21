package main

import (
	"errors"
	"slices"
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

func TestTabLabelIsIconAndFolder(t *testing.T) {
	for _, c := range []struct{ emoji, dir, want string }{
		{"⬢", "/home/op/dev/stance.ai", "⬢ stance.ai"},
		{"", "/home/op/dev/stance.ai", "stance.ai"},
		{"◆", "/home/op", "◆ ~"},
		{"◆", "/home/op/dev/b\x07ad", "◆ bad"},
	} {
		if got := tabText(c.emoji, c.dir, "/home/op"); got != c.want {
			t.Errorf("%q: got %q, want %q", c.dir, got, c.want)
		}
	}
}

// The rename runs rex three times at most and never fails the launch.
func TestNameTabCallsRexAndSurvivesFailure(t *testing.T) {
	var calls [][]string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		switch args[0] {
		case "block":
			return []byte(tabBlocks), nil
		case "window":
			if args[1] == "ls" {
				return []byte(tabWindowsDefault), nil
			}
		}
		return nil, errors.New("rename refused")
	}
	line := nameTab(run, "stance", "block:01a1-mine", "⬢ stance.ai")
	want := []string{"window", "rename", "-s", "stance", "window:01a1fbf1-7c98-724d-8ea3-ea79783affdd", "⬢ stance.ai"}
	if len(calls) != 3 || !slices.Equal(calls[2], want) {
		t.Fatalf("calls %q", calls)
	}
	if line == "" {
		t.Fatal("a failed rename must be reported for launch.log")
	}
	calls = nil
	if line := nameTab(run, "", "block:01a1-mine", "x"); line != "" || len(calls) != 0 {
		t.Fatalf("outside Rex: line %q, calls %q", line, calls)
	}
}
