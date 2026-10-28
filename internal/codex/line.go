package codex

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/vizkid/burns/internal/usage"
)

type line struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type tokens struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
	Reasoning  int64 `json:"reasoning_output_tokens"`
	Total      int64 `json:"total_tokens"`
}

func (t tokens) sum() int64 {
	if t.Total > 0 {
		return t.Total
	}
	return t.Input + t.Output
}

// Line decodes one complete rollout line. Malformed lines produce nothing.
func (d *Decoder) Line(b []byte) Output {
	var l line
	if json.Unmarshal(b, &l) != nil {
		return Output{}
	}
	switch l.Type {
	case "session_meta":
		d.sessionMeta(l.Payload)
	case "turn_context":
		var p struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(l.Payload, &p) == nil && p.Model != "" {
			d.Model = p.Model
		}
	case "token_usage_record":
		return d.record(l)
	case "compacted":
		return d.compacted(l)
	case "event_msg":
		return d.event(l)
	}
	return Output{}
}

// sessionMeta keeps the first session_meta only. A subagent rollout starts
// with its own, then a copy of its parent's.
func (d *Decoder) sessionMeta(raw json.RawMessage) {
	var p struct {
		ID         string `json:"id"`
		SessionID  string `json:"session_id"`
		CLIVersion string `json:"cli_version"`
	}
	if d.Thread != "" || json.Unmarshal(raw, &p) != nil {
		return
	}
	d.Thread, d.Session = p.ID, p.SessionID
	d.Records = d.Records || atLeast(p.CLIVersion, recordsSince)
	if d.Session == "" {
		d.Session = p.ID
	}
}

func (d *Decoder) record(l line) Output {
	var p struct {
		ThreadID   string `json:"thread_id"`
		SessionID  string `json:"session_id"`
		TurnID     string `json:"turn_id"`
		ResponseID string `json:"response_id"`
		Usage      tokens `json:"usage"`
	}
	if json.Unmarshal(l.Payload, &p) != nil || p.ThreadID == "" || p.ResponseID == "" {
		return Output{}
	}
	d.Records = true
	session := p.SessionID
	if session == "" {
		session = p.ThreadID
	}
	r := d.request(p.ThreadID+"/"+p.ResponseID, session, p.TurnID, p.Usage, l.Timestamp)
	if p.ThreadID != session {
		r.Agent = p.ThreadID
	}
	return Output{Requests: []usage.Request{r}}
}

// request normalizes Codex tokens: input_tokens includes cached and cache-write
// tokens (total_tokens = input + output), so they are subtracted from Input.
func (d *Decoder) request(key, session, turn string, t tokens, at time.Time) usage.Request {
	input := t.Input - t.Cached - t.CacheWrite
	if input < 0 {
		input = 0
	}
	return usage.Request{Provider: usage.Codex, Key: key, Account: d.Account, Session: session, Turn: turn,
		Model: d.Model, Input: input, CachedInput: t.Cached, CacheWrite: t.CacheWrite, Output: t.Output,
		Reasoning: t.Reasoning, Source: usage.SourceRollout, At: at}
}

func (d *Decoder) compacted(l line) Output {
	var p struct {
		ResponseID string `json:"compaction_response_id"`
	}
	if json.Unmarshal(l.Payload, &p) != nil || p.ResponseID == "" || d.Thread == "" {
		return Output{}
	}
	key := d.Thread + "/" + p.ResponseID
	return Output{Compactions: []usage.Compaction{{Provider: usage.Codex, Key: key, RequestKey: key,
		Account: d.Account, Session: d.Session, Source: usage.SourceRollout, At: l.Timestamp}}}
}

func (d *Decoder) event(l line) Output {
	var p struct {
		Type string `json:"type"`
		Info *struct {
			Total tokens `json:"total_token_usage"`
			Last  tokens `json:"last_token_usage"`
		} `json:"info"`
		RateLimits *rateLimits `json:"rate_limits"`
	}
	if json.Unmarshal(l.Payload, &p) != nil || p.Type != "token_count" {
		return Output{}
	}
	var out Output
	if p.RateLimits != nil {
		out.Limits = []Limit{p.RateLimits.limit(l.Timestamp)}
	}
	if p.Info != nil && !d.Records {
		out.Requests = d.fallback(p.Info.Total, p.Info.Last, l.Timestamp)
	}
	return out
}

// recordsSince is the first Codex CLI version that writes token_usage_record.
var recordsSince = [3]int{0, 153, 4}

// atLeast compares a dotted version such as "0.165.0" with min.
// An unparseable version is treated as older.
func atLeast(v string, min [3]int) bool {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(strings.SplitN(p, "-", 2)[0])
		if err != nil {
			return false
		}
		if n != min[i] {
			return n > min[i]
		}
	}
	return true
}

// fallback derives usage from token_count totals, for rollouts written before
// token_usage_record existed (Codex CLI 0.153.4). A repeated total is skipped.
// A total that drops is a reset: last_token_usage is the new usage.
func (d *Decoder) fallback(total, last tokens, at time.Time) []usage.Request {
	t := total.sum()
	var delta tokens
	switch {
	case t == d.LastTotal:
		return nil
	case t > d.LastTotal:
		delta = tokens{Input: t - d.LastTotal}
		if t-d.LastTotal == last.sum() {
			delta = last
		}
	default:
		delta = last
	}
	d.LastTotal = t
	d.TotalCount++
	key := d.Thread + "/total/" + strconv.Itoa(d.TotalCount)
	return []usage.Request{d.request(key, d.Session, "", delta, at)}
}
