package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The shape of smithersd's GET /usage/panes, with synthetic names: one pane on two
// models, and one pane that never compacted.
const panesBody = `{"host":"h1","checked":true,"panes":[
{"provider":"claude","account":"alpha","block":"block:1","model":"claude-haiku-5-5","tokens":900,"main_tokens":0,"agents":3,"compactions":2,"last_compaction":"2026-10-29T10:00:00Z","session":"work","label":"cap"},
{"provider":"claude","account":"alpha","block":"block:1","model":"claude-fable-5-1","tokens":50,"main_tokens":50,"agents":3,"compactions":2,"last_compaction":"2026-10-29T10:00:00Z","session":"work","label":"cap"},
{"provider":"codex","account":"beta","block":"block:2","model":"gpt-6-astra","tokens":40,"main_tokens":40,"agents":0,"compactions":0,"last_compaction":null,"session":"work","label":""}]}`

func runPanes(t *testing.T, body string) (string, int) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage/panes" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	now = func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) }
	local = time.UTC
	t.Cleanup(func() { now, local = time.Now, time.Local })
	var out, errOut bytes.Buffer
	code := run([]string{"panes", "--url", ts.URL}, strings.NewReader(""), &out, &errOut, env(nil))
	return out.String() + errOut.String(), code
}

// One line per pane, named by its Rex label (or block), with the model its own
// thread ran most and its compactions since it took the session.
func TestPanesPrintsOneLinePerPaneWithCompactions(t *testing.T) {
	out, code := runPanes(t, panesBody)
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("want heading, header and 2 panes:\n%s", out)
	}
	for _, want := range []string{"h1", "Compactions"} {
		if !strings.Contains(lines[0]+lines[1], want) {
			t.Errorf("heading lacks %q:\n%s", want, out)
		}
	}
	cap := strings.Fields(lines[2])
	if strings.Join(cap, " ") != "work cap claude alpha claude-fable-5-1 +3 2 2h ago" {
		t.Errorf("cap line = %q", lines[2])
	}
	bare := strings.Fields(lines[3])
	if strings.Join(bare, " ") != "work block:2 codex beta gpt-6-astra 0 never" {
		t.Errorf("unlabelled line = %q", lines[3])
	}
}

// Rule 9: with no answer from Rex the panes are unconfirmed, and the output says so.
func TestPanesSaysWhenRexDidNotAnswer(t *testing.T) {
	out, code := runPanes(t, strings.Replace(panesBody, `"checked":true`, `"checked":false`, 1))
	if code != 0 || !strings.Contains(out, "Rex did not answer") {
		t.Fatalf("code %d:\n%s", code, out)
	}
}

// Rule 9: a smithersd older than the compaction count sends no field, which is
// unknown, not zero.
func TestPanesFromAnOlderSmithersdSayUnknown(t *testing.T) {
	old := `{"host":"h1","checked":true,"panes":[
{"provider":"codex","account":"beta","block":"block:2","model":"gpt-6-astra","tokens":40,"main_tokens":40,"agents":0,"session":"work","label":"nav"}]}`
	out, code := runPanes(t, old)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 0 || strings.Join(strings.Fields(lines[len(lines)-1]), " ") != "work nav codex beta gpt-6-astra unknown unknown" {
		t.Fatalf("code %d:\n%s", code, out)
	}
}
