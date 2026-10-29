// Package quota applies the spec's window rules. Every function is pure.
package quota

import "time"

// FiveHourMax is the longest window treated as a five-hour window.
// Codex is classified by window_minutes, never by position.
const FiveHourMax = 600

// ResetJitter is how far a reported reset may move within one window. Larger
// moves start a new window.
const ResetJitter = time.Minute

// Window is one quota window. A nil Percent was not reported.
type Window struct {
	Percent    *float64  `json:"percent"`
	ResetsAt   time.Time `json:"resets_at"`
	ObservedAt time.Time `json:"observed_at"`
}

// Windows maps window length in minutes to its window.
type Windows map[int]Window

// Report is one observation of one account's limit, from one source.
// At is when the report was observed; a zero At counts as current.
type Report struct {
	Provider string
	At       time.Time
	Windows  Windows
}

// Live reports whether the window holds a current reading.
func (w Window) Live(now time.Time) bool { return w.Percent != nil && w.ResetsAt.After(now) }

// Value returns percent used. ok is false when the reading is unknown.
func (w Window) Value(now time.Time) (pct float64, ok bool) {
	if !w.Live(now) {
		return 0, false
	}
	return *w.Percent, true
}

// SameWindow reports whether two readings belong to one window: their resets
// are at most ResetJitter apart.
func SameWindow(a, b Window) bool {
	d := a.ResetsAt.Sub(b.ResetsAt)
	return d <= ResetJitter && d >= -ResetJitter
}

// Stale reports whether w is lower than prev in the same window (rule 10).
// Usage in a window never goes down; a lower percent comes from a session
// that repeats the value of its last response.
func Stale(prev, w Window) bool {
	return prev.Percent != nil && w.Percent != nil && SameWindow(prev, w) && *w.Percent < *prev.Percent
}

// Apply folds a report into the stored windows of one account and limit.
//
// A reported window replaces the stored one when it is live and not older.
// An omitted window is kept, except a Codex five-hour window, which is cleared:
// a Codex account can report only a weekly window (rule 5). Kept windows keep
// their original observation time and read as unknown once their reset passes.
func Apply(stored Windows, r Report, now time.Time) Windows {
	out := Windows{}
	for m, w := range stored {
		_, reported := r.Windows[m]
		newer := r.At.IsZero() || !r.At.Before(w.ObservedAt)
		if !reported && r.Provider == "codex" && m <= FiveHourMax && newer {
			continue
		}
		out[m] = w
	}
	for m, w := range r.Windows {
		if !w.Live(now) {
			continue
		}
		if prev, ok := out[m]; ok && prev.Live(now) && (w.ObservedAt.Before(prev.ObservedAt) || Stale(prev, w)) {
			continue
		}
		out[m] = w
	}
	return out
}

// Merge keeps the newest live reading per window across hosts. Within one
// window it keeps the highest percent instead: another host's newer reading
// can be stale (rule 10).
func Merge(now time.Time, sets ...Windows) Windows {
	out := Windows{}
	for _, set := range sets {
		for m, w := range set {
			if !w.Live(now) {
				continue
			}
			if prev, ok := out[m]; ok {
				if SameWindow(prev, w) {
					higher := *w.Percent > *prev.Percent
					if !higher && (*w.Percent < *prev.Percent || !w.ObservedAt.After(prev.ObservedAt)) {
						continue
					}
				} else if !w.ObservedAt.After(prev.ObservedAt) {
					continue
				}
			}
			out[m] = w
		}
	}
	return out
}
