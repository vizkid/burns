// Package price turns tokens into list-price dollars (docs/SPEC.md, "Proposal: billed spend and
// list price"). The table is checked in: smithersd never calls a pricing site.
package price

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sync"
	"time"
)

//go:embed prices.json
var pricesJSON []byte

// Rate is one model's price per million tokens on one basis, from a date.
type Rate struct {
	Basis     string `json:"basis"`
	Model     string `json:"model"`
	From      string `json:"from,omitempty"` // YYYY-MM-DD; empty is the start of the table
	AsOf      string `json:"as_of"`
	SourceURL string `json:"source_url"`

	Input     float64 `json:"input"`
	Cached    float64 `json:"cached_input"`
	Write     float64 `json:"cache_write"`
	WriteLong float64 `json:"cache_write_long,omitempty"` // Claude's one-hour lifetime
	Output    float64 `json:"output"`

	// Over LongOver prompt tokens, input and cache rates take LongInput, and output LongOutput.
	LongOver   int64   `json:"long_over,omitempty"`
	LongInput  float64 `json:"long_input,omitempty"`
	LongOutput float64 `json:"long_output,omitempty"`

	// Speeds multiplies every rate. A speed not listed has no known price.
	Speeds map[string]float64 `json:"speeds,omitempty"`
}

// Tokens are the priced parts of one request or a group of like requests. Write is at the
// standard cache rate and WriteLong at the long-lived one.
type Tokens struct {
	Input, Cached, Write, WriteLong, Output int64
	Long                                    bool
}

// Table holds every rate.
type Table struct {
	Rates []Rate `json:"rates"`
}

// Default returns the checked-in table. A table that fails to parse is a build mistake.
var Default = sync.OnceValue(func() Table {
	var t Table
	if err := json.Unmarshal(pricesJSON, &t); err != nil {
		panic("price table: " + err.Error())
	}
	return t
})

var dated = regexp.MustCompile(`-\d{8}$`)

// rate finds the row in force at a time: the latest that has started, else the earliest.
func (t Table) rate(basis, model string, at time.Time) (Rate, bool) {
	model = dated.ReplaceAllString(model, "")
	day := at.UTC().Format("2006-01-02")
	var best, first Rate
	var found, any bool
	for _, r := range t.Rates {
		if r.Basis != basis || r.Model != model {
			continue
		}
		if !any || r.From < first.From {
			first, any = r, true
		}
		if r.From <= day && (!found || r.From > best.From) {
			best, found = r, true
		}
	}
	if found {
		return best, true
	}
	return first, any
}

// Cost is the list price in micro-dollars. False means no price is known: a caller counts the
// tokens as unpriced, never as free.
func (t Table) Cost(basis, model, speed string, at time.Time, n Tokens) (int64, bool) {
	r, ok := t.rate(basis, model, at)
	if !ok {
		return 0, false
	}
	mult := 1.0
	if speed != "" {
		if mult, ok = r.Speeds[speed]; !ok {
			return 0, false
		}
	}
	in, out := mult, mult
	if n.Long && r.LongOver > 0 {
		in, out = mult*r.LongInput, mult*r.LongOutput
	}
	long := r.WriteLong
	if long == 0 {
		long = r.Write
	}
	cost := in*(float64(n.Input)*r.Input+float64(n.Cached)*r.Cached+float64(n.Write)*r.Write+float64(n.WriteLong)*long) + out*float64(n.Output)*r.Output
	return int64(cost + 0.5), true // tokens times dollars per million is micro-dollars
}

// Bands sort requests by prompt size in the hourly totals, one for each limit a provider uses.
// A prompt over BandMid is band 1, over BandHigh band 2. The store assigns bands in SQL with
// these values, so the two cannot drift.
const (
	BandMid  = 100_000
	BandHigh = 272_000
)

// IsLong reports whether a band is over a model's long-context limit.
func (t Table) IsLong(basis, model string, band int) bool {
	r, ok := t.rate(basis, model, time.Now())
	if !ok || r.LongOver == 0 {
		return false
	}
	limit := int64(BandHigh)
	if band == 1 {
		limit = BandMid
	}
	return band > 0 && limit >= r.LongOver
}

// Basis names the rates an account pays: an Azure account on its deployment's, others on its provider's.
func Basis(provider, kind string) string {
	switch {
	case kind == "azure":
		return "azure-global"
	case provider == "claude":
		return "anthropic"
	case provider == "codex":
		return "openai"
	}
	return ""
}
