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
	if st.TranscriptPath == "" {
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

func TestTranscriptDedupes(t *testing.T) {
	var lines [][]byte
	for _, line := range strings.Split(transcript, "\n") {
		lines = append(lines, []byte(line))
	}
	d := &Transcript{Account: "personal"}
	reqs, comps := d.Lines(lines)
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

// Telemetry names the cache lifetime and the speed. Standard speed is stored as empty.
func TestEventCarriesCacheLifetimeAndSpeed(t *testing.T) {
	logs := func(speed string) []byte {
		return []byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"1793160073000000000","attributes":[
		 {"key":"event.name","value":{"stringValue":"api_request"}},
		 {"key":"session.id","value":{"stringValue":"s1"}},
		 {"key":"request_id","value":{"stringValue":"r1"}},
		 {"key":"cache_creation_tokens","value":{"intValue":900}},
		 {"key":"cache_creation_1h_tokens","value":{"intValue":600}},
		 {"key":"speed","value":{"stringValue":"` + speed + `"}}]}]}]}]}`)
	}
	for speed, want := range map[string]string{"fast": "fast", "normal": ""} {
		evs, err := otlp.ParseLogs(logs(speed))
		if err != nil {
			t.Fatal(err)
		}
		reqs, _ := FromEvents(evs)
		if len(reqs) != 1 || reqs[0].CacheWrite1h != 600 || reqs[0].Speed != want {
			t.Fatalf("speed %s: requests = %+v", speed, reqs)
		}
	}
}

// A transcript line carries the same split under usage.cache_creation.
func TestTranscriptCarriesCacheLifetimeAndSpeed(t *testing.T) {
	line := `{"type":"assistant","sessionId":"s1","requestId":"r1","timestamp":"2026-10-20T03:26:14Z","message":{"id":"m1","model":"claude-opus-5-5",` +
		`"usage":{"input_tokens":1,"cache_creation_input_tokens":900,"cache_read_input_tokens":2,"output_tokens":3,"speed":"standard",` +
		`"cache_creation":{"ephemeral_1h_input_tokens":600,"ephemeral_5m_input_tokens":300}}}}`
	d := &Transcript{Account: "personal"}
	reqs, _ := d.Lines([][]byte{[]byte(line)})
	if len(reqs) != 1 || reqs[0].CacheWrite1h != 600 || reqs[0].Speed != "" {
		t.Fatalf("requests = %+v", reqs)
	}
}

// The statusline's context_window gives the session's context now.
func TestStatuslineReportsContext(t *testing.T) {
	now := time.Date(2026, 11, 6, 4, 0, 0, 0, time.UTC)
	body := `{"session_id":"s1","transcript_path":"/h/.claude-x/projects/p/s1.jsonl",
	 "context_window":{"total_input_tokens":124000,"context_window_size":1000000,"used_percentage":12}}`
	st, err := ParseStatusline([]byte(body), now)
	if err != nil {
		t.Fatal(err)
	}
	c := st.Context
	if c == nil || c.Provider != usage.Claude || c.Session != "s1" || c.Used != 124000 || c.Size != 1000000 || !c.At.Equal(now) {
		t.Fatalf("context = %+v", c)
	}
	st, _ = ParseStatusline([]byte(`{"session_id":"s1","context_window":{"total_input_tokens":0,"context_window_size":0}}`), now)
	if st.Context != nil {
		t.Fatalf("no window size must give no context: %+v", st.Context)
	}
}
