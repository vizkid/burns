package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/vizkid/burns/internal/spawn"
)

// member is one pane of a --pair: account.provider@model.
type member struct{ Account, Provider, Model string }

// parsePair reads '[viz.claude@opus5.5, nfx.codex@sol6.1]'. The brackets and
// each @model are optional; exactly two members are required.
func parsePair(s string) ([2]member, error) {
	var pair [2]member
	parts := strings.Split(strings.Trim(strings.TrimSpace(s), "[]"), ",")
	if len(parts) != 2 {
		return pair, fmt.Errorf("--pair takes two members, a cap and a nav, such as '[viz.claude@opus5.5, viz.codex@astra6]'")
	}
	for i, p := range parts {
		p = strings.TrimSpace(p)
		who, mdl, hasModel := strings.Cut(p, "@")
		account, provider, ok := strings.Cut(who, ".")
		switch {
		case !ok || account == "" || (provider != "claude" && provider != "codex"):
			return pair, fmt.Errorf("%q is not account.provider, with provider claude or codex", who)
		case hasModel && mdl == "":
			return pair, fmt.Errorf("%q names no model after @", p)
		}
		pair[i] = member{account, provider, mdl}
	}
	return pair, nil
}

// model is one entry of an account's GET /models list.
type model struct {
	ID      string `json:"id"`
	Display string `json:"display"`
}

// resolveModel turns a model ID or a short name such as sol6.1 into one of
// the account's model IDs (docs/SPEC.md, "Model names").
func resolveModel(name string, models []model) (string, error) {
	var found []string
	for _, m := range models {
		if strings.EqualFold(m.ID, name) {
			return m.ID, nil
		}
	}
	words, digits := nameParts(name)
	for _, m := range models {
		if namesMatch(words, digits, m.ID) || namesMatch(words, digits, m.Display) {
			found = append(found, m.ID)
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	what := "no model"
	if len(found) > 1 {
		what = "more than one model (" + strings.Join(found, ", ") + ")"
	}
	return "", fmt.Errorf("%q matches %s; the account has %s", name, what, strings.Join(ids, ", "))
}

// nameParts splits a name into its lowercase letter words and its digits:
// "GPT 6.1 Sol" is {gpt, sol} and "61".
func nameParts(s string) (map[string]bool, string) {
	words := map[string]bool{}
	var digits, word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			words[word.String()] = true
			word.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r):
			word.WriteRune(r)
		case unicode.IsDigit(r):
			flush()
			digits.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return words, digits.String()
}

func namesMatch(words map[string]bool, digits, candidate string) bool {
	have, d := nameParts(candidate)
	// A name without digits matches on words alone: "astra" is gpt-6-astra.
	if digits != "" && d != digits || len(words) == 0 {
		return false
	}
	for w := range words {
		if !have[w] {
			return false
		}
	}
	return true
}

// pair starts the cap in a new tab and the nav beside it (docs/SPEC.md, "Pair").
func (sp *spawner) pair(list, tab string, getenv func(string) string) int {
	if tab == "" {
		fmt.Fprintln(sp.errOut, "--pair needs --tab: the cap opens that tab")
		return 2
	}
	members, err := parsePair(list)
	if err != nil {
		fmt.Fprintln(sp.errOut, err)
		return 2
	}
	if members, err = sp.resolveModels(members); err != nil {
		fmt.Fprintln(sp.errOut, "spawn:", err)
		return 1
	}
	if code := sp.setUp(getenv); code != 0 {
		return code
	}
	var reqs [2]spawn.Request
	for i, m := range members {
		warnQuota(sp.base, m.Provider, m.Account, sp.errOut)
		reqs[i] = sp.request(m.Provider, m.Account, m.Model, tab+[]string{"-cap", "-nav"}[i], tab)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*sp.wait+30*time.Second)
	defer cancel()
	res, errs := spawn.Pair(ctx, sp.local(), sp.target(), reqs[0], reqs[1])
	code := 0
	for i := range res {
		logSpawn(sp.home, reqs[i], sp.host, res[i], errs[i])
		if c := report(sp.out, sp.errOut, sp.host, res[i], errs[i]); code == 0 {
			code = c
		}
	}
	return code
}

// resolveModels turns each member's model name into one of its account's IDs.
func (sp *spawner) resolveModels(members [2]member) ([2]member, error) {
	var lists struct {
		Accounts []struct {
			Provider string  `json:"provider"`
			Account  string  `json:"account"`
			Models   []model `json:"models"`
		} `json:"accounts"`
	}
	if err := getJSON(sp.base+"/models", &lists); err != nil {
		return members, fmt.Errorf("model names need smithersd's model list: %w", err)
	}
	for i, m := range members {
		if m.Model == "" {
			continue
		}
		known := false
		for _, a := range lists.Accounts {
			if a.Provider == m.Provider && a.Account == m.Account {
				known = true
				id, err := resolveModel(m.Model, a.Models)
				if err != nil {
					return members, fmt.Errorf("%s.%s: %w", m.Account, m.Provider, err)
				}
				members[i].Model = id
			}
		}
		if !known {
			return members, fmt.Errorf("smithersd lists no models for %s.%s", m.Account, m.Provider)
		}
	}
	return members, nil
}
