package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/vizkid/burns/internal/tail"
)

// Cursor loads a file cursor and its decoder state.
func (s *Store) Cursor(path string) (tail.Cursor, json.RawMessage, bool, error) {
	c := tail.Cursor{Path: path}
	var mod int64
	var state string
	err := s.db.QueryRow(`SELECT inode, "offset", size, mod_time, head_hash, tail_hash, state FROM cursor WHERE path = ?`,
		path).Scan(&c.Inode, &c.Offset, &c.Size, &mod, &c.HeadHash, &c.TailHash, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil, false, nil
	}
	if err != nil {
		return c, nil, false, err
	}
	c.ModTime = fromNanos(mod)
	return c, json.RawMessage(state), true, nil
}

// SaveCursor stores a file cursor and its decoder state.
func (s *Store) SaveCursor(c tail.Cursor, state json.RawMessage) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO cursor VALUES (?,?,?,?,?,?,?,?)`, c.Path, c.Inode, c.Offset, c.Size,
		nanos(c.ModTime), c.HeadHash, c.TailHash, string(state))
	return err
}

// Sweep deletes rows older than Retention and cursors of files that no longer exist.
func (s *Store) Sweep(now time.Time) error {
	cut := nanos(now.Add(-Retention))
	for _, q := range []string{
		`DELETE FROM request WHERE at < ?`,
		`DELETE FROM compaction WHERE at < ?`,
		`DELETE FROM binding WHERE "end" IS NOT NULL AND "end" < ?`,
	} {
		if _, err := s.db.Exec(q, cut); err != nil {
			return err
		}
	}
	rows, err := s.db.Query(`SELECT path FROM cursor`)
	if err != nil {
		return err
	}
	var gone []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			gone = append(gone, p)
		}
	}
	rows.Close()
	for _, p := range gone {
		if _, err := s.db.Exec(`DELETE FROM cursor WHERE path = ?`, p); err != nil {
			return err
		}
	}
	return nil
}
