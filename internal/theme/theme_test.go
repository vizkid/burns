package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// p3 is one Display P3 colour as Rex's design tokens write it.
func p3(r, g, b float64) map[string]any {
	return map[string]any{"colorSpace": "display-p3", "red": r, "green": g, "blue": b, "alpha": 1}
}

// fixture writes a synthetic themes.json with one dark theme, "test-dusk".
// Its palette entry 1 is out of range on purpose, to check the clamp.
func fixture(t *testing.T) string {
	t.Helper()
	palette := make([]map[string]any, 256)
	for i := range palette {
		palette[i] = p3(0, 0, 0)
	}
	palette[0] = p3(0.088535, 0.137132, 0.146398)
	palette[1] = p3(1.2, -0.1, 0.5)
	palette[255] = p3(1, 1, 1)
	themes := []map[string]any{
		{"id": "other", "appearance": "light", "palette": palette, "semantic": map[string]any{}},
		{"id": "test-dusk", "appearance": "dark", "palette": palette, "semantic": map[string]any{
			"terminalBackground": p3(0.05153, 0.086115, 0.095076),
			"terminalForeground": p3(0.774244, 0.84535, 0.846747),
			"terminalCursor":     p3(0.229989, 0.829352, 0.764771),
		}},
	}
	data, err := json.Marshal(themes)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "themes.json")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindConvertsTheThemeToHex(t *testing.T) {
	c, err := Find(fixture(t), "test-dusk")
	if err != nil {
		t.Fatal(err)
	}
	if c.Background != "#0d1618" || c.Foreground != "#c5d8d8" || c.Cursor != "#3bd3c3" || c.Scheme != "dark" {
		t.Errorf("colours = %+v", c)
	}
	if len(c.Palette) != 256 {
		t.Fatalf("palette has %d entries", len(c.Palette))
	}
	for _, want := range []string{"0=#172325", "1=#ff0080", "255=#ffffff"} {
		if !slices.Contains(c.Palette, want) {
			t.Errorf("palette lacks %s: %v", want, c.Palette[:2])
		}
	}
}

func TestFindRefusesAnUnknownTheme(t *testing.T) {
	_, err := Find(fixture(t), "no-such")
	if err == nil || !strings.Contains(err.Error(), "no-such") {
		t.Fatalf("err = %v", err)
	}
}

func TestFindRefusesAThemeWithoutTerminalColours(t *testing.T) {
	if _, err := Find(fixture(t), "other"); err == nil {
		t.Fatal("a theme without terminal colours must be refused, not sent as black")
	}
}

func TestLocateFindsTheRexApp(t *testing.T) {
	home := t.TempDir()
	if got := Locate([]string{filepath.Join(home, "none")}); got != "" {
		t.Fatalf("no app gives %q", got)
	}
	beta := filepath.Join(home, "Applications", "Rex Beta.app")
	file := filepath.Join(beta, bundlePath)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Locate([]string{filepath.Join(home, "none"), filepath.Join(home, "Applications")}); got != file {
		t.Fatalf("Locate = %q, want %q", got, file)
	}
}

func TestArgsAreRexSetThemeArguments(t *testing.T) {
	c := Colours{Background: "#000000", Foreground: "#ffffff", Cursor: "#ff0000", Scheme: "dark", Palette: []string{"0=#000000"}}
	got, _ := json.Marshal(c)
	want := `{"background":"#000000","foreground":"#ffffff","cursor":"#ff0000","scheme":"dark","palette":["0=#000000"]}`
	if string(got) != want {
		t.Fatalf("args = %s", got)
	}
}
