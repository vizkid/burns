package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type capture struct {
	path string
	body []byte
}

func sink(t *testing.T) (*httptest.Server, chan capture) {
	t.Helper()
	got := make(chan capture, 4)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- capture{r.URL.Path, b}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(ts.Close)
	return ts, got
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestHookPostsBindingAndPrintsNothing(t *testing.T) {
	ts, got := sink(t)
	var out, errOut bytes.Buffer
	in := strings.NewReader(`{"session_id":"s1","source":"resume","transcript_path":"/x"}`)
	code := run([]string{"hook", "session-start", "--provider", "claude", "--url", ts.URL}, in, &out, &errOut,
		env(map[string]string{"REX_BLOCK": "block:b1", "REX_SESSION": "session:r1"}))
	if code != 0 || out.Len() != 0 {
		t.Fatalf("exit %d, stdout %q; want 0 and nothing (Claude adds hook stdout to the conversation)", code, out.String())
	}
	c := <-got
	var b map[string]string
	json.Unmarshal(c.body, &b)
	if c.path != "/bind" || b["session"] != "s1" || b["rex_block"] != "block:b1" || b["rex_session"] != "session:r1" ||
		b["provider"] != "claude" || b["source"] != "resume" {
		t.Fatalf("posted %s %s", c.path, c.body)
	}
}

func TestHookOutsideRexDoesNothing(t *testing.T) {
	ts, got := sink(t)
	var out bytes.Buffer
	code := run([]string{"hook", "session-start", "--provider", "codex", "--url", ts.URL},
		strings.NewReader(`{"session_id":"s1"}`), &out, io.Discard, env(nil))
	if code != 0 || len(got) != 0 || out.Len() != 0 {
		t.Fatalf("exit %d, posts %d; want 0 and none", code, len(got))
	}
}

func TestHookNeverFailsTheAgent(t *testing.T) {
	code := run([]string{"hook", "session-start", "--provider", "claude", "--url", "http://127.0.0.1:1"},
		strings.NewReader(`{"session_id":"s1"}`), io.Discard, io.Discard, env(map[string]string{"REX_BLOCK": "block:b"}))
	if code != 0 {
		t.Fatalf("exit %d; a down smithersd must not fail the session", code)
	}
}

func TestStatuslineForwardsAndPassesThrough(t *testing.T) {
	ts, got := sink(t)
	var out bytes.Buffer
	input := `{"session_id":"s1","rate_limits":{}}`
	code := run([]string{"statusline", "--url", ts.URL, "--", "sh", "-c", "printf 'line:'; cat"},
		strings.NewReader(input), &out, io.Discard, env(nil))
	if code != 0 || out.String() != "line:"+input {
		t.Fatalf("exit %d, stdout %q; want the wrapped command's output", code, out.String())
	}
	if c := <-got; c.path != "/statusline" || string(c.body) != input {
		t.Fatalf("posted %s %s", c.path, c.body)
	}
}

func TestStatuslineWithoutCommandPrintsNothing(t *testing.T) {
	ts, _ := sink(t)
	var out bytes.Buffer
	if code := run([]string{"statusline", "--url", ts.URL}, strings.NewReader(`{}`), &out, io.Discard, env(nil)); code != 0 || out.Len() != 0 {
		t.Fatalf("exit %d, stdout %q", code, out.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	if code := run([]string{"nope"}, strings.NewReader(""), io.Discard, io.Discard, env(nil)); code != 2 {
		t.Fatalf("exit %d; want 2", code)
	}
}
