// Package billing reads what providers say they charged (docs/SPEC.md, "Proposal: billed spend
// and list price"). The Claude reading uses an undocumented endpoint (Viz, 2026-11-02), so any
// failure means no reading, never zero, and a change in its shape costs only that reading.
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/store"
	"github.com/vizkid/burns/internal/usage"
)

// UsageURL is Claude's usage endpoint. Interval keeps its polling gentle: the same endpoint
// rate-limits Claude Code's own /usage screen.
const (
	UsageURL  = "https://api.anthropic.com/api/oauth/usage"
	Interval  = 15 * time.Minute
	oauthBeta = "oauth-2025-04-20"
	maxBody   = 1 << 20
)

// SourceClaude names readings from the usage endpoint.
const SourceClaude = "claude-usage"

// SourceCredits names spend worked out from falls in a credit balance.
const SourceCredits = "codex-credits"

// ErrNoToken means this host has no usable login for an account. Most hosts lack most accounts.
var ErrNoToken = errors.New("no usable token")

// amount is one money value as the endpoint states it.
type amount struct {
	Minor    int64  `json:"amount_minor"`
	Currency string `json:"currency"`
	Exponent int    `json:"exponent"`
}

type usageDoc struct {
	Spend *struct {
		Used  *amount `json:"used"`
		Limit *amount `json:"limit"`
	} `json:"spend"`
	Extra *struct {
		Enabled bool   `json:"is_enabled"`
		Reason  string `json:"disabled_reason"`
	} `json:"extra_usage"`
}

// parse reads a response. A shape it does not know is an error, not a zero.
func parse(body []byte) (store.Billed, error) {
	var d usageDoc
	if err := json.Unmarshal(body, &d); err != nil {
		return store.Billed{}, err
	}
	if d.Spend == nil || d.Spend.Used == nil || d.Spend.Used.Currency == "" || d.Extra == nil {
		return store.Billed{}, errors.New("no spend in response")
	}
	b := store.Billed{Source: SourceClaude, Currency: d.Spend.Used.Currency, UsedMinor: d.Spend.Used.Minor,
		Exponent: d.Spend.Used.Exponent, Enabled: d.Extra.Enabled, Reason: d.Extra.Reason}
	if d.Spend.Limit != nil {
		b.LimitMinor = &d.Spend.Limit.Minor
	}
	return b, nil
}

// Poller reads each Claude subscription account that has a login on this host.
type Poller struct {
	Store  *store.Store
	Config config.Config
	Client *http.Client
	URL    string
	Token  func(home string) (string, error)
	// CodexURL and CodexLogin read Codex accounts. A nil CodexLogin skips them.
	CodexURL   string
	CodexLogin func(home string) (token, account string, err error)
	Now        func() time.Time
	failed     map[string]string
}

// Poll reads every account once. It logs a failure once until it changes, and never the token.
func (p *Poller) Poll(ctx context.Context) {
	if p.failed == nil {
		p.failed = map[string]string{}
	}
	for _, a := range p.Config.Accounts {
		if a.Kind != config.Subscription {
			continue
		}
		var err error
		switch {
		case a.Provider == usage.Claude:
			err = p.one(ctx, a)
		case a.Provider == usage.Codex && p.CodexLogin != nil:
			err = p.codex(ctx, a)
		default:
			continue
		}
		key := a.Provider + "/" + a.Name
		if text := fmt.Sprint(err); err != nil && !errors.Is(err, ErrNoToken) && p.failed[key] != text {
			log.Printf("WARN billing %s: %v", key, err)
			p.failed[key] = text
		} else if err == nil {
			delete(p.failed, key)
		}
	}
}

func (p *Poller) one(ctx context.Context, a config.Account) error {
	token, err := p.Token(p.Config.HomeDir(a))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", oauthBeta)
	req.Header.Set("Accept", "application/json")
	res, err := p.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("usage endpoint answered %d", res.StatusCode)
	}
	body, err := readAll(res)
	if err != nil {
		return err
	}
	b, err := parse(body)
	if err != nil {
		return err
	}
	b.Provider, b.Account, b.ObservedAt = a.Provider, a.Name, p.Now()
	return p.Store.SetBilled(b)
}

// Run polls until ctx ends.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(Interval)
	defer t.Stop()
	for {
		p.Poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
