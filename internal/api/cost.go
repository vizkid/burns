package api

import (
	"cmp"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/vizkid/burns/internal/billing"
	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/price"
	"github.com/vizkid/burns/internal/store"
)

// CostHour is one hour of an account's work on one model, at list price. Micros covers the
// priced requests, and Unpriced counts the rest: an unknown price is never zero.
type CostHour struct {
	Hour     time.Time `json:"hour"`
	Provider string    `json:"provider"`
	Account  string    `json:"account"`
	Model    string    `json:"model"`
	Requests int64     `json:"requests"`
	Tokens   int64     `json:"tokens"`
	Micros   int64     `json:"micros"`
	Unpriced int64     `json:"unpriced"`
}

// cost answers hourly list prices and billed readings. since is a Go duration, default 192h: a
// week's window from any point in it. The apps sum hours across hosts, so each host reports only
// its own work.
func (s *Server) cost(w http.ResponseWriter, r *http.Request) {
	window := 192 * time.Hour
	if v := r.URL.Query().Get("since"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			http.Error(w, "since must be a positive duration such as 24h", http.StatusBadRequest)
			return
		}
		window = min(d, store.HourRetention)
	}
	hours, err := s.Store.Hours(s.Now().Add(-window))
	if err != nil {
		fail(w, "read hours", err)
		return
	}
	billed, err := s.billed(s.Now().Add(-window))
	if err != nil {
		fail(w, "read billed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "hours": priced(hours, s.Config, price.Default()), "billed": orEmpty(billed)})
}

// priced groups hours by model and works out their dollars. An hour's requests are grouped by
// speed and band only to price them.
func priced(hours []store.Hour, cfg config.Config, table price.Table) []CostHour {
	kinds := map[[2]string]string{}
	for _, a := range cfg.Accounts {
		kinds[[2]string{a.Provider, a.Name}] = a.Kind
	}
	type key struct {
		at                       int64
		provider, account, model string
	}
	byKey := map[key]*CostHour{}
	for _, h := range hours {
		k := key{h.Hour.UnixNano(), h.Provider, h.Account, h.Model}
		c := byKey[k]
		if c == nil {
			c = &CostHour{Hour: h.Hour, Provider: h.Provider, Account: h.Account, Model: h.Model}
			byKey[k] = c
		}
		basis := price.Basis(h.Provider, kinds[[2]string{h.Provider, h.Account}])
		c.Requests += h.Requests
		c.Tokens += h.Input + h.Cached + h.Write5m + h.Write1h + h.Output
		micros, ok := table.Cost(basis, h.Model, h.Speed, h.Hour, price.Tokens{Input: h.Input, Cached: h.Cached, Write: h.Write5m,
			WriteLong: h.Write1h, Output: h.Output, Long: table.IsLong(basis, h.Model, h.Band)})
		if ok {
			c.Micros += micros
		} else {
			c.Unpriced += h.Requests
		}
	}
	out := make([]CostHour, 0, len(byKey))
	for _, c := range byKey {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b CostHour) int {
		return cmp.Or(a.Hour.Compare(b.Hour), cmp.Compare(a.Provider, b.Provider), cmp.Compare(a.Account, b.Account), cmp.Compare(a.Model, b.Model))
	})
	return out
}

// billed joins the stored provider readings with what credit balances show.
func (s *Server) billed(since time.Time) ([]store.Billed, error) {
	out, err := s.Store.BilledAll()
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].History, err = s.Store.BilledHistory(out[i].Provider, out[i].Account, since); err != nil {
			return nil, err
		}
	}
	now := s.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, a := range s.Config.Accounts {
		if a.Provider != "codex" || a.Kind != config.Subscription {
			continue
		}
		balances, err := s.Store.CreditBalances(a.Provider, a.Name, a.Provider, month)
		if err != nil {
			return nil, err
		}
		if b, ok := creditSpend(a, balances); ok {
			out = append(out, b)
		}
	}
	return out, nil
}

// creditSpend adds up the falls in a month's balances. A rise is a reload, never negative spend.
// Two readings are the least that show a fall, and fewer is no reading, not zero. With no price
// set the amount stays in credits, to two places.
func creditSpend(a config.Account, balances []store.CreditBalance) (store.Billed, bool) {
	if len(balances) < 2 {
		return store.Billed{}, false
	}
	unit := 1.0
	currency := "credits"
	if p := a.CreditPrice; p != nil {
		unit, currency = p.Amount, p.Currency
	}
	spent := 0.0
	history := []store.BilledPoint{{ObservedAt: balances[0].ObservedAt}}
	for i := 1; i < len(balances); i++ {
		if d := balances[i-1].Balance - balances[i].Balance; d > 0 {
			spent += d * unit
			history = append(history, store.BilledPoint{UsedMinor: minorUnits(spent), ObservedAt: balances[i].ObservedAt})
		}
	}
	return store.Billed{Provider: a.Provider, Account: a.Name, Source: billing.SourceCredits, Currency: currency, Exponent: 2, Enabled: true,
		UsedMinor: minorUnits(spent), ObservedAt: balances[len(balances)-1].ObservedAt, History: history}, true
}

// minorUnits rounds an amount to hundredths.
func minorUnits(amount float64) int64 { return int64(math.Round(amount * 100)) }
