package main

import (
	"cmp"
	"flag"
	"fmt"
	"io"
	"slices"
	"strconv"
	"text/tabwriter"
	"time"
)

type paneRow struct {
	Provider       string     `json:"provider"`
	Account        string     `json:"account"`
	Block          string     `json:"block"`
	Model          string     `json:"model"`
	MainTokens     int64      `json:"main_tokens"`
	Agents         int        `json:"agents"`
	Compactions    *int       `json:"compactions"`
	LastCompaction *time.Time `json:"last_compaction"`
	Session        string     `json:"session"`
	Label          string     `json:"label"`
}

// printPanes prints one line per live pane of one smithersd: its Rex session and
// label, account, the model its own thread ran most, subagents, and compactions
// since the pane took its session.
func printPanes(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("panes", flag.ContinueOnError)
	fs.SetOutput(errOut)
	base := fs.String("url", DefaultURL, "smithersd address")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var doc struct {
		Host    string
		Checked bool
		Panes   []paneRow
	}
	if err := getJSON(*base+"/usage/panes", &doc); err != nil {
		fmt.Fprintf(errOut, "smithersd at %s did not answer: %v\n", *base, err)
		return 1
	}
	t := now()
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Panes on %s at %s\n", doc.Host, t.In(local).Format("15:04"))
	if !doc.Checked {
		// Rule 9: an open binding can outlive its pane, so these are unconfirmed.
		fmt.Fprintln(tw, "Rex did not answer: these panes are unconfirmed and may be closed.")
	}
	fmt.Fprintln(tw, "Session\tPane\tAgent\tModel\tCompactions\tLast")
	for _, p := range mainRows(doc.Panes) {
		name, model, count, last := cmp.Or(p.Label, p.Block), p.Model, "unknown", "unknown"
		if p.Agents > 0 {
			model += fmt.Sprintf(" +%d", p.Agents)
		}
		// A smithersd older than the count sends none: unknown, never zero.
		if p.Compactions != nil {
			count, last = strconv.Itoa(*p.Compactions), "never"
		}
		if p.LastCompaction != nil {
			last = age(t.Sub(*p.LastCompaction)) + " ago"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s %s\t%s\t%s\t%s\n", p.Session, name, p.Provider, p.Account, model, count, last)
	}
	tw.Flush()
	return 0
}

// mainRows keeps each pane's row for the model with the most main-thread tokens,
// the model the pane runs, sorted by session, then labelled panes by label before
// unlabelled ones by block.
func mainRows(rows []paneRow) []paneRow {
	best := map[string]paneRow{}
	for _, r := range rows {
		key := r.Block + " " + r.Provider + " " + r.Account
		if b, ok := best[key]; !ok || r.MainTokens > b.MainTokens {
			best[key] = r
		}
	}
	out := make([]paneRow, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b paneRow) int {
		return cmp.Or(cmp.Compare(a.Session, b.Session), cmp.Compare(unlabelled(a), unlabelled(b)),
			cmp.Compare(a.Label, b.Label), cmp.Compare(a.Block, b.Block))
	})
	return out
}

func unlabelled(p paneRow) int {
	if p.Label == "" {
		return 1
	}
	return 0
}
