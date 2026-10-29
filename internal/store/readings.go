package store

import (
	"database/sql"
	"time"

	"github.com/vizkid/burns/internal/quota"
)

// resetJitter is how far a reported reset may move within one window. Larger
// moves start a new window.
const resetJitter = time.Minute

// Reading is one change of a quota window (docs/SPEC.md, "Reading history").
type Reading struct {
	Provider   string    `json:"provider"`
	Account    string    `json:"account"`
	LimitID    string    `json:"limit_id"`
	Minutes    int       `json:"minutes"`
	Percent    float64   `json:"percent"`
	ResetsAt   time.Time `json:"resets_at"`
	ObservedAt time.Time `json:"observed_at"`
}

// recordReadings stores each live window in reports, in order, that changed
// since the limit's last reading of it. Older reports add nothing, so the line
// only moves forward. It reports whether it wrote a row.
func recordReadings(tx *sql.Tx, provider, account, limitID string, reports []quota.Report, now time.Time) (bool, error) {
	last, err := lastReadings(tx, provider, account, limitID)
	if err != nil {
		return false, err
	}
	wrote := false
	for _, r := range reports {
		for m, w := range r.Windows {
			if !w.Live(now) {
				continue
			}
			if prev, ok := last[m]; ok && (!w.ObservedAt.After(prev.ObservedAt) || sameReading(prev, w)) {
				continue
			}
			// A new window supersedes the old one even before the old reset
			// passes: Codex can restart a week early.
			if _, err := tx.Exec(`DELETE FROM reading WHERE provider = ? AND account = ? AND limit_id = ? AND minutes = ?
				AND abs(resets_at - ?) > ?`, provider, account, limitID, m, nanos(w.ResetsAt), int64(resetJitter)); err != nil {
				return false, err
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO reading VALUES (?,?,?,?,?,?,?)`, provider, account, limitID, m,
				*w.Percent, nanos(w.ResetsAt), nanos(w.ObservedAt)); err != nil {
				return false, err
			}
			last[m], wrote = w, true
		}
	}
	return wrote, nil
}

func sameReading(a, b quota.Window) bool {
	d := a.ResetsAt.Sub(b.ResetsAt)
	return *a.Percent == *b.Percent && d <= resetJitter && d >= -resetJitter
}

// lastReadings returns the newest reading of each window of one limit.
func lastReadings(tx *sql.Tx, provider, account, limitID string) (quota.Windows, error) {
	// SQLite takes the bare columns from the row that holds max(observed_at).
	rows, err := tx.Query(`SELECT minutes, percent, resets_at, max(observed_at) FROM reading
		WHERE provider = ? AND account = ? AND limit_id = ? GROUP BY minutes`, provider, account, limitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := quota.Windows{}
	for rows.Next() {
		var m int
		var pct float64
		var reset, obs int64
		if err := rows.Scan(&m, &pct, &reset, &obs); err != nil {
			return nil, err
		}
		out[m] = quota.Window{Percent: &pct, ResetsAt: fromNanos(reset), ObservedAt: fromNanos(obs)}
	}
	return out, rows.Err()
}

// Readings returns every reading of the windows that have not reset, oldest first.
func (s *Store) Readings(now time.Time) ([]Reading, error) {
	rows, err := s.rdb.Query(`SELECT provider, account, limit_id, minutes, percent, resets_at, observed_at
		FROM reading WHERE resets_at > ? ORDER BY observed_at, provider, account, limit_id, minutes`, nanos(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reading
	for rows.Next() {
		var r Reading
		var reset, obs int64
		if err := rows.Scan(&r.Provider, &r.Account, &r.LimitID, &r.Minutes, &r.Percent, &reset, &obs); err != nil {
			return nil, err
		}
		r.ResetsAt, r.ObservedAt = fromNanos(reset), fromNanos(obs)
		out = append(out, r)
	}
	return out, rows.Err()
}
