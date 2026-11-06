package store

import "github.com/vizkid/burns/internal/usage"

// SetContext keeps each session's newest context reading. A reading older than
// the stored one changes nothing, so a tailer catching up cannot roll it back.
func (s *Store) SetContext(c usage.Context) error {
	_, err := s.db.Exec(`INSERT INTO context (provider, session, used, size, at) VALUES (?,?,?,?,?)
		ON CONFLICT (provider, session) DO UPDATE SET used = excluded.used, size = excluded.size, at = excluded.at
		WHERE excluded.at >= context.at`, c.Provider, c.Session, c.Used, c.Size, nanos(c.At))
	return err
}

type paneContext struct{ used, size int64 }

// paneContexts reads each block's own session's context, the oldest open binding.
func (s *Store) paneContexts() (map[string]paneContext, error) {
	rows, err := s.rdb.Query(openBindings + `SELECT o.block, c.used, c.size FROM o JOIN context c
		ON c.provider = o.provider AND c.session = o.session WHERE o.rank = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]paneContext{}
	for rows.Next() {
		var block string
		var c paneContext
		if err := rows.Scan(&block, &c.used, &c.size); err != nil {
			return nil, err
		}
		out[block] = c
	}
	return out, rows.Err()
}
