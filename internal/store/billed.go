package store

import (
	"database/sql"
	"errors"
	"time"
)

// Billed is what a provider says it charged. It is never an estimate. Amounts are in the
// currency's minor unit: 123 with exponent 2 is 1.23. A nil limit means none is set.
type Billed struct {
	Provider   string    `json:"provider"`
	Account    string    `json:"account"`
	Source     string    `json:"source"`
	Currency   string    `json:"currency"`
	UsedMinor  int64     `json:"used_minor"`
	Exponent   int       `json:"exponent"`
	LimitMinor *int64    `json:"limit_minor"`
	Enabled    bool      `json:"enabled"`
	Reason     string    `json:"reason,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
	// History is the amount each time it changed, oldest first, so an app can find what fell
	// inside a window: a provider reports a month's total.
	History []BilledPoint `json:"history,omitempty"`
}

// BilledPoint is an amount as it stood when it changed.
type BilledPoint struct {
	UsedMinor  int64     `json:"used_minor"`
	ObservedAt time.Time `json:"observed_at"`
}

// SetBilled stores an account's newest billed reading, and the amount when it changed.
func (s *Store) SetBilled(b Billed) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR REPLACE INTO billed VALUES (?,?,?,?,?,?,?,?,?,?)`, b.Provider, b.Account, b.Source,
		b.Currency, b.UsedMinor, b.Exponent, b.LimitMinor, boolInt(b.Enabled), b.Reason, nanos(b.ObservedAt)); err != nil {
		return err
	}
	var last int64
	err = tx.QueryRow(`SELECT used_minor FROM billed_point WHERE provider = ? AND account = ? ORDER BY observed_at DESC LIMIT 1`,
		b.Provider, b.Account).Scan(&last)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && last != b.UsedMinor) {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO billed_point VALUES (?,?,?,?)`, b.Provider, b.Account, b.UsedMinor, nanos(b.ObservedAt)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return tx.Commit()
}

// BilledHistory returns an account's amounts since a time, oldest first, after the last one
// before it: that one is what a window's spend is measured from.
func (s *Store) BilledHistory(provider, account string, since time.Time) ([]BilledPoint, error) {
	rows, err := s.rdb.Query(`SELECT used_minor, observed_at FROM billed_point WHERE provider = ?1 AND account = ?2 AND observed_at >= (
		SELECT coalesce(max(observed_at), 0) FROM billed_point WHERE provider = ?1 AND account = ?2 AND observed_at < ?3)
		ORDER BY observed_at`, provider, account, nanos(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BilledPoint
	for rows.Next() {
		var p BilledPoint
		var at int64
		if err := rows.Scan(&p.UsedMinor, &at); err != nil {
			return nil, err
		}
		p.ObservedAt = fromNanos(at)
		out = append(out, p)
	}
	return out, rows.Err()
}

// BilledAll returns every stored billed reading.
func (s *Store) BilledAll() ([]Billed, error) {
	rows, err := s.rdb.Query(`SELECT provider, account, source, currency, used_minor, exponent, limit_minor, enabled,
		reason, observed_at FROM billed ORDER BY provider, account`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Billed
	for rows.Next() {
		var b Billed
		var limit sql.NullInt64
		var enabled int
		var at int64
		if err := rows.Scan(&b.Provider, &b.Account, &b.Source, &b.Currency, &b.UsedMinor, &b.Exponent, &limit, &enabled,
			&b.Reason, &at); err != nil {
			return nil, err
		}
		if limit.Valid {
			b.LimitMinor = &limit.Int64
		}
		b.Enabled, b.ObservedAt = enabled == 1, fromNanos(at)
		out = append(out, b)
	}
	return out, rows.Err()
}
