package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vizkid/burns/internal/config"
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

type calls [][]string

func (c *calls) run(args ...string) error { *c = append(*c, args); return nil }

func TestLaunchAppliesTheAccountThemeToItsBlock(t *testing.T) {
	var ran calls
	line := applyTheme(config.Account{Theme: "t1"}, themesFile(t), ran.run, "session:s", "block:b")
	if len(ran) != 1 || strings.Join(ran[0][:7], " ") != "block call -s session:s -b block:b set_theme" {
		t.Fatalf("ran %q", ran)
	}
	if !strings.Contains(ran[0][7], `"background":"#000000"`) || !strings.Contains(line, "t1") {
		t.Fatalf("args %s, log %q", ran[0][7], line)
	}
}

func TestLaunchWithoutAThemeCallsNothing(t *testing.T) {
	var ran calls
	if line := applyTheme(config.Account{}, themesFile(t), ran.run, "s", "b"); line != "" || len(ran) != 0 {
		t.Fatalf("log %q, ran %q", line, ran)
	}
}

// A host without the Rex app leaves the theme to spawn and says so in the log.
func TestLaunchWithoutTheRexAppLeavesTheThemeToSpawn(t *testing.T) {
	var ran calls
	line := applyTheme(config.Account{Theme: "t1"}, "", ran.run, "s", "b")
	if len(ran) != 0 || !strings.Contains(line, "no Rex app") {
		t.Fatalf("log %q, ran %q", line, ran)
	}
}

func TestLaunchLogsARefusedTheme(t *testing.T) {
	fail := func(...string) error { return errors.New("refused") }
	if line := applyTheme(config.Account{Theme: "t1"}, themesFile(t), fail, "s", "b"); !strings.Contains(line, "refused") {
		t.Fatalf("log %q", line)
	}
	if line := applyTheme(config.Account{Theme: "nope"}, themesFile(t), fail, "s", "b"); !strings.Contains(line, "nope") {
		t.Fatalf("log %q", line)
	}
}
