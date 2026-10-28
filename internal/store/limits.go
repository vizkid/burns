package store

import (
	"time"

	"github.com/vizkid/burns/internal/quota"
)

// ApplyReport folds a quota report into the latest windows of one limit.
func (s *Store) ApplyReport(account, limitID string, r quota.Report, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stored := quota.Windows{}
	rows, err := tx.Query(`SELECT minutes, percent, resets_at, observed_at FROM latest
		WHERE provider = ? AND account = ? AND limit_id = ?`, r.Provider, account, limitID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m int
		var pct float64
		var reset, obs int64
		if err := rows.Scan(&m, &pct, &reset, &obs); err != nil {
			rows.Close()
			return err
		}
		stored[m] = quota.Window{Percent: &pct, ResetsAt: fromNanos(reset), ObservedAt: fromNanos(obs)}
	}
	rows.Close()
	if _, err := tx.Exec(`DELETE FROM latest WHERE provider = ? AND account = ? AND limit_id = ?`, r.Provider, account, limitID); err != nil {
		return err
	}
	for m, w := range quota.Apply(stored, r, now) {
		if _, err := tx.Exec(`INSERT INTO latest VALUES (?,?,?,?,?,?,?)`, r.Provider, account, limitID, m,
			*w.Percent, nanos(w.ResetsAt), nanos(w.ObservedAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetCredits stores the newest credits reading of one limit. An older reading is ignored.
func (s *Store) SetCredits(provider, account, limitID string, c Credits) error {
	_, err := s.db.Exec(`INSERT INTO credits VALUES (?,?,?,?,?,?,?)
		ON CONFLICT (provider, account, limit_id) DO UPDATE SET has_credits = excluded.has_credits,
		unlimited = excluded.unlimited, balance = excluded.balance, observed_at = excluded.observed_at
		WHERE excluded.observed_at >= credits.observed_at`,
		provider, account, limitID, boolInt(c.HasCredits), boolInt(c.Unlimited), c.Balance, nanos(c.ObservedAt))
	return err
}

// Limits returns every stored limit, ordered by provider, account, and limit.
func (s *Store) Limits() ([]Limit, error) {
	byKey := map[[3]string]*Limit{}
	var order [][3]string
	get := func(p, a, l string) *Limit {
		k := [3]string{p, a, l}
		if byKey[k] == nil {
			byKey[k] = &Limit{Provider: p, Account: a, LimitID: l, Windows: quota.Windows{}}
			order = append(order, k)
		}
		return byKey[k]
	}
	rows, err := s.db.Query(`SELECT provider, account, limit_id, minutes, percent, resets_at, observed_at FROM latest
		ORDER BY provider, account, limit_id, minutes`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p, a, l string
		var m int
		var pct float64
		var reset, obs int64
		if err := rows.Scan(&p, &a, &l, &m, &pct, &reset, &obs); err != nil {
			rows.Close()
			return nil, err
		}
		get(p, a, l).Windows[m] = quota.Window{Percent: &pct, ResetsAt: fromNanos(reset), ObservedAt: fromNanos(obs)}
	}
	rows.Close()
	rows, err = s.db.Query(`SELECT provider, account, limit_id, has_credits, unlimited, balance, observed_at FROM credits
		ORDER BY provider, account, limit_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p, a, l, bal string
		var has, unl int
		var obs int64
		if err := rows.Scan(&p, &a, &l, &has, &unl, &bal, &obs); err != nil {
			return nil, err
		}
		get(p, a, l).Credits = &Credits{HasCredits: has == 1, Unlimited: unl == 1, Balance: bal, ObservedAt: fromNanos(obs)}
	}
	out := make([]Limit, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out, rows.Err()
}
