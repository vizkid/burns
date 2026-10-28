package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/store"
)

var now = time.Date(2026, 10, 28, 4, 0, 0, 0, time.UTC)

func server(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "accounts.json")
	os.WriteFile(cfgPath, []byte(`{"accounts":[
	 {"provider":"claude","name":"personal","home":".claude-personal","kind":"subscription"},
	 {"provider":"codex","name":"azure","home":".codex-azure","kind":"azure"}]}`), 0o600)
	cfg, err := config.Load(cfgPath, "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st, Config: cfg, Host: "host-1", Now: func() time.Time { return now }}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	r, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	r, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 {
		t.Fatalf("GET %s = %d %s", url, r.StatusCode, b)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
}

type usageDoc struct {
	Host     string `json:"host"`
	Accounts []struct {
		Provider string `json:"provider"`
		Account  string `json:"account"`
		Kind     string `json:"kind"`
		Limits   []struct {
			LimitID string `json:"limit_id"`
			Windows map[string]struct {
				Percent  *float64 `json:"percent"`
				State    string   `json:"state"`
				ResetsAt string   `json:"resets_at"`
			} `json:"windows"`
		} `json:"limits"`
	} `json:"accounts"`
}

// Rule 9: unknown or stale data never displays as zero.
func TestUsageShowsUnknownNotZero(t *testing.T) {
	ts, _ := server(t)
	status := `{"session_id":"s1","transcript_path":"/home/u/.claude-personal/projects/p/s1.jsonl",
	 "rate_limits":{"five_hour":{"used_percentage":0,"resets_at":` + itoa(now.Add(time.Hour).Unix()) + `},
	 "seven_day":{"used_percentage":55,"resets_at":` + itoa(now.Add(-time.Minute).Unix()) + `}}}`
	if r := post(t, ts.URL+"/statusline", status); r.StatusCode/100 != 2 {
		t.Fatalf("statusline = %d", r.StatusCode)
	}
	var doc usageDoc
	getJSON(t, ts.URL+"/usage", &doc)
	if doc.Host != "host-1" || len(doc.Accounts) != 2 {
		t.Fatalf("usage = %+v", doc)
	}
	claude, azure := doc.Accounts[0], doc.Accounts[1]
	if len(claude.Limits) != 1 {
		t.Fatalf("claude limits = %+v", claude.Limits)
	}
	w := claude.Limits[0].Windows
	if p := w["300"].Percent; p == nil || *p != 0 || w["300"].State != "live" {
		t.Fatalf("five-hour zero with future reset is real: %+v", w["300"])
	}
	if _, ok := w["10080"]; ok {
		t.Fatalf("a window reported already expired is not stored: %+v", w["10080"])
	}
	if azure.Kind != "azure" || len(azure.Limits) != 0 {
		t.Fatalf("azure with no reading must list no limits: %+v", azure)
	}
}

func TestExpiredWindowReadsUnknown(t *testing.T) {
	ts, s := server(t)
	post(t, ts.URL+"/statusline", `{"transcript_path":"/home/u/.claude-personal/projects/p/s.jsonl",
	 "rate_limits":{"five_hour":{"used_percentage":40,"resets_at":`+itoa(now.Add(time.Minute).Unix())+`}}}`)
	s.Now = func() time.Time { return now.Add(2 * time.Minute) }
	var doc usageDoc
	getJSON(t, ts.URL+"/usage", &doc)
	w := doc.Accounts[0].Limits[0].Windows["300"]
	if w.Percent != nil || w.State != "unknown" {
		t.Fatalf("expired window = %+v; want percent null, state unknown", w)
	}
}

