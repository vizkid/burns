// Package codex decodes Codex rollout lines into usage records and quota reports.
package codex

import (
	"time"

	"github.com/vizkid/burns/internal/quota"
	"github.com/vizkid/burns/internal/usage"
)

// Credits is the credits block of a token_count event. Balance is a string on the wire.
type Credits struct {
	HasCredits bool   `json:"has_credits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance"`
}

// Limit is one rate_limits block, keyed by its limit_id.
type Limit struct {
	LimitID string
	Plan    string
	Report  quota.Report
	Credits *Credits
	At      time.Time
}

// Output is what one line produced.
type Output struct {
	Requests    []usage.Request
	Compactions []usage.Compaction
	Limits      []Limit
}

// Decoder holds the per-file state needed between lines.
// Its fields are exported so the tailer can persist it with the file cursor.
type Decoder struct {
	Account    string `json:"account"`
	Thread     string `json:"thread"`
	Session    string `json:"session"`
	Model      string `json:"model"`
	Records    bool   `json:"records"`
	LastTotal  int64  `json:"last_total"`
	TotalCount int    `json:"total_count"`
}
