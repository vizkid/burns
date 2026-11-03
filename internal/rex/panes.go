package rex

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Pane is a block that exists now, with the names Rex shows for it.
type Pane struct {
	Session string
	Label   string
}

// PaneCache keeps the pane list in memory, so a reader costs no Rex calls.
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
