// Package bindings ends Rex bindings from the host's Rex event stream
// (docs/SPEC.md, "Ending a binding").
package bindings

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/vizkid/burns/internal/rex"
	"github.com/vizkid/burns/internal/store"
)

// Backoff bounds for restarting the events stream.
const (
	MinBackoff = time.Second
	MaxBackoff = 30 * time.Second
)

// Rex reports a block's foreground pid, 0 when it has none.
type Rex interface {
	Foreground(ctx context.Context, block string) (int, error)
}

// Tracker applies Rex events to the host's bindings.
type Tracker struct {
	Store *store.Store
	Rex   Rex
	Host  string
	Now   func() time.Time
}

// Handle applies one event.
func (t *Tracker) Handle(e rex.Event) error {
	now := t.Now()
	switch e.Kind {
	case rex.BlockClosed:
		return t.Store.EndBlock(t.Host, e.Block, now)
	case rex.SessionDestroyed:
		return t.Store.EndRexSession(t.Host, e.Session, now)
	case rex.Foreground:
		return t.Store.ForegroundChanged(t.Host, e.Block, e.Pid, e.Ancestors, now)
	}
	return nil
}

// Reconcile catches up on events missed while the stream was down. It ends a
// block's bindings when Rex no longer knows the block or the block has no
// foreground. A different foreground is left alone: `process` gives no
// ancestors, so it cannot tell an editor the agent opened from a new program.
func (t *Tracker) Reconcile(ctx context.Context) error {
	blocks, err := t.Store.OpenBlocks(t.Host)
	if err != nil {
		return err
	}
	for _, b := range blocks {
		pid, err := t.Rex.Foreground(ctx, b)
		switch {
		case errors.Is(err, rex.ErrNoBlock), err == nil && pid == 0:
			if err := t.Store.EndBlock(t.Host, b, t.Now()); err != nil {
				return err
			}
		}
	}
	return nil
}

// Run reconciles, then follows `rex events` at path until ctx is done,
// reconciling again after each restart. It logs each new failure once.
func (t *Tracker) Run(ctx context.Context, path string) {
	backoff, lastErr := MinBackoff, ""
	for ctx.Err() == nil {
		started := time.Now()
		if err := t.Reconcile(ctx); err != nil {
			logOnce(&lastErr, "reconcile", err)
		}
		err := rex.Watch(ctx, path, func(e rex.Event) {
			if err := t.Handle(e); err != nil {
				logOnce(&lastErr, "binding event", err)
			}
		})
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("stream ended")
		}
		logOnce(&lastErr, "rex events", err)
		if time.Since(started) > MaxBackoff {
			backoff = MinBackoff // the stream was healthy; restart promptly
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, MaxBackoff)
	}
}

func logOnce(last *string, what string, err error) {
	if msg := what + ": " + err.Error(); msg != *last {
		log.Printf("WARN %s", msg)
		*last = msg
	}
}
