// Package theme turns a Rex theme id into the colours of Rex's per-block
// set_theme call (docs/SPEC.md, "Pane themes").
package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// bundlePath is themes.json inside the Rex app bundle: Rex's design tokens.
const bundlePath = "Contents/Resources/SuperUI_SuperDesignTokens.bundle/Contents/Resources/themes.json"

// maxFile bounds the themes file; Rex's is about 1 MB.
const maxFile = 16 << 20

// apps are the Rex app names, released first.
var apps = []string{"Rex.app", "Rex Beta.app"}

// Colours are the arguments of Rex's set_theme block call. Palette entries
// read "index=#rrggbb".
type Colours struct {
	Background string   `json:"background"`
	Foreground string   `json:"foreground"`
	Cursor     string   `json:"cursor"`
	Scheme     string   `json:"scheme"`
	Palette    []string `json:"palette"`
}

// Dirs are the folders that may hold the Rex app.
func Dirs(userHome string) []string {
	return []string{"/Applications", filepath.Join(userHome, "Applications")}
}

// Locate returns the first Rex app's themes file in dirs, or "" when this host
// has no Rex app, such as a Linux host.
func Locate(dirs []string) string {
	for _, d := range dirs {
		for _, app := range apps {
			p := filepath.Join(d, app, bundlePath)
			if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
				return p
			}
		}
	}
	return ""
}

type tone struct {
	Red   *float64 `json:"red"`
	Green *float64 `json:"green"`
	Blue  *float64 `json:"blue"`
}

type entry struct {
	ID         string          `json:"id"`
	Appearance string          `json:"appearance"`
	Palette    []tone          `json:"palette"`
	Semantic   map[string]tone `json:"semantic"`
}

// Find reads the theme with this id from a themes file. It uses the Display P3
// terminal colours, the same space as the palette: Rex reads hex as P3 when
// "Use Display P3 Color Space" is on.
func Find(path, id string) (Colours, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Colours{}, err
	}
	if len(data) > maxFile {
		return Colours{}, fmt.Errorf("%s: larger than %d bytes", path, maxFile)
	}
	var themes []entry
	if err := json.Unmarshal(data, &themes); err != nil {
		return Colours{}, fmt.Errorf("%s: %w", path, err)
	}
	for _, t := range themes {
		if t.ID == id {
			return colours(t)
		}
	}
	return Colours{}, fmt.Errorf("Rex has no theme %q", id)
}

func colours(t entry) (Colours, error) {
	c := Colours{Scheme: t.Appearance}
	for _, f := range []struct {
		name string
		out  *string
	}{{"terminalBackground", &c.Background}, {"terminalForeground", &c.Foreground}, {"terminalCursor", &c.Cursor}} {
		v, ok := t.Semantic[f.name]
		if !ok || !v.complete() {
			return Colours{}, fmt.Errorf("theme %q has no %s", t.ID, f.name)
		}
		*f.out = v.hex()
	}
	if len(t.Palette) != 256 {
		return Colours{}, fmt.Errorf("theme %q has %d palette entries, not 256", t.ID, len(t.Palette))
	}
	for i, v := range t.Palette {
		if !v.complete() {
			return Colours{}, errors.New("theme " + t.ID + " has an incomplete palette entry")
		}
		c.Palette = append(c.Palette, fmt.Sprintf("%d=%s", i, v.hex()))
	}
	if c.Scheme != "light" && c.Scheme != "dark" {
		return Colours{}, fmt.Errorf("theme %q has appearance %q", t.ID, t.Appearance)
	}
	return c, nil
}

func (v tone) complete() bool { return v.Red != nil && v.Green != nil && v.Blue != nil }

func (v tone) hex() string {
	return fmt.Sprintf("#%02x%02x%02x", byte8(*v.Red), byte8(*v.Green), byte8(*v.Blue))
}

func byte8(c float64) int { return int(math.Round(math.Min(1, math.Max(0, c)) * 255)) }
