// Package store keeps smithersd's readings in one SQLite file in WAL mode.
package store

import (
	"database/sql"
	"time"

	"github.com/vizkid/burns/internal/quota"
)

// Retention for request, compaction, and closed binding rows.
const Retention = 7 * 24 * time.Hour

// CompactionMatch is how close an OTel compaction and a transcript boundary
// of one session must be to count as the same compaction. The two sources
// share no key; on 2026-10-28 they were 64 ms apart.
const CompactionMatch = 5 * time.Second

// Store is the SQLite store.
type Store struct{ db *sql.DB }

// Credits is the latest credits reading of one Codex limit.
type Credits struct {
	HasCredits bool      `json:"has_credits"`
	Unlimited  bool      `json:"unlimited"`
	Balance    string    `json:"balance"`
	ObservedAt time.Time `json:"observed_at"`
}

// Limit is the stored state of one account's limit.
type Limit struct {
	Provider string        `json:"provider"`
	Account  string        `json:"account"`
	LimitID  string        `json:"limit_id"`
	Windows  quota.Windows `json:"windows"`
	Credits  *Credits      `json:"credits"`
}

// Totals sums tokens. CostMicros covers only requests with a known cost and
// is null when none has one (rule 9); UnknownCost counts the requests without one.
type Totals struct {
	Requests         int64  `json:"requests"`
	Input            int64  `json:"input"`
	CachedInput      int64  `json:"cached_input"`
	CacheWrite       int64  `json:"cache_write"`
	Output           int64  `json:"output"`
	CostMicros       *int64 `json:"cost_micros"`
	UnknownCost      int64  `json:"unknown_cost"`
	Compactions      int64  `json:"compactions"`
	CompactionTokens int64  `json:"compaction_tokens"`
}

// Group is the totals of one session, turn, or block.
type Group struct {
	Provider string    `json:"provider"`
	Account  string    `json:"account"`
	Session  string    `json:"session"`
	Turn     string    `json:"turn,omitempty"`
	Block    string    `json:"block,omitempty"`
	First    time.Time `json:"first"`
	Last     time.Time `json:"last"`
	Totals
}

// Bucket is one minute of one account's tokens.
type Bucket struct {
	Provider string    `json:"provider"`
	Account  string    `json:"account"`
	Minute   time.Time `json:"minute"`
	Tokens   int64     `json:"tokens"`
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }
