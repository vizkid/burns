package store

import (
	"strings"
	"time"
)

// attributed adds each request's block: its own (from OTel), else the binding
// that covered its session at its time, else empty (unattributed).
const attributed = `WITH r AS (
	SELECT request.*, input + cached_input + cache_write + output AS total,
	coalesce(nullif(block, ''), (SELECT b.block FROM binding b WHERE b.provider = request.provider
		AND b.session = request.session AND b.start <= request.at AND (b."end" IS NULL OR request.at < b."end")
		ORDER BY b.start DESC LIMIT 1), '') AS blk
	FROM request WHERE at >= ?)`

const sums = `count(*), sum(input), sum(cached_input), sum(cache_write), sum(output),
	sum(cost_micros), sum(cost_micros IS NULL),
	(SELECT count(*) FROM compaction c WHERE c.provider = r.provider AND c.session = r.session AND c.at >= ?%s),
	sum(CASE WHEN compaction = 1 THEN total ELSE 0 END), min(at), max(at)`

// Sessions returns per-session totals for sessions active since the time.
func (s *Store) Sessions(since time.Time) ([]Group, error) {
	q := attributed + ` SELECT provider, max(account), session, '', '', ` + fmtSums("") +
		` FROM r GROUP BY provider, session ORDER BY max(at) DESC`
	return s.groups(q, nanos(since), nanos(since))
}

// Turns returns per-turn totals of one session, oldest first.
func (s *Store) Turns(provider, session string) ([]Group, error) {
	q := attributed + ` SELECT provider, max(account), session, turn, '', ` + fmtSums(" AND c.turn = r.turn") +
		` FROM r WHERE provider = ? AND session = ? GROUP BY turn ORDER BY min(at)`
	return s.groups(q, 0, 0, provider, session)
}

// Blocks returns totals per block and session since the time. Requests with no
// block are grouped under an empty block as unattributed.
func (s *Store) Blocks(since time.Time) ([]Group, error) {
	q := attributed + ` SELECT provider, max(account), session, '', blk, ` + fmtSums("") +
		` FROM r GROUP BY blk, provider, session ORDER BY blk, max(at) DESC`
	return s.groups(q, nanos(since), nanos(since))
}

// fmtSums fills the compaction filter into sums.
func fmtSums(extra string) string { return strings.Replace(sums, "%s", extra, 1) }

func (s *Store) groups(q string, args ...any) ([]Group, error) {
	rows, err := s.rdb.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		var first, last int64
		if err := rows.Scan(&g.Provider, &g.Account, &g.Session, &g.Turn, &g.Block, &g.Requests, &g.Input,
			&g.CachedInput, &g.CacheWrite, &g.Output, &g.CostMicros, &g.UnknownCost, &g.Compactions,
			&g.CompactionTokens, &first, &last); err != nil {
			return nil, err
		}
		g.First, g.Last = fromNanos(first), fromNanos(last)
		out = append(out, g)
	}
	return out, rows.Err()
}

// History returns per-account minute buckets in [from, to), oldest first.
func (s *Store) History(from, to time.Time) ([]Bucket, error) {
	minute := int64(time.Minute)
	rows, err := s.rdb.Query(`SELECT provider, account, at / ? AS m, sum(input + cached_input + cache_write + output)
		FROM request WHERE at >= ? AND at < ? GROUP BY provider, account, m ORDER BY m, provider, account`,
		minute, nanos(from), nanos(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bucket
	for rows.Next() {
		var b Bucket
		var m int64
		if err := rows.Scan(&b.Provider, &b.Account, &m, &b.Tokens); err != nil {
			return nil, err
		}
		b.Minute = fromNanos(m * minute)
		out = append(out, b)
	}
	return out, rows.Err()
}
