package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vizkid/burns/internal/usage"
)

// RoarEvery is how often smithersd reads roar's Codex bindings.
const RoarEvery = 60 * time.Second

// roarBinding is one of roar's codex/<block>.json files (roar's docs/SPEC.md, "Binding").
type roarBinding struct {
	Thread  string    `json:"thread"`
	Session string    `json:"session"`
	Pid     int       `json:"pid"`
	At      time.Time `json:"at"`
}

// RoarDir is roar's Codex binding folder: $ROAR_HOME/codex, else under
// $XDG_STATE_HOME/roar, else ~/.local/state/roar.
func RoarDir(getenv func(string) string, userHome string) string {
	if d := getenv("ROAR_HOME"); d != "" {
		return filepath.Join(d, "codex")
	}
	if d := getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "roar", "codex")
	}
	return filepath.Join(userHome, ".local", "state", "roar", "codex")
}

// RunRoar binds roar's Codex bindings at start and every RoarEvery until ctx ends.
func (s *Server) RunRoar(ctx context.Context, dir string) {
	t := time.NewTicker(RoarEvery)
	defer t.Stop()
	for {
		s.SyncRoar(dir)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// SyncRoar binds each of roar's Codex bindings that Rex confirms and that is
// newer than its block's newest open binding (docs/SPEC.md, "Bindings from
// roar"). It returns how many it bound. A file it skips is not logged: a file
// outlives its Codex until roar deletes it.
func (s *Server) SyncRoar(dir string) int {
	paths, _ := filepath.Glob(filepath.Join(dir, "block:*.json"))
	bound := 0
	for _, p := range paths {
		var f roarBinding
		data, err := os.ReadFile(p)
		if err != nil || json.Unmarshal(data, &f) != nil || f.Thread == "" || f.Pid == 0 {
			continue
		}
		block := strings.TrimSuffix(filepath.Base(p), ".json")
		if !s.roarNewer(block, f) {
			continue
		}
		// Rex failing gives pid 0 here, so the file is skipped, never stored unverified.
		pid, err := s.check(bindRequest{RexBlock: block, Pids: []int{f.Pid}})
		if err != nil || pid == 0 {
			continue
		}
		b := usage.Binding{Host: s.Host, Block: block, RexSession: f.Session, Provider: usage.Codex,
			Session: f.Thread, Source: "roar", Verified: true, FgPid: pid, Start: f.At}
		if s.Store.Bind(b) == nil {
			bound++
		}
	}
	return bound
}

// roarNewer reports whether the file is newer than every open binding of the
// block and names a session the block does not have open.
func (s *Server) roarNewer(block string, f roarBinding) bool {
	open, err := s.Store.BlockSessions(s.Host, block)
	if err != nil {
		return false
	}
	for _, o := range open {
		if o.Session == f.Thread || !f.At.After(o.Start) {
			return false
		}
	}
	return true
}
