package theme

import (
	"fmt"
	"regexp"
	"strings"
)

// hexPattern is one colour in the --colours argument, without its #.
var hexPattern = regexp.MustCompile(`^[0-9a-f]{6}$`)

// Encode writes the colours as one argument: background, foreground, cursor,
// then the palette, as comma-separated rrggbb. Spawn passes it to a launch on
// a host without the Rex app.
func (c Colours) Encode() string {
	all := append([]string{c.Background, c.Foreground, c.Cursor}, c.Palette[:]...)
	for i, h := range all {
		all[i] = strings.TrimPrefix(h, "#")
	}
	return strings.Join(all, ",")
}

// Decode reads an Encode argument, refusing anything else: it arrives on a
// command line and is written to a terminal.
func Decode(s string) (Colours, error) {
	parts := strings.Split(s, ",")
	var c Colours
	if len(parts) != 3+len(c.Palette) {
		return Colours{}, fmt.Errorf("colours need %d values, got %d", 3+len(c.Palette), len(parts))
	}
	for _, p := range parts {
		if !hexPattern.MatchString(p) {
			return Colours{}, fmt.Errorf("colour %q is not rrggbb", p)
		}
	}
	c.Background, c.Foreground, c.Cursor = "#"+parts[0], "#"+parts[1], "#"+parts[2]
	for i, p := range parts[3:] {
		c.Palette[i] = "#" + p
	}
	return c, nil
}

// Codes are the escape sequences that set a terminal's palette (OSC 4),
// foreground (OSC 10), background (OSC 11) and cursor (OSC 12). The pane's
// own program writes them, so they apply whichever Rex client owns the pane.
func (c Colours) Codes() string {
	var b strings.Builder
	for i, h := range c.Palette {
		fmt.Fprintf(&b, "\x1b]4;%d;%s\x1b\\", i, h)
	}
	fmt.Fprintf(&b, "\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b]12;%s\x1b\\", c.Foreground, c.Background, c.Cursor)
	return b.String()
}
