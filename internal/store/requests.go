package store

import (
	"database/sql"
	"errors"

	"github.com/vizkid/burns/internal/usage"
)

const requestCols = `provider, key, account, session, agent, turn, model, input, cached_input, cache_write,
	output, reasoning, cost_micros, compaction, block, rex_session, source, at`

// AddRequests stores requests. A key already stored keeps its tokens unless the
// new row comes from OTel and the stored one does not, or the new row has a
// larger total. Cost, account, block, Rex session, and the compaction flag are
// filled from whichever copy has them.
func (s *Store) AddRequests(rs []usage.Request) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range rs {
		old, found, err := loadRequest(tx, r.Provider, r.Key)
		if err != nil {
			return err
		}
		if found {
			r = mergeRequest(old, r)
		}
		if err := flagged(tx, &r); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO request (`+requestCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			r.Provider, r.Key, r.Account, r.Session, r.Agent, r.Turn, r.Model, r.Input, r.CachedInput, r.CacheWrite,
			r.Output, r.Reasoning, r.CostMicros, boolInt(r.Compaction), r.Block, r.RexSession, r.Source, nanos(r.At)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func mergeRequest(old, next usage.Request) usage.Request {
	keep, other := old, next
	nextOTel, oldOTel := next.Source == usage.SourceOTel, old.Source == usage.SourceOTel
	if (nextOTel && !oldOTel) || (nextOTel == oldOTel && next.Total() > old.Total()) {
		keep, other = next, old
	}
	if keep.CostMicros == nil {
		keep.CostMicros = other.CostMicros
	}
	keep.Compaction = keep.Compaction || other.Compaction
	for _, f := range []struct {
		dst *string
		src string
	}{
		{&keep.Account, other.Account}, {&keep.Block, other.Block}, {&keep.RexSession, other.RexSession},
		{&keep.Turn, other.Turn}, {&keep.Agent, other.Agent}, {&keep.Model, other.Model},
	} {
		if *f.dst == "" {
			*f.dst = f.src
		}
	}
	return keep
}

// flagged marks a request as compaction work when a stored compaction names it.
func flagged(tx *sql.Tx, r *usage.Request) error {
	if r.Compaction {
		return nil
	}
	var n int
	err := tx.QueryRow(`SELECT count(*) FROM compaction WHERE provider = ? AND request_key = ?`, r.Provider, r.Key).Scan(&n)
	r.Compaction = n > 0
	return err
}

func loadRequest(tx *sql.Tx, provider, key string) (usage.Request, bool, error) {
	var r usage.Request
	var comp int
	var at int64
	var cost sql.NullInt64
	err := tx.QueryRow(`SELECT `+requestCols+` FROM request WHERE provider = ? AND key = ?`, provider, key).Scan(
		&r.Provider, &r.Key, &r.Account, &r.Session, &r.Agent, &r.Turn, &r.Model, &r.Input, &r.CachedInput,
		&r.CacheWrite, &r.Output, &r.Reasoning, &cost, &comp, &r.Block, &r.RexSession, &r.Source, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if cost.Valid {
		r.CostMicros = &cost.Int64
	}
	r.Compaction, r.At = comp == 1, fromNanos(at)
	return r, err == nil, err
}
