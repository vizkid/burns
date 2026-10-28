// Package collect scans agent homes and feeds new lines into the store.
package collect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/store"
	"github.com/vizkid/burns/internal/tail"
)

// Scan intervals. The loop scans fast while files change and slowly when idle,
// so an idle laptop wakes every 30 seconds, not every 2.
const (
	FastScan   = 2 * time.Second
	SlowScan   = 30 * time.Second
	ActiveSpan = 2 * time.Minute
)

// Collector reads Codex rollouts and Claude transcripts for configured accounts.
// It caches each file's cursor in memory, so an unchanged file costs one stat.
type Collector struct {
	Store  *store.Store
	Config config.Config
	Now    func() time.Time

	// LastChange is when a scan last found new bytes.
	LastChange time.Time

	cache       map[string]cached
	cursorLoads int
}

type cached struct {
	cur   tail.Cursor
	state json.RawMessage
}

// Interval is how long to wait before the next scan.
func (c *Collector) Interval() time.Duration {
	if !c.LastChange.IsZero() && c.Now().Sub(c.LastChange) < ActiveSpan {
		return FastScan
	}
	return SlowScan
}

// decoder turns a batch of lines into store writes. Its state is persisted
// with the file cursor as JSON.
type decoder interface {
	setAccount(name string)
	ingest(st *store.Store, lines [][]byte, now time.Time) error
}

// Scan reads every changed file once. Files untouched for longer than
// store.Retention are skipped: their rows would be swept anyway.
// One unreadable file does not stop the scan; the first error is returned.
func (c *Collector) Scan() error {
	var first error
	for _, a := range c.Config.Accounts {
		root, pattern := filepath.Join(c.Config.HomeDir(a), "projects"), "*.jsonl"
		if a.Provider == "codex" {
			root, pattern = filepath.Join(c.Config.HomeDir(a), "sessions"), "rollout-*.jsonl"
		}
		files, err := tail.Find(root, pattern)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil && first == nil {
			first = err
		}
		for _, f := range files {
			if err := c.file(a, f); err != nil && first == nil {
				first = fmt.Errorf("%s: %w", f, err)
			}
		}
	}
	return first
}

func (c *Collector) file(a config.Account, path string) error {
	now := c.Now()
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.ModTime().Before(now.Add(-store.Retention)) {
		return nil
	}
	if c.cache == nil {
		c.cache = map[string]cached{}
	}
	entry, ok := c.cache[path]
	if !ok {
		cur, state, found, err := c.Store.Cursor(path)
		if err != nil {
			return err
		}
		c.cursorLoads++
		entry = cached{cur: cur, state: state}
		c.cache[path] = entry
		if !found {
			entry.cur = tail.Cursor{Path: path}
		}
	}
	if entry.cur.Inode != 0 && !tail.ChangedInfo(entry.cur, info) {
		return nil
	}
	cur, state := entry.cur, entry.state
	defer func() { c.cache[path] = cached{cur: cur, state: state} }()
	for {
		lines, next, reset, err := tail.Read(cur)
		if err != nil {
			return err
		}
		dec, err := restore(a, state, reset)
		if err != nil {
			return err
		}
		if err := dec.ingest(c.Store, lines, now); err != nil {
			return err
		}
		if state, err = json.Marshal(dec); err != nil {
			return err
		}
		if err := c.Store.SaveCursor(next, state); err != nil {
			return err
		}
		if next.Offset != cur.Offset || reset {
			c.LastChange = now
		}
		if cur = next; !tail.Changed(cur) {
			return nil
		}
	}
}

// restore rebuilds a file's decoder. A reset file starts with fresh state.
func restore(a config.Account, state json.RawMessage, reset bool) (decoder, error) {
	var d decoder = &claudeFile{}
	if a.Provider == "codex" {
		d = &codexFile{}
	}
	if len(state) > 0 && !reset {
		if err := json.Unmarshal(state, d); err != nil {
			return nil, err
		}
	}
	d.setAccount(a.Name)
	return d, nil
}
