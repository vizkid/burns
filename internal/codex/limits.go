package codex

import (
	"time"

	"github.com/vizkid/burns/internal/quota"
	"github.com/vizkid/burns/internal/usage"
)

type rateWindow struct {
	UsedPercent   *float64 `json:"used_percent"`
	WindowMinutes int      `json:"window_minutes"`
	ResetsAt      int64    `json:"resets_at"`
}

type rateLimits struct {
	LimitID   string      `json:"limit_id"`
	Primary   *rateWindow `json:"primary"`
	Secondary *rateWindow `json:"secondary"`
	Credits   *Credits    `json:"credits"`
	PlanType  *string     `json:"plan_type"`
}

// limit classifies windows by window_minutes, never by position.
// A window without a length or a percentage is absent. Azure sends only nulls.
func (rl rateLimits) limit(at time.Time) Limit {
	l := Limit{LimitID: rl.LimitID, Credits: rl.Credits, At: at,
		Report: quota.Report{Provider: usage.Codex, At: at, Windows: quota.Windows{}}}
	if l.LimitID == "" {
		l.LimitID = "codex"
	}
	if rl.PlanType != nil {
		l.Plan = *rl.PlanType
	}
	for _, w := range []*rateWindow{rl.Primary, rl.Secondary} {
		if w == nil || w.UsedPercent == nil || w.WindowMinutes <= 0 || w.ResetsAt <= 0 {
			continue
		}
		pct := *w.UsedPercent
		l.Report.Windows[w.WindowMinutes] = quota.Window{Percent: &pct, ResetsAt: time.Unix(w.ResetsAt, 0), ObservedAt: at}
	}
	return l
}
