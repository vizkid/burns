// Package catalog lists the models each account can run on this host, read
// from the CLIs themselves so a new model needs no Burns release
// (docs/SPEC.md, "Steps").
package catalog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Model is one choice. ID is what the CLI's model flag takes.
type Model struct {
	ID      string   `json:"id"`
	Display string   `json:"display"`
	Efforts []string `json:"efforts,omitempty"`
}

// Entry is one account's catalog. DefaultID is the model the home runs with no
// flag. It is empty when the home names none: Default is then the model last
// seen, for showing only.
type Entry struct {
	Provider      string  `json:"provider"`
	Account       string  `json:"account"`
	DefaultID     string  `json:"default_id"`
	Default       string  `json:"default"`
	DefaultEffort string  `json:"default_effort"`
	Models        []Model `json:"models"`
}

// claudeEfforts are the efforts Claude Code's --effort takes.
var claudeEfforts = []string{"low", "medium", "high", "xhigh", "max"}

// claudeFamilies are the documented --model aliases. Each follows the family's
// latest version, so only a new family needs a change here.
var claudeFamilies = []string{"fable", "opus", "sonnet", "haiku"}

var claudeID = regexp.MustCompile(`^claude-([a-z]+)-(\d+)-(\d+)`)

// display turns claude-opus-5-5 into Opus 5.5, and a bare family into its name.
func display(id string) string {
	if m := claudeID.FindStringSubmatch(id); m != nil {
		return title(m[1]) + " " + m[2] + "." + m[3]
	}
	return title(id)
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Claude reads a home's settings. seen is the account's model ids, newest
// first, which name each alias's current version.
func Claude(home, account string, seen []string) Entry {
	e := Entry{Provider: "claude", Account: account}
	latest := map[string]string{}
	for _, id := range seen {
		if m := claudeID.FindStringSubmatch(id); m != nil && latest[m[1]] == "" {
			latest[m[1]] = id
		}
	}
	for _, f := range claudeFamilies {
		name := f
		if id := latest[f]; id != "" {
			name = id
		}
		e.Models = append(e.Models, Model{ID: f, Display: display(name), Efforts: claudeEfforts})
	}
	var s struct {
		Model       string `json:"model"`
		EffortLevel string `json:"effortLevel"`
	}
	if data, err := os.ReadFile(filepath.Join(home, "settings.json")); err == nil {
		json.Unmarshal(data, &s) // a home with bad settings still lists its models
	}
	e.DefaultEffort = s.EffortLevel
	e.DefaultID = s.Model
	switch {
	case s.Model != "" && latest[s.Model] != "":
		e.Default = display(latest[s.Model])
	case s.Model != "":
		e.Default = display(s.Model)
	case len(seen) > 0:
		e.Default = display(seen[0])
	}
	return e
}

type cached struct {
	Slug       string `json:"slug"`
	Display    string `json:"display_name"`
	Visibility string `json:"visibility"`
	Default    string `json:"default_reasoning_level"`
}

var (
	tomlTable = regexp.MustCompile(`^\s*\[`)
	tomlKey   = regexp.MustCompile(`^\s*(model|model_provider|model_reasoning_effort)\s*=\s*"([^"]*)"`)
)

// Codex reads a home's config.toml and models_cache.json, which Codex refreshes.
// A custom provider serves only what config.toml names: each is a deployment.
// Efforts stay empty: Codex's effort is the home's, fixed at launch.
func Codex(home, account string) Entry {
	e := Entry{Provider: "codex", Account: account}
	provider := ""
	if f, err := os.Open(filepath.Join(home, "config.toml")); err == nil {
		defer f.Close()
		for scan := bufio.NewScanner(f); scan.Scan(); {
			line := scan.Text()
			if tomlTable.MatchString(line) {
				break // only top-level keys: the file's tables hold other models
			}
			if m := tomlKey.FindStringSubmatch(line); m != nil {
				switch m[1] {
				case "model":
					e.DefaultID = m[2]
				case "model_provider":
					provider = m[2]
				case "model_reasoning_effort":
					e.DefaultEffort = m[2]
				}
			}
		}
	}
	names := map[string]string{}
	var body struct {
		Models []cached `json:"models"`
	}
	if data, err := os.ReadFile(filepath.Join(home, "models_cache.json")); err == nil {
		json.Unmarshal(data, &body) // no cache leaves the config's model
	}
	custom := provider != "" && provider != "openai"
	for _, m := range body.Models {
		names[m.Slug] = strings.ReplaceAll(m.Display, "-", " ")
		if !custom && m.Visibility != "hide" && m.Slug != "" {
			e.Models = append(e.Models, Model{ID: m.Slug, Display: names[m.Slug]})
		}
	}
	e.Default = names[e.DefaultID]
	if e.Default == "" {
		e.Default = e.DefaultID
	}
	if e.DefaultID != "" {
		listed := false
		for _, m := range e.Models {
			listed = listed || m.ID == e.DefaultID
		}
		if !listed {
			e.Models = append([]Model{{ID: e.DefaultID, Display: e.Default}}, e.Models...)
		}
	}
	return e
}
