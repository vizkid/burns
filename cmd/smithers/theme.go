package main

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/theme"
)

// themeFile is this host's Rex themes file: the Rex app's, or else the copy a
// deploy leaves in the Burns config folder. It is "" with neither.
func themeFile(userHome string) string {
	return themeSource(theme.Locate(theme.Dirs(userHome)), config.Dir(userHome))
}

// themeSource prefers the Rex app's file, which is current, to the copy.
func themeSource(appFile, configDir string) string {
	if appFile != "" {
		return appFile
	}
	p := filepath.Join(configDir, "themes.json")
	if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
		return p
	}
	return ""
}

// applyTheme writes the launch's colours into its own pane before the agent
// starts (docs/SPEC.md, "Pane themes"). Colours that spawn passed win, since
// the host may lack the Rex app; otherwise the account's theme is read here.
// It returns the line for launch.log, or "" when there is nothing to apply.
// Nothing here fails the launch.
func applyTheme(a config.Account, file, passed string, out io.Writer, block string) string {
	var c theme.Colours
	var err error
	switch {
	case passed != "":
		c, err = theme.Decode(passed)
	case a.Theme == "":
		return ""
	case file == "":
		return block + ": theme " + a.Theme + " not applied: this host has no Rex app or themes.json copy, and spawn passed no colours"
	default:
		c, err = theme.Find(file, a.Theme)
	}
	if err == nil {
		_, err = io.WriteString(out, c.Codes())
	}
	name := cmp.Or(a.Theme, "passed colours")
	if err != nil {
		return fmt.Sprintf("%s: theme %s not applied: %v", block, name, err)
	}
	return block + ": theme " + name + " applied"
}

// accountLook is an account's emoji and resolved theme on this host, for
// spawn. A theme this host cannot resolve is a warning, never a refusal.
func accountLook(home, provider, name string, errOut io.Writer) (string, *theme.Colours) {
	cfg, err := config.Load(filepath.Join(config.Dir(home), "accounts.json"), home)
	if err != nil {
		return "", nil
	}
	a, _ := cfg.Find(provider, name)
	if a.Theme == "" {
		return a.Emoji, nil
	}
	file := themeFile(home)
	if file == "" {
		fmt.Fprintf(errOut, "warning: theme %s not applied: this host has no Rex app\n", a.Theme)
		return a.Emoji, nil
	}
	c, err := theme.Find(file, a.Theme)
	if err != nil {
		fmt.Fprintf(errOut, "warning: theme %s not applied: %v\n", a.Theme, err)
		return a.Emoji, nil
	}
	return a.Emoji, &c
}
