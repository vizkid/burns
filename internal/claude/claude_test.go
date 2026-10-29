package claude

import (
	"strings"
	"testing"
	"time"

	"github.com/vizkid/burns/internal/otlp"
	"github.com/vizkid/burns/internal/usage"
)

const otelExport = `{"resourceLogs":[{"resource":{"attributes":[
 {"key":"burns.account","value":{"stringValue":"personal"}},
 {"key":"rex.block","value":{"stringValue":"block:b1"}},
 {"key":"rex.session","value":{"stringValue":"session:r1"}}]},
 "scopeLogs":[{"logRecords":[
  {"timeUnixNano":"1793160063663000000","attributes":[
   {"key":"event.name","value":{"stringValue":"api_request"}},
   {"key":"session.id","value":{"stringValue":"s1"}},
   {"key":"prompt.id","value":{"stringValue":"p1"}},
   {"key":"request_id","value":{"stringValue":"req_1"}},
   {"key":"model","value":{"stringValue":"claude-haiku-5-5"}},
   {"key":"query_source","value":{"stringValue":"main"}},
   {"key":"input_tokens","value":{"intValue":2}},
   {"key":"output_tokens","value":{"intValue":10}},
   {"key":"cache_read_tokens","value":{"intValue":46034}},
   {"key":"cache_creation_tokens","value":{"intValue":20435}},
   {"key":"cost_usd_micros","value":{"intValue":4553}}]},
  {"timeUnixNano":"1793160073000000000","attributes":[
   {"key":"event.name","value":{"stringValue":"api_request"}},
   {"key":"session.id","value":{"stringValue":"s1"}},
   {"key":"prompt.id","value":{"stringValue":"p2"}},
   {"key":"request_id","value":{"stringValue":"req_2"}},
   {"key":"agent_id","value":{"stringValue":"a9"}},
   {"key":"query_source","value":{"stringValue":"compact"}},
   {"key":"input_tokens","value":{"intValue":1998}},
   {"key":"output_tokens","value":{"intValue":1074}}]},
  {"timeUnixNano":"1793160073596000000","attributes":[
   {"key":"event.name","value":{"stringValue":"compaction"}},
   {"key":"session.id","value":{"stringValue":"s1"}},
   {"key":"prompt.id","value":{"stringValue":"p2"}},
   {"key":"trigger","value":{"stringValue":"manual"}},
   {"key":"success","value":{"stringValue":"true"}},
   {"key":"duration_ms","value":{"stringValue":"6739"}},
   {"key":"pre_tokens","value":{"stringValue":"35988"}},
   {"key":"post_tokens","value":{"stringValue":"14838"}}]},
  {"attributes":[{"key":"event.name","value":{"stringValue":"user_prompt"}}]}
 ]}]}]}`

func TestFromEvents(t *testing.T) {
	evs, err := otlp.ParseLogs([]byte(otelExport))
	if err != nil {
		t.Fatal(err)
	}
	reqs, comps := FromEvents(evs)
	if len(reqs) != 2 || len(comps) != 1 {
		t.Fatalf("got %d requests, %d compactions; want 2, 1", len(reqs), len(comps))
	}
	r := reqs[0]
	// A leftover burns.account label is ignored: the store takes the account from the session.
	if r.Key != "req_1" || r.Session != "s1" || r.Turn != "p1" || r.Account != "" ||
		r.Block != "block:b1" || r.RexSession != "session:r1" || r.Source != usage.SourceOTel {
		t.Fatalf("identity fields wrong: %+v", r)
	}
	if r.Input != 2 || r.CachedInput != 46034 || r.CacheWrite != 20435 || r.Output != 10 || r.Total() != 66481 {
		t.Fatalf("tokens wrong: %+v", r)
	}
	if r.CostMicros == nil || *r.CostMicros != 4553 || r.Compaction {
		t.Fatalf("cost or compaction flag wrong: %+v", r)
	}
	if c := reqs[1]; !c.Compaction || c.Agent != "a9" || c.CostMicros != nil {
		t.Fatalf("compact request: want flagged, agent a9, cost unknown; got %+v", c)
	}
	c := comps[0]
	if c.Trigger != "manual" || *c.PreTokens != 35988 || *c.PostTokens != 14838 || *c.DurationMs != 6739 ||
		c.Turn != "p2" || c.Account != "" || c.Source != usage.SourceOTel {
		t.Fatalf("compaction wrong: %+v", c)
	}
}

