package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

const cache = `{"models":[
 {"slug":"m-one","display_name":"M-One","visibility":"list"},
 {"slug":"m-two","display_name":"M Two","visibility":"list"},
 {"slug":"m-hidden","display_name":"Hidden","visibility":"hide"}]}`

func TestCodexListsTheCacheAndNamesTheDefault(t *testing.T) {
	home := t.TempDir()
	write(t, home, "config.toml", "model = \"m-two\"\nmodel_reasoning_effort = \"medium\"\n[profiles.x]\nmodel = \"other\"\n")
	write(t, home, "models_cache.json", cache)
	e := Codex(home, "alpha", Override{})
	if e.DefaultID != "m-two" || e.Default != "M Two" || e.DefaultEffort != "medium" {
		t.Fatalf("%+v", e)
	}
	if len(e.Models) != 2 || e.Models[0].Display != "M One" || len(e.Models[0].Efforts) != 0 {
		t.Fatalf("%+v", e.Models)
	}
}

func TestCustomProviderServesOnlyItsDeployment(t *testing.T) {
	home := t.TempDir()
	write(t, home, "config.toml", "model = \"deploy-1\"\nmodel_provider = \"cloud\"\n")
	write(t, home, "models_cache.json", cache)
	e := Codex(home, "cloud", Override{})
	if len(e.Models) != 1 || e.Models[0].ID != "deploy-1" {
		t.Fatalf("%+v", e.Models)
	}
}

func TestClaudeResolvesTheAliasToTheVersionSeen(t *testing.T) {
	home := t.TempDir()
	write(t, home, "settings.json", `{"model":"opus","effortLevel":"high"}`)
	e := Claude(home, "alpha", []string{"claude-opus-5-5", "claude-opus-4-8", "<synthetic>", "claude-haiku-5-5"}, "", Override{})
	if e.DefaultID != "opus" || e.Default != "Opus 5.5" || e.DefaultEffort != "high" {
		t.Fatalf("%+v", e)
	}
	if e.Models[1].ID != "opus" || e.Models[1].Display != "Opus 5.5" || e.Models[0].Display != "Fable" {
		t.Fatalf("%+v", e.Models)
	}
}

func TestClaudeReadsAContextSuffix(t *testing.T) {
	home := t.TempDir()
	write(t, home, "settings.json", `{"model":"opus[1m]"}`)
	if e := Claude(home, "alpha", []string{"claude-opus-5-5"}, "", Override{}); e.Default != "Opus 5.5" || e.DefaultID != "opus[1m]" {
		t.Fatalf("%+v", e)
	}
}

// With nothing set, no flag is passed, and the last model seen is only shown.
func TestClaudeWithNoSettingShowsWhatItLastRan(t *testing.T) {
	e := Claude(t.TempDir(), "alpha", []string{"claude-sonnet-5-5"}, "claude-sonnet-5-5", Override{})
	if e.DefaultID != "" || e.Default != "Sonnet 5.5" || e.DefaultEffort != "" {
		t.Fatalf("%+v", e)
	}
}

// A subagent's model is not the home's model, though it still names its alias's version.
func TestClaudeShowsTheMainThreadsModel(t *testing.T) {
	e := Claude(t.TempDir(), "alpha", []string{"claude-haiku-5-5", "claude-fable-5-5"}, "claude-fable-5-5", Override{})
	if e.Default != "Fable 5.5" || e.Models[3].Display != "Haiku 5.5" {
		t.Fatalf("%+v", e)
	}
}

// Swift's PaneShares.modelName applies the same rule; its tests pin the same ids.
func TestDisplayMatchesTheApps(t *testing.T) {
	for id, want := range map[string]string{
		"claude-opus-5-5":           "Opus 5.5",
		"claude-haiku-5-5-20261001": "Haiku 5.5",
		"claude-sonnet-4-20250514":  "Sonnet 4",
		"claude-sonnet-4":           "Sonnet 4",
		"claude-sonnet-20250514":    "Sonnet 20250514",
		"opus":                      "Opus",
		"mystery-model":             "mystery-model",
	} {
		if got := display(id); got != want {
			t.Errorf("%s: got %q, want %q", id, got, want)
		}
	}
}

