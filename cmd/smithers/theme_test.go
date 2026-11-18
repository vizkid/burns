package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/theme"
)

// themesFile writes a themes.json with one theme, "t1", whose colours are all black.
func themesFile(t *testing.T) string {
	t.Helper()
	black := `{"colorSpace":"display-p3","red":0,"green":0,"blue":0,"alpha":1}`
	palette := strings.TrimSuffix(strings.Repeat(black+",", 256), ",")
	body := `[{"id":"t1","appearance":"dark","palette":[` + palette + `],"semantic":{"terminalBackground":` + black +
		`,"terminalForeground":` + black + `,"terminalCursor":` + black + `}}]`
	p := filepath.Join(t.TempDir(), "themes.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLaunchWritesTheAccountThemeIntoItsPane(t *testing.T) {
	var out strings.Builder
	line := applyTheme(config.Account{Theme: "t1"}, themesFile(t), "", &out, "block:b")
	if !strings.Contains(out.String(), "\x1b]11;#000000\x1b\\") || !strings.Contains(line, "t1 applied") {
		t.Fatalf("wrote %.40q, log %q", out.String(), line)
	}
}

func TestLaunchWithoutAThemeWritesNothing(t *testing.T) {
	var out strings.Builder
	if line := applyTheme(config.Account{}, themesFile(t), "", &out, "b"); line != "" || out.Len() != 0 {
		t.Fatalf("log %q, wrote %d bytes", line, out.Len())
	}
}

// A host without the Rex app uses the colours spawn resolved on the caller's host.
func TestLaunchUsesTheColoursSpawnPassed(t *testing.T) {
	c, err := theme.Find(themesFile(t), "t1")
	if err != nil {
		t.Fatal(err)
	}
	c.Background = "#102030"
	var out strings.Builder
	line := applyTheme(config.Account{Theme: "t1"}, "", c.Encode(), &out, "b")
	if !strings.Contains(out.String(), "\x1b]11;#102030\x1b\\") || !strings.Contains(line, "applied") {
		t.Fatalf("wrote %.40q, log %q", out.String(), line)
	}
}

func TestLaunchLogsAThemeItCannotApply(t *testing.T) {
	var out strings.Builder
	for _, c := range []struct{ theme, file, passed, want string }{
		{"t1", "", "", "no Rex app"},
		{"nope", themesFile(t), "", "nope"},
		{"t1", themesFile(t), "\x1b]0;evil", "not applied"},
	} {
		line := applyTheme(config.Account{Theme: c.theme}, c.file, c.passed, &out, "b")
		if !strings.Contains(line, c.want) {
			t.Errorf("%+v: log %q", c, line)
		}
	}
	if out.Len() != 0 {
		t.Fatalf("wrote %q", out.String())
	}
}
