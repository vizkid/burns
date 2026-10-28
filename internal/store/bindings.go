package store

import (
	"database/sql"
	"strings"

	"github.com/vizkid/burns/internal/usage"
)

// Bind records that a session started in a block (docs/SPEC.md, "Ending a binding").
// It ends the block's open bindings to other sessions unless both share a known
// foreground pid: that is a nested agent, such as `claude -p` run by the outer
// agent's shell tool, and both stay open. A `clear` claim also ends the same
// process's previous session. A claim ends the session's open binding in
// another block. A repeat of an open binding (Claude fires SessionStart again
// on compact) only records a newly verified foreground.
func (s *Store) Bind(b usage.Binding) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	start := nanos(b.Start)
	if _, err := tx.Exec(`UPDATE binding SET "end" = ? WHERE "end" IS NULL AND host = ? AND block = ?
		AND NOT (provider = ? AND session = ?) AND NOT (? != 0 AND coalesce(fg_pid, 0) = ?)`,
		start, b.Host, b.Block, b.Provider, b.Session, b.FgPid, b.FgPid); err != nil {
		return err
	}
	if strings.HasSuffix(b.Source, ":clear") && b.FgPid != 0 {
		if _, err := tx.Exec(`UPDATE binding SET "end" = ? WHERE "end" IS NULL AND host = ? AND block = ?
			AND provider = ? AND session != ? AND fg_pid = ?`,
			start, b.Host, b.Block, b.Provider, b.Session, b.FgPid); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE binding SET "end" = ? WHERE "end" IS NULL AND provider = ? AND session = ?
		AND NOT (host = ? AND block = ?)`, start, b.Provider, b.Session, b.Host, b.Block); err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE binding SET verified = 1, fg_pid = ? WHERE "end" IS NULL AND host = ? AND block = ?
		AND provider = ? AND session = ? AND ?`, pidOrNull(b.FgPid), b.Host, b.Block, b.Provider, b.Session, b.Verified)
	if err != nil {
		return err
	}
	var open int
	if err := tx.QueryRow(`SELECT count(*) FROM binding WHERE "end" IS NULL AND host = ? AND block = ?
		AND provider = ? AND session = ?`, b.Host, b.Block, b.Provider, b.Session).Scan(&open); err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); open == 0 && n == 0 {
		if _, err := tx.Exec(`INSERT INTO binding (host, block, rex_session, provider, session, source, verified, fg_pid, start)
			VALUES (?,?,?,?,?,?,?,?,?)`, b.Host, b.Block, b.RexSession, b.Provider, b.Session, b.Source,
			boolInt(b.Verified), pidOrNull(b.FgPid), start); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func pidOrNull(pid int) any {
	if pid == 0 {
		return nil
	}
	return pid
}

// Bindings returns every binding on the host, oldest first.
func (s *Store) Bindings(host string) ([]usage.Binding, error) {
	rows, err := s.rdb.Query(`SELECT block, rex_session, provider, session, source, verified, fg_pid, start, "end"
		FROM binding WHERE host = ? ORDER BY start, id`, host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bs []usage.Binding
	for rows.Next() {
		b := usage.Binding{Host: host}
		var fg, end sql.NullInt64
		var start int64
		if err := rows.Scan(&b.Block, &b.RexSession, &b.Provider, &b.Session, &b.Source, &b.Verified, &fg, &start, &end); err != nil {
			return nil, err
		}
		b.FgPid, b.Start = int(fg.Int64), fromNanos(start)
		if end.Valid {
			t := fromNanos(end.Int64)
			b.End = &t
		}
		bs = append(bs, b)
	}
	return bs, rows.Err()
}
