package store

import (
	"slices"
	"time"
)

// EndBlock ends every open binding in a closed block.
func (s *Store) EndBlock(host, block string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE binding SET "end" = ? WHERE "end" IS NULL AND host = ? AND block = ?`,
		nanos(at), host, block)
	return err
}

// EndRexSession ends every open binding in a destroyed Rex session.
func (s *Store) EndRexSession(host, rexSession string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE binding SET "end" = ? WHERE "end" IS NULL AND host = ? AND rex_session = ?`,
		nanos(at), host, rexSession)
	return err
}

// ForegroundChanged applies a block's new foreground process. With none
// (pid 0), the block's program is gone and every open binding ends. Otherwise
// a binding ends when its agent is neither the foreground nor one of its
// ancestors; an editor the agent opened keeps the binding. Bindings with an
// unknown foreground are left for the next claim to settle.
func (s *Store) ForegroundChanged(host, block string, pid int, ancestors []int, at time.Time) error {
	if pid == 0 {
		return s.EndBlock(host, block, at)
	}
	rows, err := s.db.Query(`SELECT id, fg_pid FROM binding WHERE "end" IS NULL AND host = ? AND block = ?
		AND fg_pid IS NOT NULL`, host, block)
	if err != nil {
		return err
	}
	var stale []int64
	for rows.Next() {
		var id int64
		var fg int
		if err := rows.Scan(&id, &fg); err != nil {
			rows.Close()
			return err
		}
		if fg != pid && !slices.Contains(ancestors, fg) {
			stale = append(stale, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range stale {
		if _, err := s.db.Exec(`UPDATE binding SET "end" = ? WHERE id = ?`, nanos(at), id); err != nil {
			return err
		}
	}
	return nil
}

// OpenBlocks lists the host's blocks that have an open binding, sorted.
func (s *Store) OpenBlocks(host string) ([]string, error) {
	rows, err := s.rdb.Query(`SELECT DISTINCT block FROM binding WHERE "end" IS NULL AND host = ? ORDER BY block`, host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var blocks []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, rows.Err()
}
