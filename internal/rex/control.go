package rex

import (
	"context"
	"fmt"
	"time"

	"github.com/vizkid/rexkit/backoff"
	"github.com/vizkid/rexkit/rexctl"
)

// Control lists panes over one Rex control connection that stays up, so a
// refresh starts no process. Before, each refresh ran `rex session ls` and one
// `rex block ls` per session, and under load those processes were killed.
type Control struct{ s *rexctl.Session }

// StartControl starts connecting to the Rex server at endpoint and returns at once.
// The connection lives until ctx ends.
func StartControl(ctx context.Context, endpoint, version string) *Control {
	return &Control{s: rexctl.Start(ctx, rexctl.SessionOptions{
		Options:     rexctl.Options{Endpoint: endpoint, Info: rexctl.Info{Kind: "other", Name: "smithersd", Version: version}},
		Backoff:     backoff.Backoff{Initial: time.Second, Factor: 2, Max: 30 * time.Second, Jitter: 0.2, ResetAfter: time.Minute},
		DialTimeout: 5 * time.Second,
		// Rex answers session.list_blocks only on a connection attached to that
		// session. Attaching to the whole server covers sessions made later too.
		// The "session" scope leaves out block events, the busiest kind.
		Setup: func(ctx context.Context, c *rexctl.Client) error {
			return c.Call(ctx, "session.attach_all", map[string]string{"event_scope": "session"}, nil)
		},
	})}
}

// Panes lists every block on the server with its session's label and its own.
// Any failure fails the whole list: a partial one would call live panes dead.
func (c *Control) Panes(ctx context.Context) (map[string]Pane, error) {
	cl, err := c.s.Client(ctx)
	if err != nil {
		return nil, fmt.Errorf("rex control: %w", err)
	}
	var sessions struct {
		Sessions []struct {
			ID    string `json:"session_id"`
			Label string `json:"label"`
		} `json:"sessions"`
	}
	if err := cl.Call(ctx, "session.list", struct{}{}, &sessions); err != nil {
		return nil, fmt.Errorf("rex session.list: %w", err)
	}
	panes := map[string]Pane{}
	for _, s := range sessions.Sessions {
		var blocks struct {
			Blocks []struct {
				ID    *string `json:"block_id"`
				Label string  `json:"label"`
			} `json:"blocks"`
		}
		if err := cl.Call(ctx, "session.list_blocks", map[string]string{"session_id": s.ID}, &blocks); err != nil {
			return nil, fmt.Errorf("rex session.list_blocks %s: %w", s.ID, err)
		}
		for _, b := range blocks.Blocks {
			if b.ID != nil {
				panes[*b.ID] = Pane{Session: s.Label, Label: b.Label}
			}
		}
	}
	return panes, nil
}
