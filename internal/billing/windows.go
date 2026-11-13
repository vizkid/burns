package billing

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/vizkid/burns/internal/quota"
)

// usageWindows are the endpoint's quota windows by length in minutes.
var usageWindows = map[string]int{"five_hour": 300, "seven_day": 7 * 24 * 60}

// parseWindows reads the usage endpoint's quota windows (docs/SPEC.md, rule 11).
// It returns nil, and so no report, unless the body names at least one window
// key: a changed shape must not clear the statusline's readings. A window that
// is null or has no reset time is left out, so it reads as unknown: Burns never
// infers a reset (rule 7).
func parseWindows(body []byte, now time.Time) quota.Windows {
	var doc map[string]json.RawMessage
	if json.Unmarshal(body, &doc) != nil {
		return nil
	}
	var out quota.Windows
	for key, minutes := range usageWindows {
		raw, named := doc[key]
		if !named {
			continue
		}
		if out == nil {
			out = quota.Windows{}
		}
		var w struct {
			Utilization *float64        `json:"utilization"`
			ResetsAt    json.RawMessage `json:"resets_at"`
		}
		if json.Unmarshal(raw, &w) != nil || w.Utilization == nil {
			continue
		}
		reset, ok := resetTime(w.ResetsAt)
		if !ok {
			continue
		}
		out[minutes] = quota.Window{Percent: w.Utilization, ResetsAt: reset, ObservedAt: now}
	}
	return out
}

// resetTime accepts an RFC 3339 string or epoch seconds.
func resetTime(raw json.RawMessage) (time.Time, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		t, err := time.Parse(time.RFC3339Nano, s)
		return t.UTC(), err == nil
	}
	secs, err := strconv.ParseFloat(string(raw), 64)
	if err != nil || len(raw) == 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(secs), 0).UTC(), true
}
