package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/vizkid/burns/internal/rex"
	"github.com/vizkid/burns/internal/usage"
)

// Rex reports a block's foreground pid, 0 when it has none.
type Rex interface {
	Foreground(ctx context.Context, block string) (int, error)
}

// CheckTimeout bounds the Rex call that checks a claim. The hook does not wait for it.
const CheckTimeout = 2 * time.Second

type bindRequest struct {
	Provider   string `json:"provider"`
	Session    string `json:"session"`
	RexSession string `json:"rex_session"`
	RexBlock   string `json:"rex_block"`
	Source     string `json:"source"`
	Pids       []int  `json:"pids"`
}

// errStale means Rex contradicts the claim: the hook is not in the block.
var errStale = errors.New("block foreground is not the hook's ancestor")

// bind records a SessionStart hook's claim that a session runs in a Rex block,
// after checking it with Rex (docs/SPEC.md, "Checking a claim"). It answers 202
// even for a dropped claim, because the hook can do nothing about it.
func (s *Server) bind(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r, MaxBind)
	if !ok {
		return
	}
	var b bindRequest
	if json.Unmarshal(body, &b) != nil || (b.Provider != usage.Claude && b.Provider != usage.Codex) ||
		b.Session == "" || b.RexBlock == "" {
		http.Error(w, "provider, session, and rex_block are required", http.StatusBadRequest)
		return
	}
	binding := usage.Binding{Host: s.Host, Block: b.RexBlock, RexSession: b.RexSession,
		Provider: b.Provider, Session: b.Session, Source: usage.SourceHook + ":" + b.Source, Start: s.Now()}
	switch pid, err := s.check(b); {
	case errors.Is(err, errStale):
		log.Printf("bind: dropped %s session %s in %s: %v", b.Provider, b.Session, b.RexBlock, err)
		w.WriteHeader(http.StatusAccepted)
		return
	case err == nil && pid != 0:
		binding.Verified, binding.FgPid = true, pid
	}
	if err := s.Store.Bind(binding); err != nil {
		fail(w, "store binding", err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// check returns the block's foreground pid when it is one of the hook's
// ancestors. It returns 0 and no error when the claim cannot be checked.
func (s *Server) check(b bindRequest) (int, error) {
	if s.Rex == nil || len(b.Pids) == 0 {
		return 0, nil
	}
	// Not the request's context: a hook that gives up must not cancel the check.
	ctx, cancel := context.WithTimeout(context.Background(), CheckTimeout)
	defer cancel()
	pid, err := s.Rex.Foreground(ctx, b.RexBlock)
	switch {
	case errors.Is(err, rex.ErrNoBlock):
		return 0, errStale
	case err != nil:
		return 0, nil // Rex is unavailable; keep the claim unverified
	case pid == 0 || !slices.Contains(b.Pids, pid):
		return 0, errStale
	}
	return pid, nil
}
