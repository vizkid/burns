package spawn

import (
	"slices"
	"strings"
	"testing"

	"github.com/vizkid/burns/internal/theme"
)

// Spawn resolves the theme on the caller's host and passes it to the launch,
// so a host without the Rex app still gets the colours (docs/SPEC.md, "Pane themes").
func TestSpawnPassesTheThemeToTheLaunch(t *testing.T) {
	l := local()
	req := request()
	c := theme.Colours{Background: "#0d1618", Foreground: "#c5d8d8", Cursor: "#3bd3c3"}
	for i := range c.Palette {
		c.Palette[i] = "#000000"
	}
	req.Theme = &c
	if _, err := Spawn(t.Context(), l, l, req); err != nil {
		t.Fatal(err)
	}
	cmd := l.ran[0]
	i := slices.Index(cmd, "--colours")
	if i < 0 || !strings.HasPrefix(cmd[i+1], "0d1618,c5d8d8,3bd3c3,000000,") || cmd[len(cmd)-2] != "--dir" {
		t.Fatalf("ran %.200q", cmd)
	}
}

func TestSpawnWithoutAThemePassesNoColours(t *testing.T) {
	l := local()
	if _, err := Spawn(t.Context(), l, l, request()); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(l.ran[0], "--colours") {
		t.Fatalf("ran %q", l.ran[0])
	}
}
