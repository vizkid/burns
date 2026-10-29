package store

import (
	"database/sql"
	"errors"

	"github.com/vizkid/burns/internal/usage"
)

// AddCompactions stores compactions and flags their requests.
//
// A compaction with a key (Codex) is stored once per key. One without a key
// (Claude) matches a stored compaction of the same session within
// CompactionMatch; the OTel copy replaces a transcript copy, never the reverse.
func (s *Store) AddCompactions(cs []usage.Compaction) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	attr := newAttribution(tx)
	for _, c := range cs {
		if err := attr.fill(c.Provider, c.Session, &c.Account); err != nil {
			return err
		}
		id, source, err := matchCompaction(tx, c)
		if err != nil {
			return err
		}
		switch {
		case id == 0:
			_, err = tx.Exec(`INSERT INTO compaction (provider, key, account, session, turn, trigger, pre_tokens,
				post_tokens, duration_ms, request_key, source, at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				c.Provider, c.Key, c.Account, c.Session, c.Turn, c.Trigger, c.PreTokens, c.PostTokens,
				c.DurationMs, c.RequestKey, c.Source, nanos(c.At))
		case c.Source == usage.SourceOTel && source != usage.SourceOTel:
			_, err = tx.Exec(`UPDATE compaction SET account = coalesce(nullif(?, ''), account), turn = coalesce(nullif(?, ''), turn),
				trigger = ?, pre_tokens = ?, post_tokens = ?, duration_ms = ?, source = ?, at = ? WHERE id = ?`,
				c.Account, c.Turn, c.Trigger, c.PreTokens, c.PostTokens, c.DurationMs, c.Source, nanos(c.At), id)
		}
		if err != nil {
			return err
		}
		if c.RequestKey != "" {
			if _, err := tx.Exec(`UPDATE request SET compaction = 1 WHERE provider = ? AND key = ? AND compaction = 0`, c.Provider, c.RequestKey); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// keyedCompactionQuery repeats the partial index's condition so SQLite can use it.
const keyedCompactionQuery = `SELECT id, source FROM compaction WHERE provider = ? AND key = ? AND key != ''`

func matchCompaction(tx *sql.Tx, c usage.Compaction) (id int64, source string, err error) {
	if c.Key != "" {
		err = tx.QueryRow(keyedCompactionQuery, c.Provider, c.Key).Scan(&id, &source)
	} else {
		at, window := nanos(c.At), int64(CompactionMatch)
		err = tx.QueryRow(`SELECT id, source FROM compaction WHERE provider = ? AND session = ? AND key = ''
			AND at BETWEEN ? AND ? ORDER BY abs(at - ?) LIMIT 1`, c.Provider, c.Session, at-window, at+window, at).Scan(&id, &source)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	return id, source, err
}
