package store

import (
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// CreditBalance is a balance as it was when it changed.
type CreditBalance struct {
	Balance    float64   `json:"balance"`
	ObservedAt time.Time `json:"observed_at"`
}

// recordBalance adds a reading when it differs from the one before it in time. Rollouts arrive
// out of order across files, so the comparison is with the previous reading, not the newest.
func recordBalance(tx *sql.Tx, provider, account, limitID string, c Credits) error {
	balance, err := strconv.ParseFloat(c.Balance, 64)
	if err != nil {
		return nil // Empty or unreadable: no reading.
	}
	var prev float64
	err = tx.QueryRow(`SELECT balance FROM credit_balance WHERE provider = ? AND account = ? AND limit_id = ?
		AND observed_at < ? ORDER BY observed_at DESC LIMIT 1`, provider, account, limitID, nanos(c.ObservedAt)).Scan(&prev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && prev == balance {
		return nil
	}
	_, err = tx.Exec(`INSERT OR REPLACE INTO credit_balance VALUES (?,?,?,?,?)`, provider, account, limitID, balance, nanos(c.ObservedAt))
	return err
}

// CreditBalances returns one limit's balance changes since a time, oldest first.
func (s *Store) CreditBalances(provider, account, limitID string, since time.Time) ([]CreditBalance, error) {
	rows, err := s.rdb.Query(`SELECT balance, observed_at FROM credit_balance WHERE provider = ? AND account = ?
		AND limit_id = ? AND observed_at >= ? ORDER BY observed_at`, provider, account, limitID, nanos(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CreditBalance
	for rows.Next() {
		var b CreditBalance
		var at int64
		if err := rows.Scan(&b.Balance, &at); err != nil {
			return nil, err
		}
		b.ObservedAt = fromNanos(at)
		out = append(out, b)
	}
	return out, rows.Err()
}
