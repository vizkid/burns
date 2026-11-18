package main

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/vizkid/burns/internal/config"
)

// title is the terminal title a launch sets: `<icon> <folder> · <provider>
// <account>` (docs/SPEC.md, "Shell launchers"). It is written to the terminal,
// so control characters are dropped.
func title(a config.Account, dir, userHome string) string {
	folder := filepath.Base(dir)
	if filepath.Clean(dir) == filepath.Clean(userHome) {
		folder = "~"
	}
	text := folder + " · " + a.Provider + " " + a.Name
	if a.Emoji != "" {
		text = a.Emoji + " " + text
	}
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
	return "\x1b]0;" + text + "\x1b\\"
}

// noPause reports --no-pause among Burns' own arguments, those before `--`. It
// is read apart from the flag set because a refused flag set must still honour it.
func noPause(args []string) bool {
	if i := slices.Index(args, "--"); i >= 0 {
		args = args[:i]
	}
	return slices.ContainsFunc(args, func(s string) bool {
		return s == "--no-pause" || s == "-no-pause" || s == "--no-pause=true" || s == "-no-pause=true"
	})
}
