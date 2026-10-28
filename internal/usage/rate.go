package usage

import "time"

// Sample is one session's cumulative token count at a time.
type Sample struct {
	At     time.Time
	Tokens int64
}

// Rate returns tokens per minute between the first and last sample.
// It needs two samples at different times (rule 8).
func Rate(samples []Sample) (perMinute float64, ok bool) {
	if len(samples) < 2 {
		return 0, false
	}
	first, last := samples[0], samples[len(samples)-1]
	minutes := last.At.Sub(first.At).Minutes()
	if minutes <= 0 {
		return 0, false
	}
	return float64(last.Tokens-first.Tokens) / minutes, true
}
