package spawn

import (
	"slices"
	"testing"
	"time"
)

func pairRequests() (Request, Request) {
	cap, nav := request(), request()
	cap.Label, cap.Tab = "fix9-cap", "fix9"
	nav.Label, nav.Tab, nav.Provider, nav.Account = "fix9-nav", "", "codex", "alpha"
	return cap, nav
}

// The cap opens the tab; the nav splits beside the cap's block, found by ID.
func TestPairSplitsTheNavBesideTheCap(t *testing.T) {
	l := local()
	l.out["window new"] = `{"block_ids":["block:cap"]}`
	l.agent = "claude"
	l.after = 0
	cap, nav := pairRequests()
	nav.Provider = "claude" // the fake's foreground is one agent for every block
	res, errs := Pair(t.Context(), l, l, cap, nav)
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("errs %v", errs)
	}
	if res[0].Block != "block:cap" || res[1].Block != "block:new" {
		t.Fatalf("results %+v", res)
	}
	var split []string
	for _, r := range l.ran {
		if r[0] == "split" {
			split = r
		}
	}
	want := []string{"split", "-s", "session:mine", "-b", "block:cap", "--label", "fix9-nav"}
	if len(split) < len(want) || !slices.Equal(split[:len(want)], want) {
		t.Errorf("split %q\nwant %q…", split, want)
	}
}

// Without a cap pane there is nothing to split beside, so the nav is skipped.
func TestPairSkipsTheNavWithoutACapPane(t *testing.T) {
	l := local()
	l.fail = "window"
	cap, nav := pairRequests()
	res, errs := Pair(t.Context(), l, l, cap, nav)
	if errs[0] == nil || errs[1] == nil || res[1].Block != "" {
		t.Fatalf("results %+v, errs %v", res, errs)
	}
	for _, r := range l.ran {
		if r[0] == "split" {
			t.Errorf("ran a split: %q", r)
		}
	}
}

// A cap whose agent is slow still has a pane, so the nav goes beside it.
func TestPairPlacesTheNavBesideASlowCap(t *testing.T) {
	l := local()
	l.out["window new"] = `{"block_ids":["block:cap"]}`
	l.agent = "" // no agent ever starts
	cap, nav := pairRequests()
	cap.Wait, nav.Wait = 20*time.Millisecond, 20*time.Millisecond
	_, errs := Pair(t.Context(), l, l, cap, nav)
	if errs[0] != ErrNotStarted {
		t.Fatalf("cap err %v", errs[0])
	}
	if !slices.ContainsFunc(l.ran, func(r []string) bool { return r[0] == "split" }) {
		t.Errorf("no split ran: %q", l.ran)
	}
}
