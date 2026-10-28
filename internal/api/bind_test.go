package api

import (
	"context"
	"errors"
	"testing"

	"github.com/vizkid/burns/internal/rex"
)

type rexFunc func(context.Context, string) (int, error)

func (f rexFunc) Foreground(ctx context.Context, block string) (int, error) { return f(ctx, block) }

func foreground(pid int, err error) rexFunc {
	return func(context.Context, string) (int, error) { return pid, err }
}

const claimBody = `{"provider":"claude","session":"s1","rex_session":"session:r1","rex_block":"block:b1","source":"startup","pids":[30,20,10]}`

func TestBindVerifiesForegroundAmongAncestors(t *testing.T) {
	ts, s := server(t)
	s.Rex = foreground(20, nil)
	if r := post(t, ts.URL+"/bind", claimBody); r.StatusCode != 202 {
		t.Fatalf("bind = %d", r.StatusCode)
	}
	bs, err := s.Store.Bindings("host-1")
	if err != nil || len(bs) != 1 || !bs[0].Verified || bs[0].FgPid != 20 || bs[0].Source != "hook:startup" {
		t.Fatalf("bindings = %+v, %v", bs, err)
	}
}

// A stale REX_BLOCK, such as an agent run by a daemon, must not bind the session.
func TestBindDropsClaimRexContradicts(t *testing.T) {
	ts, s := server(t)
	for _, pid := range []int{99, 0} {
		s.Rex = foreground(pid, nil)
		if r := post(t, ts.URL+"/bind", claimBody); r.StatusCode != 202 {
			t.Fatalf("bind = %d; the hook must never see a failure", r.StatusCode)
		}
	}
	if bs, err := s.Store.Bindings("host-1"); err != nil || len(bs) != 0 {
		t.Fatalf("bindings = %+v, %v; want none", bs, err)
	}
}

func TestBindKeepsUnverifiedClaimWhenRexFails(t *testing.T) {
	ts, s := server(t)
	s.Rex = foreground(0, errors.New("rex: server unavailable"))
	post(t, ts.URL+"/bind", claimBody)
	bs, err := s.Store.Bindings("host-1")
	if err != nil || len(bs) != 1 || bs[0].Verified || bs[0].FgPid != 0 {
		t.Fatalf("bindings = %+v, %v; want one unverified", bs, err)
	}
}

func TestBindWithoutPidsIsUnverified(t *testing.T) {
	ts, s := server(t)
	s.Rex = foreground(20, nil)
	post(t, ts.URL+"/bind", `{"provider":"codex","session":"s2","rex_block":"block:b1","source":"startup"}`)
	bs, err := s.Store.Bindings("host-1")
	if err != nil || len(bs) != 1 || bs[0].Verified {
		t.Fatalf("bindings = %+v, %v; want one unverified", bs, err)
	}
}

func TestBindDropsClaimForClosedBlock(t *testing.T) {
	ts, s := server(t)
	s.Rex = foreground(0, rex.ErrNoBlock)
	post(t, ts.URL+"/bind", claimBody)
	if bs, err := s.Store.Bindings("host-1"); err != nil || len(bs) != 0 {
		t.Fatalf("bindings = %+v, %v; want none", bs, err)
	}
}
