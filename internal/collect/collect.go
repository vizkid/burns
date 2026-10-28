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

// Collector reads Codex rollouts and Claude transcripts for configured accounts.
type Collector struct {
	Store  *store.Store
	Config config.Config
	Now    func() time.Time
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
	cur, state, found, err := c.Store.Cursor(path)
	if err != nil || (found && !tail.Changed(cur)) {
		return err
	}
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
