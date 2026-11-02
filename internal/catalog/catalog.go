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

// Override is an account's launch overrides, which replace the home's defaults.
type Override struct{ Model, Effort string }

// Entry is one account's catalog. DefaultID is the model a launch runs with no
// flag: the account's override, or the home's. It is empty when neither names
// one: Default is then the model last seen, for showing only.
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

var (
	claudeID = regexp.MustCompile(`^claude-([a-z]+)-\d`)
	snapshot = regexp.MustCompile(`^\d{8}$`)
)

// display turns claude-opus-5-5 into Opus 5.5 and a bare family into its name.
// It leaves out a dated snapshot, and any other shape shows its id. Swift's
// PaneShares.modelName applies the same rule to the same ids.
func display(id string) string {
	parts := strings.Split(id, "-")
	if n := len(parts); n > 3 && snapshot.MatchString(parts[n-1]) {
		parts = parts[:n-1]
	}
	switch {
	case len(parts) >= 3 && parts[0] == "claude":
		return title(parts[1]) + " " + strings.Join(parts[2:], ".")
	case len(parts) == 1:
		return title(id)
	}
	return id
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Claude reads a home's settings. seen is the account's model ids, newest
// first, which name each alias's current version. last is the model its main
// thread ran last: a subagent's model says nothing about what the home runs.
func Claude(home, account string, seen []string, last string, over Override) Entry {
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
	e.DefaultEffort = first(over.Effort, s.EffortLevel)
	e.DefaultID = first(over.Model, s.Model)
	family, _, _ := strings.Cut(e.DefaultID, "[") // opus[1m] names a context size
	switch {
	case latest[family] != "":
		e.Default = display(latest[family])
	case family != "":
		e.Default = display(family)
	case last != "":
		e.Default = display(last)
	}
	return e
}

// first is the first value that is set.
func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

type cached struct {
	Slug       string `json:"slug"`
	Display    string `json:"display_name"`
	Visibility string `json:"visibility"`
	Priority   int    `json:"priority"`
	Effort     string `json:"default_reasoning_level"`
}

var (
	tomlTable = regexp.MustCompile(`^\s*\[`)
	tomlKey   = regexp.MustCompile(`^\s*(model|model_provider|model_reasoning_effort)\s*=\s*"([^"]*)"`)
)

// Codex reads a home's config.toml and models_cache.json, which Codex refreshes.
// A custom provider serves only what config.toml names: each is a deployment.
// Efforts stay empty: Codex's effort is the home's, fixed at launch.
func Codex(home, account string, over Override) Entry {
	e := Entry{Provider: "codex", Account: account, Models: []Model{}} // an empty list, never null: Swift requires it
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
	e.DefaultID = first(over.Model, e.DefaultID)
	efforts := map[string]string{}
	var top *cached
	for i, m := range body.Models {
		names[m.Slug] = strings.ReplaceAll(m.Display, "-", " ")
		efforts[m.Slug] = m.Effort
		if !custom && m.Visibility != "hide" && m.Slug != "" {
			e.Models = append(e.Models, Model{ID: m.Slug, Display: names[m.Slug]})
			if top == nil || m.Priority < top.Priority {
				top = &body.Models[i]
			}
		}
	}
	// With no model named, Codex takes the first model its picker lists, the
	// listed one with the lowest priority.
	if e.DefaultID == "" && top != nil {
		e.DefaultID = top.Slug
	}
	e.DefaultEffort = first(e.DefaultEffort, efforts[e.DefaultID])
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
