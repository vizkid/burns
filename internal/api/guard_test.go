package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/config"
)

// Local tools post with a loopback Host and no Origin. A rebound page sends its
// own Host, and a browser names the page in Origin.
func TestLoopbackRoutesRefuseBrowsers(t *testing.T) {
	_, s := server(t)
	h := s.Handler()
	cases := []struct {
		host, origin string
		want         int
	}{
		{"127.0.0.1:9766", "", 200},
		{"localhost:9766", "", 200},
		{"LOCALHOST:9766", "", 200},
		{"[::1]:9766", "", 200},
		{"localhost", "", 200},
		{"evil.example:9766", "", 403},
		{"127.0.0.1.evil.example:9766", "", 403},
		{"", "", 403},
		{"127.0.0.1:9766", "https://evil.example", 403},
		{"localhost:9766", "null", 403},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/usage", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("host %q origin %q: code %d, want %d", c.host, c.origin, w.Code, c.want)
		}
	}
	r := httptest.NewRequest("POST", "/statusline", strings.NewReader(`{}`))
	r.Host, r.Header["Origin"] = "evil.example", []string{"https://evil.example"}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Errorf("statusline: code %d, want 403", w.Code)
	}
}

// The tailnet's Host is the node's own name, so only the browser check applies.
func TestTailnetRoutesRefuseBrowsers(t *testing.T) {
	_, s := server(t)
	h := s.RemoteHandler()
	r := httptest.NewRequest("GET", "/usage", nil)
	r.Host = "host-1.example.ts.net:9766"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Errorf("app: code %d", w.Code)
	}
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Errorf("browser: code %d", w.Code)
	}
}

// An account collected through an extra home is set up on a host that lacks its home.
func TestAnExtraHomeCountsAsSetUp(t *testing.T) {
	ts, s := server(t)
	cfgPath := filepath.Join(t.TempDir(), "accounts.json")
	os.WriteFile(cfgPath, []byte(`{"accounts":[
	 {"provider":"claude","name":"personal","home":".claude-personal","extra_homes":[".claude"],"kind":"subscription"}]}`), 0o600)
	cfg, err := config.Load(cfgPath, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	s.Config = cfg
	s.HomeExists = func(dir string) bool { return dir == "/home/u/.claude" }
	post(t, ts.URL+"/statusline", `{"transcript_path":"/home/u/.claude/projects/p/s.jsonl",
	 "rate_limits":{"five_hour":{"used_percentage":40,"resets_at":`+itoa(now.Add(time.Hour).Unix())+`}}}`)
	var doc struct {
		Accounts []struct {
			SetUp  bool `json:"set_up"`
			Limits []struct{}
		}
	}
	getJSON(t, ts.URL+"/usage", &doc)
	if !doc.Accounts[0].SetUp || len(doc.Accounts[0].Limits) != 1 {
		t.Fatalf("%+v", doc)
	}
}

// The panel must show what a launch would run.
func TestModelsApplyLaunchOverrides(t *testing.T) {
	ts, s := server(t)
	cfgPath := filepath.Join(t.TempDir(), "accounts.json")
	os.WriteFile(cfgPath, []byte(`{"accounts":[
	 {"provider":"claude","name":"personal","home":".claude-personal","kind":"subscription","launch":{"model":"sonnet","effort":"low"}},
	 {"provider":"codex","name":"bare","home":".codex-bare","kind":"subscription"}]}`), 0o600)
	cfg, err := config.Load(cfgPath, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	s.Config = cfg
	r, err := http.Get(ts.URL + "/models")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	for _, want := range []string{`"default_id":"sonnet"`, `"default_effort":"low"`, `"models":[]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("models lacks %s: %s", want, b)
		}
	}
}
