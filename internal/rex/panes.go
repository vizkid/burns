package rex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Pane is a block that exists now, with the names Rex shows for it.
type Pane struct {
	Session string
	Label   string
}

// Panes lists every block on this server. Any failure fails the whole list: a
// partial one would call live panes dead.
func (c CLI) Panes(ctx context.Context) (map[string]Pane, error) {
	var sessions struct {
		Sessions []struct {
			ID    string `json:"session_id"`
			Label string `json:"label"`
		} `json:"sessions"`
	}
	if err := c.readJSON(ctx, &sessions, "session", "ls", "--json"); err != nil {
		return nil, err
	}
	panes := map[string]Pane{}
	for _, s := range sessions.Sessions {
		var blocks struct {
			Blocks []struct {
				ID    string `json:"block_id"`
				Label string `json:"label"`
			} `json:"blocks"`
		}
		if err := c.readJSON(ctx, &blocks, "block", "ls", "-s", s.ID, "--json", "--short=false"); err != nil {
			return nil, err
		}
		for _, b := range blocks.Blocks {
			panes[b.ID] = Pane{Session: s.Label, Label: b.Label}
		}
	}
	return panes, nil
}

func (c CLI) readJSON(ctx context.Context, v any, args ...string) error {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Path, args...)
	cmd.Env, cmd.Stderr = serverEnv(), &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("rex %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("rex %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// PaneCache keeps the pane list in memory, so a reader costs no Rex calls: one
// listing takes about 0.4 s and spawns a process per session.
type PaneCache struct {
	List   func(context.Context) (map[string]Pane, error)
	Now    func() time.Time
	MaxAge time.Duration // Older than this, the list counts as no answer.

	mu    sync.Mutex
	panes map[string]Pane
	at    time.Time
}

// Refresh replaces the list. A failure keeps the last one until MaxAge.
func (c *PaneCache) Refresh(ctx context.Context) {
	panes, err := c.List(ctx)
	if err != nil {
		log.Printf("list panes: %v", err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.panes, c.at = panes, c.Now()
}

// Panes returns the last list, or an error when it is missing or too old.
func (c *PaneCache) Panes(context.Context) (map[string]Pane, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.panes == nil || c.Now().Sub(c.at) > c.MaxAge {
		return nil, fmt.Errorf("no pane list newer than %s", c.MaxAge)
	}
	return c.panes, nil
}

// Run refreshes every interval until ctx ends.
func (c *PaneCache) Run(ctx context.Context, every, limit time.Duration) {
	for {
		call, cancel := context.WithTimeout(ctx, limit)
		c.Refresh(call)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}
