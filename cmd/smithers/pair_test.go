package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A member is account.provider@model; brackets and the model are optional.
func TestParsePair(t *testing.T) {
	got, err := parsePair("[viz.claude@opus5.5, nfx.codex@sol6.1]")
	if err != nil {
		t.Fatal(err)
	}
	want := [2]member{{"viz", "claude", "opus5.5"}, {"nfx", "codex", "sol6.1"}}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
	if got, err := parsePair("viz.claude,viz.codex@gpt-6-astra"); err != nil || got[0].Model != "" || got[1].Model != "gpt-6-astra" {
		t.Errorf("got %+v, %v", got, err)
	}
	for _, bad := range []string{"", "viz.claude@opus", "a.claude,b.codex,c.codex", "viz@opus,nfx.codex", "viz.gemini,viz.codex", "viz.claude@,viz.codex"} {
		if _, err := parsePair(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

var codexModels = []model{{ID: "gpt-6.1-sol", Display: "GPT 6.1 Sol"}, {ID: "gpt-6-sol", Display: "GPT 6 Sol"},
	{ID: "gpt-6-astra", Display: "GPT 6 Astra"}, {ID: "gpt-6.1-terra", Display: "GPT 6.1 Terra"}}
var claudeModels = []model{{ID: "opus", Display: "Opus 5.5"}, {ID: "fable", Display: "Fable 5.1"}, {ID: "sonnet", Display: "Sonnet"}}

// A short name's words and digits pick one model by ID or display name.
func TestResolveModel(t *testing.T) {
	for _, c := range []struct {
		name, want string
		models     []model
	}{
		{"sol6.1", "gpt-6.1-sol", codexModels},
		{"sol6", "gpt-6-sol", codexModels},
		{"astra6", "gpt-6-astra", codexModels},
		{"astra", "gpt-6-astra", codexModels},
		{"gpt-6-astra", "gpt-6-astra", codexModels},
		{"opus5.5", "opus", claudeModels},
		{"Opus", "opus", claudeModels},
		{"fable5.1", "fable", claudeModels},
	} {
		got, err := resolveModel(c.name, c.models)
		if err != nil || got != c.want {
			t.Errorf("%q: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	for _, bad := range []string{"sol", "luna6", "opus6", "6.1"} {
		_, err := resolveModel(bad, append(codexModels, claudeModels...))
		if err == nil || !strings.Contains(err.Error(), "gpt-6.1-sol") {
			t.Errorf("%q: err %v; want a refusal listing the models", bad, err)
		}
	}
}

// A single pane's --model takes short names too, and a bad one is refused
// before any pane opens. Seen live on 2026-11-06: opus5.5 reached Claude as is.
func TestSpawnResolvesModel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"accounts":[{"provider":"claude","account":"nfx","models":[{"id":"opus","display":"Opus 5.5"},{"id":"fable","display":"Fable 5.1"}]}]}`))
	}))
	defer ts.Close()
	sp := spawner{base: ts.URL}
	if got, err := sp.modelID("claude", "nfx", "opus5.5"); err != nil || got != "opus" {
		t.Errorf("got %q, %v; want opus", got, err)
	}
	if got, err := sp.modelID("claude", "nfx", ""); err != nil || got != "" {
		t.Errorf("no model: got %q, %v; want the account's default", got, err)
	}
	pane := map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}
	var out, errOut bytes.Buffer
	args := []string{"spawn", "--provider", "claude", "--account", "nfx", "--model", "opus6", "--label", "x", "--tab", "t", "--url", ts.URL}
	if code := run(args, strings.NewReader(""), &out, &errOut, env(pane)); code != 1 || !strings.Contains(errOut.String(), "opus, fable") {
		t.Errorf("code %d, stderr %q; want 1 and the account's models", code, errOut.String())
	}
}

// --pair stands in for the single-pane flags and needs --tab.
func TestSpawnPairRefusesSingleFlags(t *testing.T) {
	pane := map[string]string{"REX_SESSION": "session:s", "REX_BLOCK": "block:b"}
	for _, args := range [][]string{
		{"--pair", "viz.claude,viz.codex"},
		{"--pair", "viz.claude,viz.codex", "--tab", "t", "--label", "x"},
		{"--pair", "viz.claude,viz.codex", "--tab", "t", "--provider", "claude"},
		{"--pair", "viz.claude,viz.codex", "--tab", "t", "--model", "opus"},
		{"--pair", "viz.claude,viz.codex", "--tab", "t", "--anchor", "a"},
		{"--pair", "viz.claude,viz.codex", "--tab", "t", "--beside"},
	} {
		var out, errOut bytes.Buffer
		if code := run(append([]string{"spawn"}, args...), strings.NewReader(""), &out, &errOut, env(pane)); code != 2 {
			t.Errorf("%v: code %d, stderr %q; want 2", args, code, errOut.String())
		}
	}
}
