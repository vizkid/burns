package store

import (
	"testing"

	"github.com/vizkid/burns/internal/usage"
)

func accountOf(t *testing.T, s *Store, table, where string, args ...any) string {
	t.Helper()
	var a string
	if err := s.db.QueryRow(`SELECT account FROM `+table+` WHERE `+where, args...).Scan(&a); err != nil {
		t.Fatal(err)
	}
	return a
}

func otelReq(key string) usage.Request {
	r := req(key, usage.SourceOTel, 5)
	r.Account = ""
	return r
}

// OTel names no account: a request takes it from its session's transcript rows.
func TestOTelRequestTakesSessionAccount(t *testing.T) {
	s := open(t)
	if err := s.AddRequests([]usage.Request{req("t1", usage.SourceTranscript, 3)}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRequests([]usage.Request{otelReq("o1")}); err != nil {
		t.Fatal(err)
	}
	if a := accountOf(t, s, "request", "key = 'o1'"); a != "personal" {
		t.Fatalf("account = %q; want personal from the session", a)
	}
}

// OTel can arrive before the transcript is read. The session's unattributed
// rows get the account when the transcript's rows arrive, even for a request
// the transcript never records, such as the compaction request.
func TestTranscriptAttributesEarlierOTelRows(t *testing.T) {
	s := open(t)
	comp := usage.Compaction{Provider: usage.Claude, Session: "s1", Turn: "p1", Trigger: "manual", Source: usage.SourceOTel, At: t0}
	if err := s.AddRequests([]usage.Request{otelReq("compact-req")}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCompactions([]usage.Compaction{comp}); err != nil {
		t.Fatal(err)
	}
	if a := accountOf(t, s, "request", "key = 'compact-req'"); a != "" {
		t.Fatalf("account = %q before the transcript; want unattributed", a)
	}
	if err := s.AddRequests([]usage.Request{req("t1", usage.SourceTranscript, 3)}); err != nil {
		t.Fatal(err)
	}
	if a := accountOf(t, s, "request", "key = 'compact-req'"); a != "personal" {
		t.Fatalf("request account = %q; want personal", a)
	}
	if a := accountOf(t, s, "compaction", "session = 's1'"); a != "personal" {
		t.Fatalf("compaction account = %q; want personal", a)
	}
}

// A compaction from OTel takes the account of a session already known.
func TestOTelCompactionTakesSessionAccount(t *testing.T) {
	s := open(t)
	s.AddRequests([]usage.Request{req("t1", usage.SourceTranscript, 3)})
	comp := usage.Compaction{Provider: usage.Claude, Session: "s1", Turn: "p1", Trigger: "auto", Source: usage.SourceOTel, At: t0}
	if err := s.AddCompactions([]usage.Compaction{comp}); err != nil {
		t.Fatal(err)
	}
	if a := accountOf(t, s, "compaction", "session = 's1'"); a != "personal" {
		t.Fatalf("account = %q; want personal", a)
	}
}

// Another session's account never leaks in.
func TestOtherSessionStaysUnattributed(t *testing.T) {
	s := open(t)
	s.AddRequests([]usage.Request{req("t1", usage.SourceTranscript, 3)})
	r := otelReq("o2")
	r.Session = "s2"
	s.AddRequests([]usage.Request{r})
	if a := accountOf(t, s, "request", "key = 'o2'"); a != "" {
		t.Fatalf("account = %q; want unattributed", a)
	}
}
