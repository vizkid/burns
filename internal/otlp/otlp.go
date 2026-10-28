// Package otlp decodes OTLP/HTTP JSON log exports.
// It keeps only what smithersd needs: one flat Event per log record.
package otlp

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Event is one log record with its resource attributes copied in.
// Record attributes win over resource attributes with the same key.
type Event struct {
	Name  string
	Time  time.Time
	Attrs map[string]json.RawMessage
}

type keyValue struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type exportRequest struct {
	ResourceLogs []struct {
		Resource struct {
			Attributes []keyValue `json:"attributes"`
		} `json:"resource"`
		ScopeLogs []struct {
			LogRecords []record `json:"logRecords"`
		} `json:"scopeLogs"`
	} `json:"resourceLogs"`
}

type record struct {
	TimeUnixNano string          `json:"timeUnixNano"`
	Body         json.RawMessage `json:"body"`
	Attributes   []keyValue      `json:"attributes"`
}

// anyValue is the subset of the OTLP AnyValue that carries scalars.
// intValue arrives bare or quoted, so it stays raw until read.
type anyValue struct {
	StringValue *string         `json:"stringValue"`
	IntValue    json.RawMessage `json:"intValue"`
	DoubleValue *float64        `json:"doubleValue"`
	BoolValue   *bool           `json:"boolValue"`
}

// ParseLogs decodes an ExportLogsServiceRequest body.
func ParseLogs(body []byte) ([]Event, error) {
	var ex exportRequest
	if err := json.Unmarshal(body, &ex); err != nil {
		return nil, err
	}
	var out []Event
	for _, rl := range ex.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			for _, r := range sl.LogRecords {
				out = append(out, toEvent(rl.Resource.Attributes, r))
			}
		}
	}
	return out, nil
}

func toEvent(resource []keyValue, r record) Event {
	e := Event{Attrs: make(map[string]json.RawMessage, len(resource)+len(r.Attributes))}
	for _, kv := range resource {
		e.Attrs[kv.Key] = kv.Value
	}
	for _, kv := range r.Attributes {
		e.Attrs[kv.Key] = kv.Value
	}
	e.Name, _ = e.String("event.name")
	if e.Name == "" {
		var body anyValue
		if json.Unmarshal(r.Body, &body) == nil && body.StringValue != nil {
			e.Name = strings.TrimPrefix(*body.StringValue, "claude_code.")
		}
	}
	if ns, err := strconv.ParseInt(r.TimeUnixNano, 10, 64); err == nil && ns > 0 {
		e.Time = time.Unix(0, ns)
	} else if ts, ok := e.String("event.timestamp"); ok {
		e.Time, _ = time.Parse(time.RFC3339Nano, ts)
	}
	return e
}

func (e Event) value(key string) (anyValue, bool) {
	raw, ok := e.Attrs[key]
	if !ok {
		return anyValue{}, false
	}
	var v anyValue
	return v, json.Unmarshal(raw, &v) == nil
}

// String returns a string attribute.
func (e Event) String(key string) (string, bool) {
	v, ok := e.value(key)
	if !ok || v.StringValue == nil {
		return "", false
	}
	return *v.StringValue, true
}

// Int returns an integer attribute sent as intValue (bare or quoted) or as a numeric stringValue.
func (e Event) Int(key string) (int64, bool) {
	v, ok := e.value(key)
	if !ok {
		return 0, false
	}
	var s string
	switch {
	case len(v.IntValue) > 0 && string(v.IntValue) != "null":
		s = strings.Trim(string(v.IntValue), `"`)
	case v.StringValue != nil:
		s = *v.StringValue
	default:
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}
