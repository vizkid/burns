package store

import "github.com/vizkid/burns/internal/usage"

// Bind records that a session started in a block. It ends the block's open
// binding to another session, and the session's open binding in another block.
// A repeat of an open binding (Claude fires SessionStart again on compact) is a no-op.
func (s *Store) Bind(b usage.Binding) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	start := nanos(b.Start)
	if _, err := tx.Exec(`UPDATE binding SET "end" = ? WHERE "end" IS NULL AND host = ? AND block = ?
		AND NOT (provider = ? AND session = ?)`, start, b.Host, b.Block, b.Provider, b.Session); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE binding SET "end" = ? WHERE "end" IS NULL AND provider = ? AND session = ?
		AND NOT (host = ? AND block = ?)`, start, b.Provider, b.Session, b.Host, b.Block); err != nil {
		return err
	}
	var open int
	if err := tx.QueryRow(`SELECT count(*) FROM binding WHERE "end" IS NULL AND host = ? AND block = ?
		AND provider = ? AND session = ?`, b.Host, b.Block, b.Provider, b.Session).Scan(&open); err != nil {
		return err
	}
	if open == 0 {
		if _, err := tx.Exec(`INSERT INTO binding (host, block, rex_session, provider, session, source, start)
			VALUES (?,?,?,?,?,?,?)`, b.Host, b.Block, b.RexSession, b.Provider, b.Session, b.Source, start); err != nil {
			return err
		}
	}
	return tx.Commit()
}
