package store

import (
	"testing"
	"time"

	"github.com/vizkid/burns/internal/quota"
	"github.com/vizkid/burns/internal/usage"
)

func changes(t *testing.T, s *Store) int64 {
	t.Helper()
	var n int64
	if err := s.db.QueryRow(`SELECT total_changes()`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Re-reading a file must not rewrite rows that did not change.
func TestUnchangedWritesAreSkipped(t *testing.T) {
	s := open(t)
	p := func(v float64) *float64 { return &v }
	rep := quota.Report{Provider: usage.Codex, At: t0, Windows: quota.Windows{
		10080: {Percent: p(30), ResetsAt: t0.Add(48 * time.Hour), ObservedAt: t0}}}
	rs := []usage.Request{req("r1", usage.SourceOTel, 10)}
	cr := []usage.Compaction{{Provider: usage.Codex, Key: "k", RequestKey: "r1", Session: "s1", Source: usage.SourceRollout, At: t0}}
	if err := s.ApplyReports("work", "codex", []quota.Report{rep}, t0); err != nil {
		t.Fatal(err)
	}
	s.AddRequests(rs)
	s.AddCompactions(cr)
	before := changes(t, s)
	s.ApplyReports("work", "codex", []quota.Report{rep, rep}, t0)
	s.AddRequests(rs)
	s.AddCompactions(cr)
	if after := changes(t, s); after != before {
		t.Fatalf("re-ingest changed %d rows; want 0", after-before)
	}
}

// A batch of reports folds in order: rule 5 still clears an omitted
// five-hour window when a later report leaves it out.
func TestApplyReportsFoldsInOrder(t *testing.T) {
	s := open(t)
	p := func(v float64) *float64 { return &v }
	first := quota.Report{Provider: usage.Codex, At: t0, Windows: quota.Windows{
		300:   {Percent: p(10), ResetsAt: t0.Add(time.Hour), ObservedAt: t0},
		10080: {Percent: p(30), ResetsAt: t0.Add(48 * time.Hour), ObservedAt: t0}}}
	later := quota.Report{Provider: usage.Codex, At: t0.Add(time.Minute), Windows: quota.Windows{
		10080: {Percent: p(31), ResetsAt: t0.Add(48 * time.Hour), ObservedAt: t0.Add(time.Minute)}}}
	if err := s.ApplyReports("work", "codex", []quota.Report{first, later}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	ls, _ := s.Limits()
	if len(ls) != 1 || len(ls[0].Windows) != 1 {
		t.Fatalf("limits = %+v; want only the weekly window", ls)
	}
}

// Readers use their own connections: an open write never blocks a read.
func TestReadsDoNotWaitForWriter(t *testing.T) {
	s := open(t)
	s.AddRequests([]usage.Request{req("r1", usage.SourceOTel, 1)})
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE request SET output = 2`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Sessions(t0.Add(-time.Hour)); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read blocked behind an open write transaction")
	}
}
