package collect

import (
	"time"

	"github.com/vizkid/burns/internal/claude"
	"github.com/vizkid/burns/internal/codex"
	"github.com/vizkid/burns/internal/store"
	"github.com/vizkid/burns/internal/usage"
)

type codexFile struct{ codex.Decoder }

func (f *codexFile) setAccount(a string) { f.Account = a }

func (f *codexFile) ingest(st *store.Store, lines [][]byte, now time.Time) error {
	var reqs []usage.Request
	var comps []usage.Compaction
	for _, l := range lines {
		out := f.Line(l)
		reqs = append(reqs, out.Requests...)
		comps = append(comps, out.Compactions...)
		for _, lim := range out.Limits {
			if err := st.ApplyReport(f.Account, lim.LimitID, lim.Report, now); err != nil {
				return err
			}
			if c := lim.Credits; c != nil {
				err := st.SetCredits(usage.Codex, f.Account, lim.LimitID, store.Credits{HasCredits: c.HasCredits,
					Unlimited: c.Unlimited, Balance: c.Balance, ObservedAt: lim.At})
				if err != nil {
					return err
				}
			}
		}
	}
	if err := st.AddRequests(reqs); err != nil {
		return err
	}
	return st.AddCompactions(comps)
}

type claudeFile struct{ claude.Transcript }

func (f *claudeFile) setAccount(a string) { f.Account = a }

func (f *claudeFile) ingest(st *store.Store, lines [][]byte, _ time.Time) error {
	reqs, comps := f.Lines(lines)
	if err := st.AddRequests(reqs); err != nil {
		return err
	}
	return st.AddCompactions(comps)
}
