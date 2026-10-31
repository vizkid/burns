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

const accountsFile = `{"accounts":[
 {"provider":"claude","name":"alpha","home":".claude-alpha","kind":"subscription"}]}`

const accountsEdit = `{"accounts":[
 {"provider":"claude","name":"alpha","home":".claude-alpha","kind":"subscription"},
 {"provider":"codex","name":"beta","home":".codex-beta","kind":"subscription"}]}`

// accountsServer serves a real accounts file and counts restarts.
func accountsServer(t *testing.T) (*httptest.Server, string, *int) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	if err := os.WriteFile(path, []byte(accountsFile), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	restarts := 0
	s := &Server{Store: st, Config: cfg, ConfigPath: path, Host: "host-1", Now: func() time.Time { return now },
		Restart: func() { restarts++ }}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, path, &restarts
}

func getAccounts(t *testing.T, url string) (string, string) {
	t.Helper()
	r, err := http.Get(url + "/accounts")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("GET /accounts = %d %s", r.StatusCode, b)
	}
	return string(b), r.Header.Get("ETag")
}

func putAccounts(t *testing.T, url, etag, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url+"/accounts", strings.NewReader(body))
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r, string(b)
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// GET answers the file as it is on disk, with a hash of it as the ETag.
func TestGetAccountsAnswersTheFileAndItsHash(t *testing.T) {
	ts, _, _ := accountsServer(t)
	body, etag := getAccounts(t, ts.URL)
	if body != accountsFile {
		t.Fatalf("body = %s", body)
	}
	if !strings.HasPrefix(etag, `"`) || len(etag) < 10 {
		t.Fatalf("ETag = %q", etag)
	}
}

// A write with the current ETag replaces the file, mode 0600, answers the new
// ETag, and restarts smithersd once so the collector takes the new accounts.
func TestPutAccountsReplacesTheFileAndRestarts(t *testing.T) {
	ts, path, restarts := accountsServer(t)
	_, etag := getAccounts(t, ts.URL)
	r, b := putAccounts(t, ts.URL, etag, accountsEdit)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("PUT = %d %s", r.StatusCode, b)
	}
	if got := fileText(t, path); got != accountsEdit {
		t.Fatalf("file = %s", got)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
	var doc struct {
		ETag string `json:"etag"`
	}
	if err := json.Unmarshal([]byte(b), &doc); err != nil || doc.ETag == "" || doc.ETag == etag || r.Header.Get("ETag") != doc.ETag {
		t.Fatalf("answer = %s, header %q", b, r.Header.Get("ETag"))
	}
	if *restarts != 1 {
		t.Fatalf("restarts = %d", *restarts)
	}
}

// Without If-Match the write is refused: an app must have read the file first.
func TestPutAccountsNeedsIfMatch(t *testing.T) {
	ts, path, restarts := accountsServer(t)
	if r, b := putAccounts(t, ts.URL, "", accountsEdit); r.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("PUT = %d %s", r.StatusCode, b)
	}
	if fileText(t, path) != accountsFile || *restarts != 0 {
		t.Fatal("a refused write changed the file or restarted")
	}
}

// The file changed since the app read it, by hand or from another device: 412,
// and nothing is overwritten.
func TestPutAccountsWithAStaleETagIsRefused(t *testing.T) {
	ts, path, restarts := accountsServer(t)
	_, etag := getAccounts(t, ts.URL)
	hand := strings.Replace(accountsFile, "alpha", "gamma", 1)
	os.WriteFile(path, []byte(hand), 0o600)
	if r, b := putAccounts(t, ts.URL, etag, accountsEdit); r.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("PUT = %d %s", r.StatusCode, b)
	}
	if fileText(t, path) != hand || *restarts != 0 {
		t.Fatal("a stale write changed the file or restarted")
	}
}

// A list that would not load at startup is refused with the reason, and the file
// is unchanged.
func TestPutAccountsValidatesAsStartupDoes(t *testing.T) {
	ts, path, restarts := accountsServer(t)
	_, etag := getAccounts(t, ts.URL)
	bad := `{"accounts":[{"provider":"claude","name":"Bad Name","home":".claude-x","kind":"subscription"}]}`
	r, b := putAccounts(t, ts.URL, etag, bad)
	if r.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(b, "name") {
		t.Fatalf("PUT = %d %s", r.StatusCode, b)
	}
	if fileText(t, path) != accountsFile || *restarts != 0 {
		t.Fatal("an invalid write changed the file or restarted")
	}
}

// Accounts apply to every host, so a home missing here is accepted: the account
// is "not set up" on this host until its first launch creates the home.
func TestPutAccountsAcceptsAMissingHome(t *testing.T) {
	ts, _, _ := accountsServer(t)
	_, etag := getAccounts(t, ts.URL)
	if r, b := putAccounts(t, ts.URL, etag, accountsEdit); r.StatusCode != http.StatusOK {
		t.Fatalf("PUT with homes missing = %d %s", r.StatusCode, b)
	}
}

// An account whose home is missing on this host lists as not set up, with no
// limits: no usage here is unknown, never zero.
func TestUsageMarksAnAccountWithoutItsHomeNotSetUp(t *testing.T) {
	ts, s := server(t)
	s.HomeExists = func(dir string) bool { return dir == "/home/u/.claude-personal" }
	status := `{"session_id":"s1","transcript_path":"/home/u/.claude-personal/projects/p/s1.jsonl",
	 "rate_limits":{"five_hour":{"used_percentage":5,"resets_at":` + itoa(now.Add(time.Hour).Unix()) + `}}}`
	post(t, ts.URL+"/statusline", status)
	var doc struct {
		Accounts []struct {
			Account string            `json:"account"`
			SetUp   *bool             `json:"set_up"`
			Limits  []json.RawMessage `json:"limits"`
		} `json:"accounts"`
	}
	getJSON(t, ts.URL+"/usage", &doc)
	claude, azure := doc.Accounts[0], doc.Accounts[1]
	if claude.SetUp == nil || !*claude.SetUp || len(claude.Limits) != 1 {
		t.Fatalf("claude = %+v", claude)
	}
	if azure.SetUp == nil || *azure.SetUp || len(azure.Limits) != 0 {
		t.Fatalf("azure = %+v", azure)
	}
}
