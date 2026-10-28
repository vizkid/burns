package collect

import (
	"time"

	"github.com/vizkid/burns/internal/claude"
	"github.com/vizkid/burns/internal/codex"
	"github.com/vizkid/burns/internal/quota"
	"github.com/vizkid/burns/internal/store"
	"github.com/vizkid/burns/internal/usage"
)

type codexFile struct{ codex.Decoder }

func (f *codexFile) setAccount(a string) { f.Account = a }

// ingest folds a batch: every quota report of a limit in one transaction,
// only the newest credits per limit, then requests and compactions.
func (f *codexFile) ingest(st *store.Store, lines [][]byte, now time.Time) error {
	var reqs []usage.Request
	var comps []usage.Compaction
	reports := map[string][]quota.Report{}
	credits := map[string]codex.Limit{}
	var order []string
	for _, l := range lines {
		out := f.Line(l)
		reqs = append(reqs, out.Requests...)
		comps = append(comps, out.Compactions...)
		for _, lim := range out.Limits {
			if _, seen := reports[lim.LimitID]; !seen {
				order = append(order, lim.LimitID)
			}
			reports[lim.LimitID] = append(reports[lim.LimitID], lim.Report)
			if lim.Credits != nil {
				credits[lim.LimitID] = lim
			}
		}
	}
	for _, id := range order {
		if err := st.ApplyReports(f.Account, id, reports[id], now); err != nil {
			return err
		}
	}
	for id, lim := range credits {
		c := lim.Credits
		err := st.SetCredits(usage.Codex, f.Account, id, store.Credits{HasCredits: c.HasCredits,
			Unlimited: c.Unlimited, Balance: c.Balance, ObservedAt: lim.At})
		if err != nil {
			return err
		}
	}
	return addRecords(st, reqs, comps, now)
}

type claudeFile struct{ claude.Transcript }

func (f *claudeFile) setAccount(a string) { f.Account = a }

func (f *claudeFile) ingest(st *store.Store, lines [][]byte, now time.Time) error {
	reqs, comps := f.Lines(lines)
	return addRecords(st, reqs, comps, now)
}

// addRecords stores requests and compactions, dropping any older than
// retention: decoding an old file still advances decoder state, but its
// expired rows would only be swept again.
func addRecords(st *store.Store, reqs []usage.Request, comps []usage.Compaction, now time.Time) error {
	cut := now.Add(-store.Retention)
	keptReqs := reqs[:0]
	for _, r := range reqs {
		if !r.At.Before(cut) {
			keptReqs = append(keptReqs, r)
		}
	}
	keptComps := comps[:0]
	for _, c := range comps {
		if !c.At.Before(cut) {
			keptComps = append(keptComps, c)
		}
	}
	if err := st.AddRequests(keptReqs); err != nil {
		return err
	}
	return st.AddCompactions(keptComps)
}
