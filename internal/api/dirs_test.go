package api

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type dirsDoc struct {
	Host string   `json:"host"`
	Home string   `json:"home"`
	Dirs []string `json:"dirs"`
}

// A synthetic home: dev/{stance,Stuff,.hidden,file.txt}, and a link to a folder.
func dirsHome(t *testing.T, s *Server) string {
	t.Helper()
	home := t.TempDir()
	for _, d := range []string{"dev/stance", "dev/Stuff", "dev/.hidden", "other"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(home, "dev", "status.txt"), nil, 0o644)
	os.Symlink(filepath.Join(home, "other"), filepath.Join(home, "dev", "styles"))
	s.Home = home
	return home
}

// The panel completes a typed folder from every host (docs/SPEC.md, "Beside, typed folders and
// new sessions"): folders only, matched on the last part without case, links to folders included.
func TestDirsCompletesTheLastPart(t *testing.T) {
	ts, s := server(t)
	home := dirsHome(t, s)
	var doc dirsDoc
	getJSON(t, ts.URL+"/dirs?prefix=dev/st", &doc)
	if doc.Home != home || doc.Host == "" || !slices.Equal(doc.Dirs, []string{"dev/stance", "dev/Stuff", "dev/styles"}) {
		t.Fatalf("doc = %+v", doc)
	}
	getJSON(t, ts.URL+"/dirs?prefix=dev/", &doc)
	if !slices.Equal(doc.Dirs, []string{"dev/stance", "dev/Stuff", "dev/styles"}) {
		t.Fatalf("children = %v", doc.Dirs)
	}
	getJSON(t, ts.URL+"/dirs?prefix=dev/.h", &doc)
	if !slices.Equal(doc.Dirs, []string{"dev/.hidden"}) {
		t.Fatalf("hidden = %v", doc.Dirs)
	}
	getJSON(t, ts.URL+"/dirs?prefix=", &doc)
	if !slices.Equal(doc.Dirs, []string{"dev", "other"}) {
		t.Fatalf("home = %v", doc.Dirs)
	}
	getJSON(t, ts.URL+"/dirs?prefix=nowhere/x", &doc)
	if doc.Dirs == nil || len(doc.Dirs) != 0 {
		t.Fatalf("missing parent = %#v; want an empty list", doc.Dirs)
	}
}

// Names never leave the home folder.
func TestDirsRefusesPathsOutsideHome(t *testing.T) {
	ts, s := server(t)
	dirsHome(t, s)
	for _, p := range []string{"/etc", "..", "dev/../../x", "dev/../.."} {
		r, err := http.Get(ts.URL + "/dirs?prefix=" + p)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusBadRequest {
			t.Errorf("prefix %q = %d; want 400", p, r.StatusCode)
		}
	}
}

// A long folder lists at most 50.
func TestDirsCapsTheList(t *testing.T) {
	ts, s := server(t)
	home := dirsHome(t, s)
	for i := range 60 {
		os.MkdirAll(filepath.Join(home, "many", "d"+string(rune('a'+i/26))+string(rune('a'+i%26))), 0o755)
	}
	var doc dirsDoc
	getJSON(t, ts.URL+"/dirs?prefix=many/", &doc)
	if len(doc.Dirs) != 50 {
		t.Fatalf("len = %d; want 50", len(doc.Dirs))
	}
}

// A link inside home that points outside it offers nothing, either as a folder or as a parent:
// the lexical checks alone let `outside/` list another tree's folders.
func TestDirsRefusesLinksThatLeaveHome(t *testing.T) {
	ts, s := server(t)
	home := dirsHome(t, s)
	outside := t.TempDir()
	os.MkdirAll(filepath.Join(outside, "secret"), 0o755)
	os.Symlink(outside, filepath.Join(home, "outside"))
	var doc dirsDoc
	getJSON(t, ts.URL+"/dirs?prefix=outside/", &doc)
	if len(doc.Dirs) != 0 {
		t.Fatalf("through an outside link = %v", doc.Dirs)
	}
	getJSON(t, ts.URL+"/dirs?prefix=ou", &doc)
	if slices.Contains(doc.Dirs, "outside") {
		t.Fatalf("an outside link was offered: %v", doc.Dirs)
	}
	getJSON(t, ts.URL+"/dirs?prefix=dev/sty", &doc)
	if !slices.Equal(doc.Dirs, []string{"dev/styles"}) {
		t.Fatalf("a link inside home must still count: %v", doc.Dirs)
	}
}
