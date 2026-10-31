package catalog

import (
	"os"
	"path/filepath"
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
	e := Codex(home, "alpha")
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
	e := Codex(home, "cloud")
	if len(e.Models) != 1 || e.Models[0].ID != "deploy-1" {
		t.Fatalf("%+v", e.Models)
	}
}

func TestClaudeResolvesTheAliasToTheVersionSeen(t *testing.T) {
	home := t.TempDir()
	write(t, home, "settings.json", `{"model":"opus","effortLevel":"high"}`)
	e := Claude(home, "alpha", []string{"claude-opus-5-5", "claude-opus-4-8", "<synthetic>", "claude-haiku-5-5"})
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
	if e := Claude(home, "alpha", []string{"claude-opus-5-5"}); e.Default != "Opus 5.5" || e.DefaultID != "opus[1m]" {
		t.Fatalf("%+v", e)
	}
}

// With nothing set, no flag is passed, and the last model seen is only shown.
func TestClaudeWithNoSettingShowsWhatItLastRan(t *testing.T) {
	e := Claude(t.TempDir(), "alpha", []string{"claude-sonnet-5-5"})
	if e.DefaultID != "" || e.Default != "Sonnet 5.5" || e.DefaultEffort != "" {
		t.Fatalf("%+v", e)
	}
}
