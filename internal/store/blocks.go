package store

import (
	"database/sql"
	"time"
)

// BlockSession is one open binding of a block. Account is the one its newest
// request names, or "" before the session has made a request.
type BlockSession struct {
	Provider string    `json:"provider"`
	Session  string    `json:"session"`
	Account  string    `json:"account"`
	Start    time.Time `json:"start"`
}

// BlockSessions returns the block's open bindings, oldest first.
func (s *Store) BlockSessions(host, block string) ([]BlockSession, error) {
	rows, err := s.rdb.Query(`SELECT b.provider, b.session, b.start, coalesce((SELECT r.account FROM request r
		WHERE r.provider = b.provider AND r.session = b.session ORDER BY r.at DESC LIMIT 1), '')
		FROM binding b WHERE b."end" IS NULL AND b.host = ? AND b.block = ? ORDER BY b.start, b.id`, host, block)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlockSession
	for rows.Next() {
		var b BlockSession
		var start int64
		if err := rows.Scan(&b.Provider, &b.Session, &start, &b.Account); err != nil {
			return nil, err
		}
		b.Start = fromNanos(start)
		out = append(out, b)
	}
	return out, rows.Err()
}

// SessionLast returns the time of the session's last request, or nil with none.
func (s *Store) SessionLast(provider, session string) (*time.Time, error) {
	var at sql.NullInt64
	if err := s.rdb.QueryRow(`SELECT max(at) FROM request WHERE provider = ? AND session = ?`,
		provider, session).Scan(&at); err != nil {
		return nil, err
	}
	if !at.Valid {
		return nil, nil
	}
	t := fromNanos(at.Int64)
	return &t, nil
}
