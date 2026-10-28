package codex

import (
	"strings"
	"testing"

	"github.com/vizkid/burns/internal/usage"
)

func run(d *Decoder, lines string) Output {
	var all Output
	for _, l := range strings.Split(strings.TrimSpace(lines), "\n") {
		o := d.Line([]byte(l))
		all.Requests = append(all.Requests, o.Requests...)
		all.Compactions = append(all.Compactions, o.Compactions...)
		all.Limits = append(all.Limits, o.Limits...)
	}
	return all
}

const meta = `{"timestamp":"2026-10-20T03:26:09Z","type":"session_meta","payload":{"id":"t1","session_id":"t1","cli_version":"0.165.0"}}
{"timestamp":"2026-10-20T03:26:10Z","type":"turn_context","payload":{"turn_id":"u1","model":"gpt-6.1-sol"}}`

const usage1 = `{"input_tokens":1000,"cached_input_tokens":800,"cache_write_input_tokens":0,"output_tokens":50,"reasoning_output_tokens":20,"total_tokens":1050}`

func TestRecordsAreTheTokenSource(t *testing.T) {
	d := &Decoder{Account: "work"}
	out := run(d, meta+`
{"timestamp":"2026-10-20T03:26:14Z","type":"token_usage_record","payload":{"thread_id":"t1","session_id":"t1","turn_id":"u1","root_turn_id":"u1","response_id":"r1","usage":`+usage1+`}}
{"timestamp":"2026-10-20T03:26:15Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":999999,"output_tokens":1,"total_tokens":1000000},"last_token_usage":{"input_tokens":1,"output_tokens":1}},"rate_limits":null}}`)
	if len(out.Requests) != 1 {
		t.Fatalf("requests = %d; want 1 (token_count ignored once records exist)", len(out.Requests))
	}
	r := out.Requests[0]
	if r.Key != "t1/r1" || r.Session != "t1" || r.Turn != "u1" || r.Account != "work" || r.Model != "gpt-6.1-sol" ||
		r.Provider != usage.Codex || r.Source != usage.SourceRollout || r.Agent != "" {
		t.Fatalf("identity = %+v", r)
	}
	if r.Input != 200 || r.CachedInput != 800 || r.Output != 50 || r.Reasoning != 20 || r.Total() != 1050 || r.CostMicros != nil {
		t.Fatalf("tokens = %+v", r)
	}
}

func TestSubagentRollsUpToRoot(t *testing.T) {
	d := &Decoder{Account: "work"}
	out := run(d, `{"timestamp":"2026-10-20T03:26:14Z","type":"token_usage_record","payload":{"thread_id":"t2","session_id":"t1","turn_id":"u9","response_id":"r1","usage":`+usage1+`}}`)
	if r := out.Requests[0]; r.Session != "t1" || r.Agent != "t2" || r.Key != "t2/r1" {
		t.Fatalf("subagent = %+v", r)
	}
}

func TestCompactionPairsWithItsResponse(t *testing.T) {
	d := &Decoder{Account: "work"}
	out := run(d, meta+`
{"timestamp":"2026-10-20T04:00:00Z","type":"token_usage_record","payload":{"thread_id":"t1","session_id":"t1","turn_id":"u1","response_id":"rc","usage":`+usage1+`}}
{"timestamp":"2026-10-20T04:00:01Z","type":"compacted","payload":{"message":"","window_number":1,"compaction_response_id":"rc"}}`)
	if len(out.Compactions) != 1 {
		t.Fatalf("compactions = %d; want 1", len(out.Compactions))
	}
	c := out.Compactions[0]
	if c.Key != "t1/rc" || c.RequestKey != "t1/rc" || c.Session != "t1" || c.Trigger != "" || c.PreTokens != nil ||
		c.Source != usage.SourceRollout || c.Account != "work" {
		t.Fatalf("compaction = %+v", c)
	}
}

