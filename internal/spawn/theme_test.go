package spawn

import (
	"slices"
	"testing"

	"github.com/vizkid/burns/internal/theme"
)

var dusk = &theme.Colours{Background: "#0d1618", Foreground: "#c5d8d8", Cursor: "#3bd3c3", Scheme: "dark", Palette: []string{"0=#172325"}}

const duskArgs = `{"background":"#0d1618","foreground":"#c5d8d8","cursor":"#3bd3c3","scheme":"dark","palette":["0=#172325"]}`

// Spawn applies the account's theme to the new block on the target, so a host
// without the Rex app still gets the colours (docs/SPEC.md, "Pane themes").
func TestSpawnAppliesTheThemeToTheNewPane(t *testing.T) {
	l, other := local(), local()
	other.host = "h2"
	req := request()
	req.Theme = dusk
	res, err := Spawn(t.Context(), l, other, req)
	if err != nil || res.ThemeErr != nil {
		t.Fatalf("err = %v, theme = %v", err, res.ThemeErr)
	}
	want := []string{"block", "call", "-s", "session:mine", "-b", "block:new", "set_theme", duskArgs}
	if !slices.ContainsFunc(other.ran, func(r []string) bool { return slices.Equal(r, want) }) {
		t.Fatalf("ran %q\nwant %q", other.ran, want)
	}
}

func TestSpawnWithoutAThemeMakesNoCall(t *testing.T) {
	l := local()
	if _, err := Spawn(t.Context(), l, l, request()); err != nil {
		t.Fatal(err)
	}
	for _, r := range l.ran {
		if slices.Contains(r, "set_theme") {
			t.Fatalf("ran %q", r)
		}
	}
}

// Colour is cosmetic: a refused call is reported, and the agent still starts.
func TestSpawnKeepsGoingWhenTheThemeFails(t *testing.T) {
	l := local()
	l.fail = "block" // a split beside renames nothing, so only the theme call fails
	req := request()
	req.Beside, req.Theme = true, dusk
	res, err := Spawn(t.Context(), l, l, req)
	if err != nil || res.Agent != "claude" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if res.ThemeErr == nil {
		t.Fatal("a refused theme must be reported")
	}
}