func TestStatuslineAbsentWindowStaysAbsent(t *testing.T) {
	now := time.Date(2026, 10, 28, 4, 0, 0, 0, time.UTC)
	body := `{"session_id":"s1","transcript_path":"/h/.claude-x/projects/p/s1.jsonl",
	 "rate_limits":{"seven_day":{"used_percentage":0,"resets_at":1793300000}}}`
	st, err := ParseStatusline([]byte(body), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Windows[300]; ok {
		t.Fatal("absent five_hour must stay absent")
	}
	w, ok := st.Windows[10080]
	if !ok || w.Percent == nil || *w.Percent != 0 || !w.ResetsAt.Equal(time.Unix(1793300000, 0)) || !w.ObservedAt.Equal(now) {
		t.Fatalf("seven_day = %+v", w)
	}
	if st.Session != "s1" || st.TranscriptPath == "" {
		t.Fatalf("identity missing: %+v", st)
	}
}

func TestStatuslineAcceptsRFC3339AndNoLimits(t *testing.T) {
	now := time.Date(2026, 10, 28, 4, 0, 0, 0, time.UTC)
	st, err := ParseStatusline([]byte(`{"rate_limits":{"five_hour":{"used_percentage":12.5,"resets_at":"2026-10-28T06:00:00Z"}}}`), now)
	if err != nil || *st.Windows[300].Percent != 12.5 || st.Windows[300].ResetsAt.Hour() != 6 {
		t.Fatalf("rfc3339 window: %+v, %v", st.Windows, err)
	}
	st, err = ParseStatusline([]byte(`{"session_id":"s"}`), now)
	if err != nil || len(st.Windows) != 0 {
		t.Fatalf("no rate_limits must give no windows: %+v, %v", st.Windows, err)
	}
}

// One request written once per content block, with the largest copy kept.
const transcript = `{"type":"user","promptId":"p1","sessionId":"s1","timestamp":"2026-10-28T04:01:01Z"}
{"type":"assistant","requestId":"req_1","sessionId":"s1","timestamp":"2026-10-28T04:01:03Z","message":{"id":"m1","model":"claude-haiku-5-5","usage":{"input_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":3}}}
{"type":"assistant","requestId":"req_1","sessionId":"s1","timestamp":"2026-10-28T04:01:03Z","message":{"id":"m1","model":"claude-haiku-5-5","usage":{"input_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":10}}}
{"type":"assistant","requestId":"req_1","sessionId":"s1","timestamp":"2026-10-28T04:01:03Z","message":{"id":"m1","model":"claude-haiku-5-5","usage":{"input_tokens":2,"cache_read_input_tokens":100,"cache_creation_input_tokens":50,"output_tokens":10}}}
{"type":"assistant","sessionId":"s1","isSidechain":true,"agentId":"a9","timestamp":"2026-10-28T04:01:04Z","message":{"id":"m2","usage":{"input_tokens":5,"output_tokens":5}}}
not json
{"type":"user","promptId":"p2","sessionId":"s1","timestamp":"2026-10-28T04:01:06Z"}
{"type":"system","subtype":"compact_boundary","sessionId":"s1","uuid":"u1","timestamp":"2026-10-28T04:01:13.532Z","compactMetadata":{"trigger":"manual","preTokens":66481,"postTokens":14838,"durationMs":6675}}
`

func TestScanTranscriptDedupes(t *testing.T) {
	reqs, comps, err := ScanTranscript(strings.NewReader(transcript), "personal")
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 2 {
		t.Fatalf("requests = %d; want 2 (req_1 once, m2 by message id)", len(reqs))
	}
	r := reqs[0]
	if r.Key != "req_1" || r.Output != 10 || r.Total() != 162 || r.Turn != "p1" || r.Account != "personal" ||
		r.Source != usage.SourceTranscript || r.CostMicros != nil {
		t.Fatalf("req_1 = %+v", r)
	}
	if reqs[1].Key != "m2" || reqs[1].Agent != "a9" {
		t.Fatalf("subagent row = %+v", reqs[1])
	}
	if len(comps) != 1 || comps[0].Trigger != "manual" || *comps[0].PreTokens != 66481 || comps[0].Turn != "p2" ||
		comps[0].Source != usage.SourceTranscript {
		t.Fatalf("compactions = %+v", comps)
	}
}
