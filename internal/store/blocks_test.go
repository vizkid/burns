package store

import (
	"testing"
	"time"

	"github.com/vizkid/burns/internal/usage"
)

// A block's open sessions carry the account their requests name, and a
// session's last request time is nil until it makes one.
func TestBlockSessionsAndSessionLast(t *testing.T) {
	s := open(t)
	bind(t, s, claim("old", 100, "startup", 0), claim("new", 100, "clear", time.Minute))
	if err := s.AddRequests([]usage.Request{paneReq("a", "new", "m", 1, 0, 0, 1, t0.Add(2*time.Minute))}); err != nil {
		t.Fatal(err)
	}
	got, err := s.BlockSessions("h", "block:b1")
	if err != nil || len(got) != 1 || got[0].Session != "new" || got[0].Account != "alpha" ||
		got[0].Provider != usage.Claude || !got[0].Start.Equal(t0.Add(time.Minute)) {
		t.Fatalf("sessions = %+v, %v; want only new, account alpha", got, err)
	}
	last, err := s.SessionLast(usage.Claude, "new")
	if err != nil || last == nil || !last.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("last = %v, %v", last, err)
	}
	if last, err := s.SessionLast(usage.Claude, "old"); err != nil || last != nil {
		t.Fatalf("old last = %v, %v; want nil", last, err)
	}
}
