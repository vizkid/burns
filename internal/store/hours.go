package store

import (
	"time"
)

// HourRetention is how long hourly totals are kept: over a year, so a month can be compared with the last.
const HourRetention = 400 * 24 * time.Hour

// Hour is the tokens of one hour's requests that share a model, speed and prompt band.
// Dollars are worked out on read, so correcting a price corrects history.
type Hour struct {
	Hour              time.Time
	Provider, Account string
	Model, Speed      string
	Band              int
	Requests          int64
	Input, Cached     int64
	Write5m, Write1h  int64
	Output            int64
}

// hourNanos is an hour in the nanoseconds that rows store times in.
const hourNanos = int64(time.Hour)

// RollUp rebuilds the hourly totals from `from` on. A row can change after it is first stored, as
// when a transcript copy of a request arrives, so hours are rebuilt, not added to. Hours older
// than Retention are left alone, because their request rows are gone.
func (s *Store) RollUp(from, now time.Time) error {
	floor := now.Add(-Retention).UnixNano()/hourNanos*hourNanos + hourNanos
	start := from.UnixNano() / hourNanos * hourNanos
	if start < floor {
		start = floor
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM usage_hour WHERE hour >= ?`, start); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO usage_hour
		SELECT at / ?1 * ?1, provider, account, model, speed, band, count(*), sum(input), sum(cached_input),
			sum(cache_write - min(cache_write_1h, cache_write)), sum(min(cache_write_1h, cache_write)), sum(output)
		FROM (SELECT *, CASE WHEN input + cached_input + cache_write > 272000 THEN 2
			WHEN input + cached_input + cache_write > 100000 THEN 1 ELSE 0 END AS band
			FROM request WHERE at >= ?2)
		GROUP BY 1, 2, 3, 4, 5, 6`, hourNanos, start)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM usage_hour WHERE hour < ?`, now.Add(-HourRetention).UnixNano()); err != nil {
		return err
	}
	return tx.Commit()
}

// Hours returns the hourly totals since a time, oldest first.
func (s *Store) Hours(since time.Time) ([]Hour, error) {
	rows, err := s.rdb.Query(`SELECT hour, provider, account, model, speed, band, requests, input, cached_input,
		write_5m, write_1h, output FROM usage_hour WHERE hour >= ? ORDER BY hour, provider, account, model`, nanos(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Hour
	for rows.Next() {
		var h Hour
		var at int64
		if err := rows.Scan(&at, &h.Provider, &h.Account, &h.Model, &h.Speed, &h.Band, &h.Requests, &h.Input, &h.Cached,
			&h.Write5m, &h.Write1h, &h.Output); err != nil {
			return nil, err
		}
		h.Hour = fromNanos(at)
		out = append(out, h)
	}
	return out, rows.Err()
}
