package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Clock and zone for printed times; tests fix both.
var (
	now   = time.Now
	local = time.Local
)

const readTimeout = 5 * time.Second

type window struct {
	Percent    *float64  `json:"percent"`
	ResetsAt   time.Time `json:"resets_at"`
	ObservedAt time.Time `json:"observed_at"`
}

type account struct {
	Provider string `json:"provider"`
	Account  string `json:"account"`
	Limits   []struct {
		Windows map[string]window `json:"windows"`
	} `json:"limits"`
}

type total struct {
	Provider    string `json:"provider"`
	Account     string `json:"account"`
	Requests    int64  `json:"requests"`
	Input       int64  `json:"input"`
	CachedInput int64  `json:"cached_input"`
	CacheWrite  int64  `json:"cache_write"`
	Output      int64  `json:"output"`
	CostMicros  *int64 `json:"cost_micros"`
	UnknownCost int64  `json:"unknown_cost"`
	sessions    int
}

// printUsage prints each configured account's quota, then its consumption over --since.
func printUsage(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	fs.SetOutput(errOut)
	base := fs.String("url", DefaultURL, "smithersd address")
	since := fs.String("since", "24h", "consumption window")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if d, err := time.ParseDuration(*since); err != nil || d <= 0 {
		fmt.Fprintln(errOut, "--since must be a positive duration such as 24h")
		return 2
	}
	var u struct{ Accounts []account }
	var s struct{ Sessions []total }
	for _, r := range []struct {
		path string
		v    any
	}{{"/usage", &u}, {"/usage/sessions?since=" + url.QueryEscape(*since), &s}} {
		if err := getJSON(*base+r.path, r.v); err != nil {
			fmt.Fprintf(errOut, "smithersd at %s did not answer: %v\n", *base, err)
			return 1
		}
	}
	t := now()
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Quota at %s\n", t.In(local).Format("15:04"))
	for _, a := range u.Accounts {
		fmt.Fprintf(tw, "%s %s\t%s\n", a.Provider, a.Account, quota(a, t))
	}
	fmt.Fprintf(tw, "\nLast %s\tSessions\tRequests\tInput\tCached\tOutput\tCost\n", *since)
	totals := sum(s.Sessions)
	for _, a := range u.Accounts {
		name := a.Provider + " " + a.Account
		g, ok := totals[name]
		if !ok {
			fmt.Fprintf(tw, "%s\t\t\t\t\t\tnone seen\n", name) // empty cells keep the columns aligned
			continue
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\t%s\t%s\n", name, g.sessions, g.Requests,
			compact(g.Input), compact(g.CachedInput+g.CacheWrite), compact(g.Output), cost(g))
	}
	tw.Flush()
	return 0
}

func getJSON(u string, v any) error {
	client := &http.Client{Timeout: readTimeout}
	resp, err := client.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", u, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// quota lists the account's windows, shortest first, with the newest observation's age.
// No limits, or a window whose reset passed, prints unknown, never 0%.
func quota(a account, t time.Time) string {
	var parts []string
	var seen time.Time
	for _, l := range a.Limits {
		keys := make([]int, 0, len(l.Windows))
		for k := range l.Windows {
			if m, err := strconv.Atoi(k); err == nil {
				keys = append(keys, m)
			}
		}
		slices.Sort(keys)
		for _, m := range keys {
			w := l.Windows[strconv.Itoa(m)]
			if w.ObservedAt.After(seen) {
				seen = w.ObservedAt
			}
			if w.Percent == nil {
				parts = append(parts, span(m)+" unknown")
				continue
			}
			parts = append(parts, fmt.Sprintf("%s %.0f%% resets %s", span(m), *w.Percent, when(w.ResetsAt, t)))
		}
	}
	if len(parts) == 0 {
		return "unknown\t" // a closed cell keeps the next row's columns aligned
	}
	// The age comes first so it lines up across accounts with different windows.
	return "seen " + age(t.Sub(seen)) + " ago\t" + strings.Join(parts, "\t")
}

func span(minutes int) string {
	switch {
	case minutes%1440 == 0:
		return fmt.Sprintf("%dd", minutes/1440)
	case minutes%60 == 0:
		return fmt.Sprintf("%dh", minutes/60)
	}
	return fmt.Sprintf("%dm", minutes)
}

// when prints a reset as a time today, a weekday and time within a week, or a date.
func when(at, t time.Time) string {
	at, t = at.In(local), t.In(local)
	switch {
	case at.YearDay() == t.YearDay() && at.Year() == t.Year():
		return at.Format("15:04")
	case at.Sub(t) < 7*24*time.Hour:
		return at.Format("Mon 15:04")
	}
	return at.Format("Jan 2 15:04")
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func sum(sessions []total) map[string]*total {
	out := map[string]*total{}
	for _, s := range sessions {
		name := s.Provider + " " + s.Account
		g := out[name]
		if g == nil {
			g = &total{}
			out[name] = g
		}
		g.sessions++
		g.Requests += s.Requests
		g.Input += s.Input
		g.CachedInput += s.CachedInput
		g.CacheWrite += s.CacheWrite
		g.Output += s.Output
		g.UnknownCost += s.UnknownCost
		if s.CostMicros != nil {
			c := *s.CostMicros
			if g.CostMicros != nil {
				c += *g.CostMicros
			}
			g.CostMicros = &c
		}
	}
	return out
}

// cost prints known cost, and counts requests without one; unknown is never $0.
func cost(g *total) string {
	switch {
	case g.CostMicros == nil:
		return fmt.Sprintf("unknown (%d)", g.UnknownCost)
	case g.UnknownCost > 0:
		return fmt.Sprintf("$%.2f + %d unknown", float64(*g.CostMicros)/1e6, g.UnknownCost)
	}
	return fmt.Sprintf("$%.2f", float64(*g.CostMicros)/1e6)
}

func compact(n int64) string {
	switch {
	case n < 1000:
		return strconv.FormatInt(n, 10)
	case n < 1e6:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	case n < 1e9:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	}
	return fmt.Sprintf("%.1fB", float64(n)/1e9)
}
