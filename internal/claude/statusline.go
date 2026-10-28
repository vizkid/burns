package claude

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"github.com/vizkid/burns/internal/quota"
)

// Status is one statusline post.
type Status struct {
	Session        string
	TranscriptPath string
	Windows        quota.Windows
}

// Window lengths of Claude's statusline windows.
const (
	fiveHour = 300
	sevenDay = 7 * 24 * 60
)

type statusWindow struct {
	UsedPercentage *float64        `json:"used_percentage"`
	ResetsAt       json.RawMessage `json:"resets_at"`
}

type statusPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	RateLimits     *struct {
		FiveHour *statusWindow `json:"five_hour"`
		SevenDay *statusWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

// ParseStatusline decodes statusline JSON. An absent window stays absent.
func ParseStatusline(body []byte, now time.Time) (Status, error) {
	var p statusPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Status{}, err
	}
	st := Status{Session: p.SessionID, TranscriptPath: p.TranscriptPath, Windows: quota.Windows{}}
	if p.RateLimits == nil {
		return st, nil
	}
	for minutes, w := range map[int]*statusWindow{fiveHour: p.RateLimits.FiveHour, sevenDay: p.RateLimits.SevenDay} {
		if w == nil || w.UsedPercentage == nil {
			continue
		}
		reset, ok := parseReset(w.ResetsAt)
		if !ok {
			continue
		}
		pct := *w.UsedPercentage
		st.Windows[minutes] = quota.Window{Percent: &pct, ResetsAt: reset, ObservedAt: now}
	}
	return st, nil
}

// parseReset accepts epoch seconds or an RFC 3339 string.
func parseReset(raw json.RawMessage) (time.Time, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return time.Time{}, false
		}
		t, err := time.Parse(time.RFC3339, s)
		return t, err == nil
	}
	secs, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(int64(secs), 0), true
}
