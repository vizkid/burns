// Package usage holds the records smithersd stores and serves.
// Token fields are normalized across providers so totals add up the same way.
package usage

import "time"

// Version is stamped at build time.
var Version = "dev"

// Providers.
const (
	Claude = "claude"
	Codex  = "codex"
)

// Sources of a record.
const (
	SourceOTel       = "otel"
	SourceTranscript = "transcript"
	SourceRollout    = "rollout"
	SourceStatusline = "statusline"
	SourceHook       = "hook"
)

// Request is one Claude API request or one Codex response.
//
// Input is uncached input only. Claude reports it that way; Codex includes
// cached tokens in input_tokens, so the Codex decoder subtracts them.
// Output includes reasoning tokens for both providers.
type Request struct {
	Provider    string    `json:"provider"`
	Key         string    `json:"key"`
	Account     string    `json:"account"`
	Session     string    `json:"session"`
	Agent       string    `json:"agent,omitempty"`
	Turn        string    `json:"turn,omitempty"`
	Model       string    `json:"model,omitempty"`
	Input       int64     `json:"input"`
	CachedInput int64     `json:"cached_input"`
	CacheWrite  int64     `json:"cache_write"`
	Output      int64     `json:"output"`
	Reasoning   int64     `json:"reasoning"`
	CostMicros  *int64    `json:"cost_micros"`
	Compaction  bool      `json:"compaction"`
	Block       string    `json:"block,omitempty"`
	RexSession  string    `json:"rex_session,omitempty"`
	Source      string    `json:"source"`
	At          time.Time `json:"at"`
}

// Total is every token the request consumed.
func (r Request) Total() int64 { return r.Input + r.CachedInput + r.CacheWrite + r.Output }

// Compaction is one context compaction. Nil fields were not reported.
type Compaction struct {
	Provider   string    `json:"provider"`
	Key        string    `json:"key,omitempty"`
	Account    string    `json:"account"`
	Session    string    `json:"session"`
	Turn       string    `json:"turn,omitempty"`
	Trigger    string    `json:"trigger"`
	PreTokens  *int64    `json:"pre_tokens"`
	PostTokens *int64    `json:"post_tokens"`
	DurationMs *int64    `json:"duration_ms"`
	RequestKey string    `json:"request_key,omitempty"`
	Source     string    `json:"source"`
	At         time.Time `json:"at"`
}

// Binding says an agent session ran in a Rex block from Start until End.
type Binding struct {
	Host       string     `json:"host"`
	Block      string     `json:"block"`
	RexSession string     `json:"rex_session"`
	Provider   string     `json:"provider"`
	Session    string     `json:"session"`
	Source     string     `json:"source"`
	Start      time.Time  `json:"start"`
	End        *time.Time `json:"end"`
}
