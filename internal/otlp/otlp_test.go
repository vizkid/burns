package otlp

import (
	"testing"
	"time"
)

const export = `{"resourceLogs":[{"resource":{"attributes":[
 {"key":"service.name","value":{"stringValue":"claude-code"}},
 {"key":"burns.account","value":{"stringValue":"personal"}},
 {"key":"rex.block","value":{"stringValue":"block:01a17dc5-6fe2-7b13-a7bf-000000000001"}},
 {"key":"session.id","value":{"stringValue":"from-resource"}}]},
 "scopeLogs":[{"logRecords":[
  {"timeUnixNano":"1793160063663000000","body":{"stringValue":"claude_code.api_request"},"attributes":[
   {"key":"event.name","value":{"stringValue":"api_request"}},
   {"key":"session.id","value":{"stringValue":"s1"}},
   {"key":"input_tokens","value":{"intValue":2}},
   {"key":"output_tokens","value":{"intValue":"10"}},
   {"key":"cost_usd_micros","value":{"intValue":"9007199254740993"}},
   {"key":"pre_tokens","value":{"stringValue":"35988"}},
   {"key":"model","value":{"stringValue":"claude-haiku-5-5"}}]},
  {"body":{"stringValue":"claude_code.compaction"},"attributes":[
   {"key":"event.timestamp","value":{"stringValue":"2026-10-28T04:01:13.596Z"}}]}
 ]}]}]}`

func TestParseLogsFlattensRecords(t *testing.T) {
	evs, err := ParseLogs([]byte(export))
	if err != nil || len(evs) != 2 {
		t.Fatalf("events = %d, err = %v; want 2, nil", len(evs), err)
	}
	e := evs[0]
	if e.Name != "api_request" {
		t.Fatalf("name = %q; want api_request", e.Name)
	}
	if want := time.Unix(0, 1793160063663000000); !e.Time.Equal(want) {
		t.Fatalf("time = %v; want %v", e.Time, want)
	}
	if v, _ := e.String("session.id"); v != "s1" {
		t.Fatalf("record attribute must win over resource: got %q", v)
	}
	if v, _ := e.String("burns.account"); v != "personal" {
		t.Fatalf("resource attribute missing: got %q", v)
	}
	if v, _ := e.String("rex.block"); v != "block:01a17dc5-6fe2-7b13-a7bf-000000000001" {
		t.Fatalf("rex.block = %q", v)
	}
}

func TestIntAcceptsAllWireForms(t *testing.T) {
	evs, _ := ParseLogs([]byte(export))
	e := evs[0]
	for key, want := range map[string]int64{
		"input_tokens": 2, "output_tokens": 10, "cost_usd_micros": 9007199254740993, "pre_tokens": 35988,
	} {
		if got, ok := e.Int(key); !ok || got != want {
			t.Errorf("%s = %d, %v; want %d", key, got, ok, want)
		}
	}
}

// Rule 9 starts here: a missing number is unknown, not zero.
func TestMissingIntIsNotZero(t *testing.T) {
	evs, _ := ParseLogs([]byte(export))
	if _, ok := evs[0].Int("cache_read_tokens"); ok {
		t.Fatal("missing attribute must report ok=false")
	}
	if _, ok := evs[0].Int("model"); ok {
		t.Fatal("non-numeric string must report ok=false")
	}
}

// The name falls back from event.name to the body without its prefix.
// A record without timeUnixNano uses event.timestamp, never the receive time.
func TestNameAndTimeFallbacks(t *testing.T) {
	evs, _ := ParseLogs([]byte(export))
	e := evs[1]
	if e.Name != "compaction" {
		t.Fatalf("name = %q; want compaction", e.Name)
	}
	if want := time.Date(2026, 10, 28, 4, 1, 13, 596e6, time.UTC); !e.Time.Equal(want) {
		t.Fatalf("time = %v; want %v", e.Time, want)
	}
}

func TestParseLogsRejectsMalformed(t *testing.T) {
	if _, err := ParseLogs([]byte(`{"resourceLogs":[`)); err == nil {
		t.Fatal("malformed JSON must return an error")
	}
}
