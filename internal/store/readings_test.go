package store

import (
	"testing"
	"time"

	"github.com/vizkid/burns/internal/quota"
	"github.com/vizkid/burns/internal/usage"
)

func weekly(pct float64, reset, at time.Time) quota.Report {
	return quota.Report{Provider: usage.Codex, At: at, Windows: quota.Windows{
		10080: {Percent: &pct, ResetsAt: reset, ObservedAt: at}}}
}

func percents(t *testing.T, s *Store, now time.Time) []float64 {
	t.Helper()
	rs, err := s.Readings(now)
	if err != nil {
		t.Fatal(err)
	}
	var out []float64
	for _, r := range rs {
		out = append(out, r.Percent)
	}
	return out
}

func equal(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The statusline repeats the same percent on every render; only changes are kept.
func TestReadingsKeepOnlyChanges(t *testing.T) {
	s := open(t)
	reset := t0.Add(72 * time.Hour)
	for i, pct := range []float64{30, 30, 31, 31, 33} {
		at := t0.Add(time.Duration(i) * time.Minute)
		if err := s.ApplyReports("work", "codex", []quota.Report{weekly(pct, reset, at)}, at); err != nil {
			t.Fatal(err)
		}
	}
	if got := percents(t, s, t0.Add(time.Hour)); !equal(got, []float64{30, 31, 33}) {
		t.Fatalf("readings = %v; want [30 31 33]", got)
	}
}

// A batch from one file records each change in it, in order.
func TestReadingsFromOneBatch(t *testing.T) {
	s := open(t)
	reset := t0.Add(72 * time.Hour)
	batch := []quota.Report{weekly(10, reset, t0), weekly(12, reset, t0.Add(time.Minute)), weekly(12, reset, t0.Add(2*time.Minute))}
	if err := s.ApplyReports("work", "codex", batch, t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := percents(t, s, t0.Add(time.Hour)); !equal(got, []float64{10, 12}) {
		t.Fatalf("readings = %v; want [10 12]", got)
	}
}

// A reset that moves by seconds is the same window; one that moves by hours is a new one.
func TestReadingsNewWindowButNotResetJitter(t *testing.T) {
	s := open(t)
	reset := t0.Add(time.Hour)
	s.ApplyReports("work", "codex", []quota.Report{weekly(40, reset, t0)}, t0)
	at := t0.Add(time.Minute)
	s.ApplyReports("work", "codex", []quota.Report{weekly(40, reset.Add(time.Second), at)}, at)
	if got := percents(t, s, at); !equal(got, []float64{40}) {
		t.Fatalf("readings = %v; want [40]: a one-second reset shift is not a new window", got)
	}
	next := t0.Add(2 * time.Hour)
	s.ApplyReports("work", "codex", []quota.Report{weekly(1, next.Add(7*24*time.Hour), next)}, next)
	if got := percents(t, s, next); !equal(got, []float64{1}) {
		t.Fatalf("readings = %v; want [1]: the finished window's readings are not served", got)
	}
}

// A report older than the last reading adds nothing: the line only moves forward.
func TestReadingsIgnoreOlderReports(t *testing.T) {
	s := open(t)
	reset := t0.Add(72 * time.Hour)
	later := t0.Add(10 * time.Minute)
	s.ApplyReports("work", "codex", []quota.Report{weekly(20, reset, later)}, later)
	s.ApplyReports("work", "codex", []quota.Report{weekly(15, reset, t0)}, later)
	if got := percents(t, s, later); !equal(got, []float64{20}) {
		t.Fatalf("readings = %v; want [20]", got)
	}
}

// Readings name their account and window, oldest first, and sweep away once the window resets.
func TestReadingsServeLiveWindowsAndSweep(t *testing.T) {
	s := open(t)
	p := func(v float64) *float64 { return &v }
	soon, week := t0.Add(time.Hour), t0.Add(72*time.Hour)
	claude := quota.Report{Provider: usage.Claude, At: t0, Windows: quota.Windows{
		300:   {Percent: p(5), ResetsAt: soon, ObservedAt: t0},
		10080: {Percent: p(50), ResetsAt: week, ObservedAt: t0}}}
	s.ApplyReports("home", "claude", []quota.Report{claude}, t0)
	at := t0.Add(time.Minute)
	s.ApplyReports("work", "codex", []quota.Report{weekly(9, week, at)}, at)

	rs, err := s.Readings(at)
	if err != nil || len(rs) != 3 {
		t.Fatalf("readings = %+v, %v; want 3", rs, err)
	}
	if last := rs[2]; last.Account != "work" || last.LimitID != "codex" || last.Minutes != 10080 ||
		last.Percent != 9 || !last.ResetsAt.Equal(week) || !last.ObservedAt.Equal(at) {
		t.Fatalf("newest reading = %+v", last)
	}

	after := soon.Add(time.Minute)
	if err := s.Sweep(after); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM reading`).Scan(&n)
	if n != 2 {
		t.Fatalf("%d readings after sweep; want 2: the five-hour window reset", n)
	}
}