func TestRateLimitsByWindowMinutes(t *testing.T) {
	d := &Decoder{Account: "work"}
	out := run(d, `{"timestamp":"2026-10-20T03:26:15Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"limit_id":"codex","primary":{"used_percent":28.0,"window_minutes":10080,"resets_at":1792909435},"secondary":{"used_percent":3.0,"window_minutes":300,"resets_at":1792800000},"credits":{"has_credits":true,"unlimited":false,"balance":"17563.16"},"plan_type":"prolite"}}}
{"timestamp":"2026-10-20T03:26:16Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"limit_id":"premium","primary":null,"secondary":null}}}`)
	if len(out.Limits) != 2 {
		t.Fatalf("limits = %d; want 2", len(out.Limits))
	}
	l := out.Limits[0]
	if l.LimitID != "codex" || l.Plan != "prolite" || l.Report.Provider != usage.Codex {
		t.Fatalf("limit = %+v", l)
	}
	if w := l.Report.Windows[10080]; w.Percent == nil || *w.Percent != 28 || w.ResetsAt.Unix() != 1792909435 || w.ObservedAt.IsZero() {
		t.Fatalf("weekly = %+v", w)
	}
	if w := l.Report.Windows[300]; w.Percent == nil || *w.Percent != 3 {
		t.Fatalf("five-hour from secondary = %+v", w)
	}
	if l.Credits == nil || !l.Credits.HasCredits || l.Credits.Balance != "17563.16" {
		t.Fatalf("credits = %+v", l.Credits)
	}
	if p := out.Limits[1]; p.LimitID != "premium" || len(p.Report.Windows) != 0 {
		t.Fatalf("premium must report no windows: %+v", p)
	}
}

// Azure sends a rate_limits object whose fields are all null.
func TestHollowAzureBlockHasNoWindows(t *testing.T) {
	d := &Decoder{Account: "azure"}
	out := run(d, `{"timestamp":"2026-10-20T03:26:15Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"limit_id":"codex","primary":null,"secondary":null,"credits":null,"plan_type":null}}}`)
	if len(out.Limits) != 1 || len(out.Limits[0].Report.Windows) != 0 || out.Limits[0].Credits != nil {
		t.Fatalf("azure = %+v", out.Limits)
	}
}

// Rollouts older than 0.153.4 have no records: use the growth of the total.
func TestFallbackForOldRollouts(t *testing.T) {
	tc := func(ts string, total, last int64) string {
		return `{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":` +
			itoa(total) + `,"output_tokens":0,"total_tokens":` + itoa(total) + `},"last_token_usage":{"input_tokens":` +
			itoa(last) + `,"output_tokens":0,"total_tokens":` + itoa(last) + `}}}}`
	}
	old := `{"timestamp":"2026-08-01T00:00:00Z","type":"session_meta","payload":{"id":"t1","session_id":"t1","cli_version":"0.150.0"}}`
	d := &Decoder{Account: "work"}
	out := run(d, old+"\n"+
		tc("2026-08-01T00:00:01Z", 100, 100)+"\n"+
		tc("2026-08-01T00:00:02Z", 100, 100)+"\n"+ // repeat: skipped
		tc("2026-08-01T00:00:03Z", 250, 150)+"\n"+
		`{"timestamp":"2026-08-01T00:00:04Z","type":"event_msg","payload":{"type":"token_count","info":null}}`+"\n"+
		tc("2026-08-01T00:00:05Z", 40, 40)) // reset: take last_token_usage
	var sum int64
	for _, r := range out.Requests {
		sum += r.Total()
	}
	if len(out.Requests) != 3 || sum != 290 {
		t.Fatalf("fallback requests = %d, sum %d; want 3, 290", len(out.Requests), sum)
	}
	if out.Requests[0].Key == out.Requests[1].Key {
		t.Fatal("fallback keys must be unique")
	}
}

// A rollout from a CLI that writes records never falls back, even before its first record.
func TestNewRolloutNeverFallsBack(t *testing.T) {
	d := &Decoder{Account: "work"}
	out := run(d, meta+`
{"timestamp":"2026-10-20T03:26:15Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":5,"output_tokens":5,"total_tokens":10},"last_token_usage":{"input_tokens":5,"output_tokens":5,"total_tokens":10}}}}`)
	if len(out.Requests) != 0 {
		t.Fatalf("requests = %+v; want none", out.Requests)
	}
	for v, want := range map[string]bool{"0.153.4": true, "0.153.3": false, "0.165.0": true, "1.0.0": true, "0.160.0-alpha.1": true, "": false} {
		if got := atLeast(v, recordsSince); got != want {
			t.Errorf("atLeast(%q) = %v", v, got)
		}
	}
}

func TestMalformedLineIgnored(t *testing.T) {
	d := &Decoder{}
	if o := d.Line([]byte(`{"type":"token_usage_record","payload":`)); len(o.Requests) != 0 {
		t.Fatal("malformed line must produce nothing")
	}
}

func itoa(v int64) string {
	b := []byte{}
	if v == 0 {
		return "0"
	}
	for ; v > 0; v /= 10 {
		b = append([]byte{byte('0' + v%10)}, b...)
	}
	return string(b)
}