func TestClaudeNamesADatedVersion(t *testing.T) {
	e := Claude(t.TempDir(), "alpha", []string{"claude-sonnet-4-20250514"}, "claude-sonnet-4-20250514", Override{})
	if e.Default != "Sonnet 4" || e.Models[2].Display != "Sonnet 4" {
		t.Fatalf("%+v", e)
	}
}

// The launch window shows what a launch will run: the account's overrides win.
func TestClaudeOverridesReplaceTheHomesDefault(t *testing.T) {
	home := t.TempDir()
	write(t, home, "settings.json", `{"model":"opus","effortLevel":"high"}`)
	e := Claude(home, "alpha", []string{"claude-sonnet-5-5", "claude-opus-5-5"}, "", Override{Model: "sonnet", Effort: "low"})
	if e.DefaultID != "sonnet" || e.Default != "Sonnet 5.5" || e.DefaultEffort != "low" {
		t.Fatalf("%+v", e)
	}
}

func TestCodexOverrideReplacesTheHomesDefault(t *testing.T) {
	home := t.TempDir()
	write(t, home, "config.toml", "model = \"m-two\"\n")
	write(t, home, "models_cache.json", cache)
	if e := Codex(home, "alpha", Override{Model: "m-one"}); e.DefaultID != "m-one" || e.Default != "M One" {
		t.Fatalf("%+v", e)
	}
}

const ranked = `{"models":[
 {"slug":"m-hidden","display_name":"Hidden","visibility":"hide","priority":0,"default_reasoning_level":"xhigh"},
 {"slug":"m-two","display_name":"M Two","visibility":"list","priority":2,"default_reasoning_level":"high"},
 {"slug":"m-one","display_name":"M One","visibility":"list","priority":1,"default_reasoning_level":"low"}]}`

// With no model in config.toml, Codex takes the first model its picker lists.
func TestCodexResolvesAnUnnamedDefaultFromTheCache(t *testing.T) {
	home := t.TempDir()
	write(t, home, "models_cache.json", ranked)
	if e := Codex(home, "alpha", Override{}); e.DefaultID != "m-one" || e.Default != "M One" || e.DefaultEffort != "low" {
		t.Fatalf("%+v", e)
	}
}

func TestCodexConfigEffortWinsOverTheModels(t *testing.T) {
	home := t.TempDir()
	write(t, home, "config.toml", "model = \"m-two\"\n")
	write(t, home, "models_cache.json", ranked)
	if e := Codex(home, "alpha", Override{}); e.DefaultEffort != "high" {
		t.Fatalf("named model: %+v", e)
	}
	write(t, home, "config.toml", "model = \"m-two\"\nmodel_reasoning_effort = \"medium\"\n")
	if e := Codex(home, "alpha", Override{}); e.DefaultEffort != "medium" {
		t.Fatalf("set effort: %+v", e)
	}
}

// A custom provider serves its deployment, so the cache says nothing about its default.
func TestCustomProviderGuessesNoDefault(t *testing.T) {
	home := t.TempDir()
	write(t, home, "config.toml", "model_provider = \"cloud\"\n")
	write(t, home, "models_cache.json", ranked)
	if e := Codex(home, "cloud", Override{}); e.DefaultID != "" || e.Default != "" || e.DefaultEffort != "" {
		t.Fatalf("%+v", e)
	}
}

// Swift's ModelEntry.models is not optional: one null loses the host's whole answer.
func TestAnEmptyCatalogMarshalsAnEmptyList(t *testing.T) {
	b, err := json.Marshal(Codex(t.TempDir(), "alpha", Override{}))
	if err != nil || !strings.Contains(string(b), `"models":[]`) {
		t.Fatalf("%s, %v", b, err)
	}
}
