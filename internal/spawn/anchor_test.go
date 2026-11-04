package spawn

import (
	"slices"
	"strings"
	"testing"

	"github.com/vizkid/rexkit/rex"
)

// withLane adds a lane's pane to the caller's session, as an anchor target.
func withLane(f *fakeRex, panes ...rex.Block) *fakeRex {
	f.blocks["session:mine"] = append(f.blocks["session:mine"], panes...)
	return f
}

// --anchor splits beside the named pane, not the caller's, and needs no --tab.
func TestSpawnAnchorSplitsBesideTheNamedPane(t *testing.T) {
	l := withLane(local(), rex.Block{ID: "block:nav", Label: "nav"})
	req := request()
	req.Anchor, req.Tab = "nav", ""
	res, err := Spawn(t.Context(), l, l, req)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{"split", "-s", "session:mine", "-b", "block:nav", "--label", "helper", "--focus=false", "--json"}, launchCmd...)
	if len(l.ran) != 1 || !slices.Equal(l.ran[0], want) {
		t.Errorf("ran %q\nwant %q", l.ran, want)
	}
	if res.SessionID != "session:mine" || res.Block != "block:new" {
		t.Errorf("result %+v", res)
	}
}

// On another host the anchor lives in the session with the caller's label there.
func TestSpawnAnchorWorksOnAnotherHost(t *testing.T) {
	l := local()
	other := local()
	other.host = "h2"
	other.sessions = []rex.Session{{ID: "session:there", Label: "proj"}}
	other.blocks = map[string][]rex.Block{"session:there": {{ID: "block:nav2", Label: "nav"}}}
	req := request()
	req.Anchor = "nav"
	res, err := Spawn(t.Context(), l, other, req)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{"split", "-s", "session:there", "-b", "block:nav2", "--label", "helper", "--focus=false", "--json"}, launchCmd...)
	if len(other.ran) != 1 || !slices.Equal(other.ran[0], want) || len(l.ran) != 0 {
		t.Errorf("ran there %q, here %q\nwant %q", other.ran, l.ran, want)
	}
	if res.SessionID != "session:there" {
		t.Errorf("session %q", res.SessionID)
	}
}

// A missing or repeated anchor label is refused before Rex runs anything.
func TestSpawnAnchorRefusesAGuess(t *testing.T) {
	for _, c := range []struct {
		name  string
		panes []rex.Block
		want  string
	}{
		{"missing", nil, `no pane labelled "nav"`},
		{"repeated", []rex.Block{{ID: "block:a", Label: "nav"}, {ID: "block:b", Label: "nav"}}, "block:a, block:b"},
	} {
		l := withLane(local(), c.panes...)
		req := request()
		req.Anchor = "nav"
		_, err := Spawn(t.Context(), l, l, req)
		if err == nil || !strings.Contains(err.Error(), c.want) || len(l.ran) != 0 {
			t.Errorf("%s: err %v, ran %q; want it to mention %q", c.name, err, l.ran, c.want)
		}
	}
}

// An anchor on a host without the caller's session has nothing to split.
func TestSpawnAnchorNeedsTheSessionOnThatHost(t *testing.T) {
	l, other := local(), local()
	other.host = "h2"
	other.sessions = nil
	req := request()
	req.Anchor = "nav"
	if _, err := Spawn(t.Context(), l, other, req); err == nil || len(other.ran) != 0 {
		t.Fatalf("err %v, ran %q", err, other.ran)
	}
}
