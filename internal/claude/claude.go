// Package claude turns Claude Code telemetry, statusline posts, and transcripts into usage records.
package claude

import (
	"github.com/vizkid/burns/internal/otlp"
	"github.com/vizkid/burns/internal/usage"
)

// compactSource is the query_source of the request that writes a compaction summary.
const compactSource = "compact"

// FromEvents converts api_request and compaction log events. Other events are ignored.
// Events without a session, or an api_request without a request_id, are dropped:
// they cannot be deduplicated against the transcript.
func FromEvents(evs []otlp.Event) ([]usage.Request, []usage.Compaction) {
	var reqs []usage.Request
	var comps []usage.Compaction
	for _, e := range evs {
		session, _ := e.String("session.id")
		if session == "" {
			continue
		}
		switch e.Name {
		case "api_request":
			if r, ok := request(e, session); ok {
				reqs = append(reqs, r)
			}
		case "compaction":
			comps = append(comps, compaction(e, session))
		}
	}
	return reqs, comps
}

func request(e otlp.Event, session string) (usage.Request, bool) {
	key, _ := e.String("request_id")
	if key == "" {
		return usage.Request{}, false
	}
	r := usage.Request{Provider: usage.Claude, Key: key, Session: session, Source: usage.SourceOTel, At: e.Time}
	r.Account, _ = e.String("burns.account")
	r.Block, _ = e.String("rex.block")
	r.RexSession, _ = e.String("rex.session")
	r.Turn, _ = e.String("prompt.id")
	r.Agent, _ = e.String("agent_id")
	r.Model, _ = e.String("model")
	r.Input, _ = e.Int("input_tokens")
	r.CachedInput, _ = e.Int("cache_read_tokens")
	r.CacheWrite, _ = e.Int("cache_creation_tokens")
	r.Output, _ = e.Int("output_tokens")
	if c, ok := e.Int("cost_usd_micros"); ok {
		r.CostMicros = &c
	}
	qs, _ := e.String("query_source")
	r.Compaction = qs == compactSource
	return r, true
}

func compaction(e otlp.Event, session string) usage.Compaction {
	c := usage.Compaction{Provider: usage.Claude, Session: session, Source: usage.SourceOTel, At: e.Time}
	c.Account, _ = e.String("burns.account")
	c.Turn, _ = e.String("prompt.id")
	c.Trigger, _ = e.String("trigger")
	c.PreTokens = optInt(e, "pre_tokens")
	c.PostTokens = optInt(e, "post_tokens")
	c.DurationMs = optInt(e, "duration_ms")
	return c
}

func optInt(e otlp.Event, key string) *int64 {
	if v, ok := e.Int(key); ok {
		return &v
	}
	return nil
}
