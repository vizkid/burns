package quota

import (
	"testing"
	"time"
)

var now = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func pct(v float64) *float64 { return &v }

func win(p float64, resetIn, age time.Duration) Window {
	return Window{Percent: pct(p), ResetsAt: now.Add(resetIn), ObservedAt: now.Add(-age)}
}

func mustValue(t *testing.T, w Window, want float64) {
	t.Helper()
	got, ok := w.Value(now)
	if !ok || got != want {
		t.Fatalf("value = %v, %v; want %v, true", got, ok, want)
	}
}

func mustUnknown(t *testing.T, w Window) {
	t.Helper()
	if got, ok := w.Value(now); ok {
		t.Fatalf("value = %v; want unknown", got)
	}
}

// Rule 1: a report that omits one window never erases the other.
func TestRule1WindowsIndependent(t *testing.T) {
	stored := Windows{300: win(40, time.Hour, time.Minute), 10080: win(20, 48*time.Hour, time.Minute)}
	got := Apply(stored, Report{Provider: "claude", Windows: Windows{300: win(45, time.Hour, 0)}}, now)
	mustValue(t, got[300], 45)
	mustValue(t, got[10080], 20)
}

// Rule 2: a window whose reset time has passed is unknown, never zero.
func TestRule2ExpiredIsUnknown(t *testing.T) {
	mustUnknown(t, win(80, -time.Minute, time.Hour))
	mustUnknown(t, Window{})
}

// Rule 2: a report that arrives with a past reset does not replace a live reading.
func TestRule2StaleReportRejected(t *testing.T) {
	stored := Windows{300: win(40, time.Hour, time.Minute)}
	got := Apply(stored, Report{Provider: "codex", Windows: Windows{300: win(90, -time.Minute, 0)}}, now)
	mustValue(t, got[300], 40)
}

// Rule 3: a reported zero with a future reset is a real reading.
func TestRule3ZeroIsReal(t *testing.T) {
	got := Apply(nil, Report{Provider: "claude", Windows: Windows{300: win(0, time.Hour, 0)}}, now)
	mustValue(t, got[300], 0)
}

// Rule 4: if Claude omits a window, keep the stored reading until its reset passes.
func TestRule4ClaudeOmitKeeps(t *testing.T) {
	stored := Windows{300: win(40, time.Hour, 10*time.Minute)}
	got := Apply(stored, Report{Provider: "claude", Windows: Windows{}}, now)
	mustValue(t, got[300], 40)
	if !got[300].ObservedAt.Equal(stored[300].ObservedAt) {
		t.Fatal("kept window must keep its original observation time")
	}
	expired := Windows{300: win(40, -time.Minute, time.Hour)}
	mustUnknown(t, Apply(expired, Report{Provider: "claude"}, now)[300])
}

// Rule 5: if Codex omits the five-hour window, clear it. The weekly window is kept.
func TestRule5CodexOmitFiveHourClears(t *testing.T) {
	stored := Windows{300: win(40, time.Hour, time.Minute), 10080: win(20, 48*time.Hour, time.Minute)}
	got := Apply(stored, Report{Provider: "codex", Windows: Windows{10080: win(21, 48*time.Hour, 0)}}, now)
	if _, ok := got[300]; ok {
		t.Fatal("codex five-hour window must be cleared when omitted")
	}
	mustValue(t, got[10080], 21)
}

// An older report never overwrites a newer stored reading.
func TestOlderReportIgnored(t *testing.T) {
	stored := Windows{300: win(50, time.Hour, time.Minute)}
	got := Apply(stored, Report{Provider: "claude", Windows: Windows{300: win(30, time.Hour, time.Hour)}}, now)
	mustValue(t, got[300], 50)
}

// Rule 6: when merging hosts, take the newest live reading per window. Keep source times.
// The five-hour readings have resets 30 minutes apart: two windows, so the newer one wins
// even though it is lower (rule 10 applies only within one window).
func TestRule6MergeNewestPerWindow(t *testing.T) {
	a := Windows{300: win(10, time.Hour, time.Minute), 10080: win(60, 48*time.Hour, time.Hour)}
	b := Windows{300: win(20, 30*time.Minute, time.Hour), 10080: win(70, 48*time.Hour, time.Minute)}
	got := Merge(now, a, b)
	mustValue(t, got[300], 10)
	mustValue(t, got[10080], 70)
	if !got[10080].ObservedAt.Equal(b[10080].ObservedAt) {
		t.Fatal("merge must keep the source observation time")
	}
}

// Rule 6: an expired reading never wins a merge, however new it is.
func TestRule6MergeSkipsExpired(t *testing.T) {
	a := Windows{300: win(10, time.Hour, time.Hour)}
	b := Windows{300: win(99, -time.Second, 0)}
	mustValue(t, Merge(now, a, b)[300], 10)
	mustUnknown(t, Merge(now, b)[300])
}

// Inputs are never modified.
func TestApplyDoesNotMutate(t *testing.T) {
	stored := Windows{300: win(40, time.Hour, time.Minute)}
	Apply(stored, Report{Provider: "codex", Windows: Windows{}}, now)
	mustValue(t, stored[300], 40)
}

// Backfill replays old files: an older Codex report never clears a newer window.
func TestOlderCodexReportDoesNotClear(t *testing.T) {
	stored := Windows{300: win(40, time.Hour, time.Minute)}
	got := Apply(stored, Report{Provider: "codex", At: now.Add(-time.Hour), Windows: Windows{}}, now)
	mustValue(t, got[300], 40)
}

// Rule 10: usage in one window never goes down. A lower percent for the same
// reset comes from a session reporting its last-known value: it is stale.
func TestRule10LowerPercentInSameWindowIsStale(t *testing.T) {
	stored := Windows{10080: win(30, 48*time.Hour, time.Minute)}
	got := Apply(stored, Report{Provider: "claude", Windows: Windows{10080: win(29, 48*time.Hour+30*time.Second, 0)}}, now)
	mustValue(t, got[10080], 30)
	got = Apply(stored, Report{Provider: "claude", Windows: Windows{10080: win(31, 48*time.Hour, 0)}}, now)
	mustValue(t, got[10080], 31)
}

// A new window (a reset more than a minute away) starts low: that is not stale.
func TestRule10NewWindowMayBeLower(t *testing.T) {
	stored := Windows{10080: win(90, time.Hour, time.Minute)}
	got := Apply(stored, Report{Provider: "codex", Windows: Windows{10080: win(2, 7*24*time.Hour, 0)}}, now)
	mustValue(t, got[10080], 2)
}

// Across hosts, the same window keeps its highest percent even when another
// host's newer reading is stale.
func TestRule10MergeKeepsHighestOfSameWindow(t *testing.T) {
	a := Windows{10080: win(31, 48*time.Hour, 5*time.Minute)}
	b := Windows{10080: win(30, 48*time.Hour, time.Minute)}
	mustValue(t, Merge(now, a, b)[10080], 31)
	mustValue(t, Merge(now, b, a)[10080], 31)
	c := Windows{10080: win(1, 7*24*time.Hour, 0)}
	mustValue(t, Merge(now, a, c)[10080], 1)
}