func TestStatuslineFromUnknownHomeIgnored(t *testing.T) {
	ts, _ := server(t)
	r := post(t, ts.URL+"/statusline", `{"transcript_path":"/elsewhere/s.jsonl","rate_limits":{"five_hour":{"used_percentage":1,"resets_at":`+itoa(now.Add(time.Hour).Unix())+`}}}`)
	if r.StatusCode/100 != 2 {
		t.Fatalf("status = %d", r.StatusCode)
	}
	var doc usageDoc
	getJSON(t, ts.URL+"/usage", &doc)
	if len(doc.Accounts[0].Limits) != 0 {
		t.Fatal("a post from an unconfigured home must not land on an account")
	}
}

const logs = `{"resourceLogs":[{"resource":{"attributes":[
 {"key":"burns.account","value":{"stringValue":"personal"}},
 {"key":"rex.block","value":{"stringValue":"block:b1"}}]},
 "scopeLogs":[{"logRecords":[{"timeUnixNano":"1793160000000000000","attributes":[
   {"key":"event.name","value":{"stringValue":"api_request"}},
   {"key":"session.id","value":{"stringValue":"s1"}},
   {"key":"prompt.id","value":{"stringValue":"p1"}},
   {"key":"request_id","value":{"stringValue":"req_1"}},
   {"key":"input_tokens","value":{"intValue":3}},
   {"key":"output_tokens","value":{"intValue":7}},
   {"key":"cost_usd_micros","value":{"intValue":99}}]}]}]}]}`

func TestLogsToSessions(t *testing.T) {
	ts, _ := server(t)
	if r := post(t, ts.URL+"/v1/logs", logs); r.StatusCode != 200 {
		t.Fatalf("logs = %d", r.StatusCode)
	}
	post(t, ts.URL+"/v1/logs", logs)
	if r := post(t, ts.URL+"/v1/logs", `{"resourceLogs":[`); r.StatusCode != 200 {
		t.Fatalf("malformed export must answer 200 so the exporter does not retry: %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/v1/metrics", `{}`); r.StatusCode != 200 {
		t.Fatalf("metrics = %d", r.StatusCode)
	}
	var doc struct {
		Sessions []store.Group `json:"sessions"`
		Blocks   []store.Group `json:"blocks"`
	}
	getJSON(t, ts.URL+"/usage/sessions?since=24h", &doc)
	if len(doc.Sessions) != 1 || doc.Sessions[0].Requests != 1 || doc.Sessions[0].Output != 7 || doc.Sessions[0].CostMicros == nil || *doc.Sessions[0].CostMicros != 99 {
		t.Fatalf("sessions = %+v", doc.Sessions)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Block != "block:b1" {
		t.Fatalf("blocks = %+v", doc.Blocks)
	}
	var turns struct {
		Turns []store.Group `json:"turns"`
	}
	getJSON(t, ts.URL+"/usage/sessions?provider=claude&session=s1", &turns)
	if len(turns.Turns) != 1 || turns.Turns[0].Turn != "p1" {
		t.Fatalf("turns = %+v", turns)
	}
	var hist struct {
		Buckets []store.Bucket `json:"buckets"`
	}
	getJSON(t, ts.URL+"/usage/history", &hist)
}

func TestBindAndLimits(t *testing.T) {
	ts, _ := server(t)
	r := post(t, ts.URL+"/bind", `{"provider":"claude","session":"s1","rex_session":"session:r1","rex_block":"block:b9","source":"startup"}`)
	if r.StatusCode/100 != 2 {
		t.Fatalf("bind = %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/bind", `{"provider":"claude","session":"s1"}`); r.StatusCode != 400 {
		t.Fatalf("bind without a block must be 400, got %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/statusline", strings.Repeat("x", MaxStatusline+1)); r.StatusCode != 413 {
		t.Fatalf("oversized statusline = %d; want 413", r.StatusCode)
	}
	if r, _ := http.Get(ts.URL + "/v1/logs"); r.StatusCode != 405 {
		t.Fatalf("GET /v1/logs = %d; want 405", r.StatusCode)
	}
}

func itoa(v int64) string { b, _ := json.Marshal(v); return string(b) }
