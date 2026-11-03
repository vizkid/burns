package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/store"
)

// CodexUsageURL is the daily credit report in Codex's own source. It is undocumented, like the
// Claude endpoint, so the same rule holds: any failure is no reading, never zero. Credit balances
// are null on some plans, and this still reports. Its 2026-10-21 and 22 figures added up to the
// fall in a real balance, to a hundredth of a credit.
const CodexUsageURL = "https://chatgpt.com/backend-api/wham/usage/daily-workspace-user-token-usage-breakdown"

// SourceCodex names readings from the daily report.
const SourceCodex = "codex-usage"

// codexDoc is the part of the report Burns reads.
type codexDoc struct {
	Units     string     `json:"units"`
	Freshness *time.Time `json:"data_freshness_ts"`
	Data      []struct {
		Date    string `json:"date"`
		Premium struct {
			Credits map[string]float64 `json:"credit_usage_credits"`
		} `json:"premium_usage_values"`
	} `json:"data"`
}

// CodexDay is one day's credits.
type CodexDay struct {
	Day     time.Time
	Credits float64
}

// parseCodex reads a report. A shape it does not know is an error, not a zero.
func parseCodex(body []byte) ([]CodexDay, time.Time, error) {
	var d codexDoc
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, time.Time{}, err
	}
	if d.Units != "credits" || d.Freshness == nil || d.Data == nil {
		return nil, time.Time{}, errors.New("no credits in response")
	}
	days := make([]CodexDay, 0, len(d.Data))
	for _, row := range d.Data {
		day, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return nil, time.Time{}, err
		}
		// A real idle day lists each surface at 0, so a missing map is no reading, not a zero.
		if row.Premium.Credits == nil {
			return nil, time.Time{}, fmt.Errorf("no credits for %s", row.Date)
		}
		total := 0.0
		for _, v := range row.Premium.Credits {
			total += v
		}
		days = append(days, CodexDay{Day: day, Credits: total})
	}
	return days, *d.Freshness, nil
}

// codexBilled turns days into a month's total and a running total per day, so a window's spend
// is a rise in that running total. With no price set, amounts stay in credits.
func codexBilled(a config.Account, days []CodexDay, freshness, now time.Time) (store.Billed, []store.BilledPoint) {
	unit, currency := 1.0, "credits"
	if p := a.CreditPrice; p != nil {
		unit, currency = p.Amount, p.Currency
	}
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	running, inMonth := 0.0, 0.0
	points := make([]store.BilledPoint, 0, len(days))
	for _, d := range days {
		running += d.Credits * unit
		if !d.Day.Before(month) {
			inMonth += d.Credits * unit
		}
		// A day counts at its end, so one that a window starts during is not lost. Spend should show.
		points = append(points, store.BilledPoint{UsedMinor: minor(running), ObservedAt: d.Day.Add(24*time.Hour - time.Second)})
	}
	return store.Billed{Provider: a.Provider, Account: a.Name, Source: SourceCodex, Currency: currency, UsedMinor: minor(inMonth),
		Exponent: 2, Enabled: true, ObservedAt: freshness}, points
}

func minor(amount float64) int64 { return int64(math.Round(amount * 100)) }

// codexLogin is the part of Codex's auth.json that Burns reads.
type codexLogin struct {
	Tokens struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	} `json:"tokens"`
}

// StoredCodexLogin reads a home's Codex login. It never refreshes one: Codex does that itself.
func StoredCodexLogin(home string) (token, account string, err error) {
	raw, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return "", "", ErrNoToken
	}
	var l codexLogin
	if json.Unmarshal(raw, &l) != nil || l.Tokens.AccessToken == "" {
		return "", "", ErrNoToken
	}
	return l.Tokens.AccessToken, l.Tokens.AccountID, nil
}

// codex reads one account's report.
func (p *Poller) codex(ctx context.Context, a config.Account) error {
	token, account, err := p.CodexLogin(p.Config.HomeDir(a))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.CodexURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "burns")
	if account != "" {
		req.Header.Set("ChatGPT-Account-Id", account)
	}
	res, err := p.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("credit report answered %d", res.StatusCode)
	}
	body, err := readAll(res)
	if err != nil {
		return err
	}
	days, freshness, err := parseCodex(body)
	if err != nil {
		return err
	}
	b, points := codexBilled(a, days, freshness, p.Now())
	return p.Store.SetBilledDays(b, points)
}
