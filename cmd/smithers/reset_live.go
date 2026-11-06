package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/vizkid/burns/internal/codexd"
	"github.com/vizkid/burns/internal/config"
	rex "github.com/vizkid/rexkit/rex"
)

// typePause lets the agent's input box take the text before Enter submits it.
const typePause = 500 * time.Millisecond

// liveResetHost wires a reset to this host's Rex, smithersd and accounts file.
func liveResetHost(session string) (resetHost, error) {
	bin, _, err := find("rex", os.Environ())
	if err != nil {
		return resetHost{}, err
	}
	r := rex.Client{Bin: bin}
	return resetHost{
		blocks:  func(ctx context.Context) ([]rex.Block, error) { return r.Blocks(ctx, session) },
		process: func(ctx context.Context, block string) (rex.Process, error) { return r.Process(ctx, "", block) },
		typeIn: func(ctx context.Context, block, text string) error {
			if text != "" {
				if _, err := r.Run(ctx, "send", "-b", block, text); err != nil {
					return err
				}
				time.Sleep(typePause)
			}
			_, err := r.Run(ctx, "send", "-b", block, "\r")
			return err
		},
		sessions: func(_ context.Context, block string) ([]blockSession, error) {
			var doc struct{ Sessions []blockSession }
			err := getJSON(DefaultURL+"/block-sessions?block="+url.QueryEscape(block), &doc)
			return doc.Sessions, err
		},
		last: func(_ context.Context, provider, s string) (*time.Time, error) {
			var doc struct{ Last *time.Time }
			q := url.Values{"provider": {provider}, "session": {s}}
			err := getJSON(DefaultURL+"/session-last?"+q.Encode(), &doc)
			return doc.Last, err
		},
		home:     accountHome,
		newCodex: watchNewCodex,
		now:      time.Now,
		poll:     time.Second,
	}, nil
}

func accountHome(provider, name string) (string, error) {
	user, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cfg, err := config.Load(filepath.Join(config.Dir(user), "accounts.json"), user)
	if err != nil {
		return "", err
	}
	a, ok := cfg.Find(provider, name)
	if !ok {
		return "", os.ErrNotExist
	}
	return cfg.HomeDir(a), nil
}

// watchNewCodex listens on the home's daemon before clear types /new, so the new
// thread cannot start unseen, then binds the one thread started in the pane's
// folder. It skips the launch's check for other unbound panes: reset knows when
// it typed /new, and another pane's thread in the window makes two, which Pick refuses.
func watchNewCodex(ctx context.Context, l codexLaunch, clear func() error) (string, string, error) {
	conn, err := codexd.Dial(ctx, l.Home)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	if err := clear(); err != nil {
		return "", "", err
	}
	threads, complete := conn.Started(ctx, codexWindow)
	if !complete {
		return "", "the daemon connection ended before the window did", nil
	}
	t, why := codexd.Pick(threads, l.Dir)
	if why != "" {
		return "", why, nil
	}
	return t.Session(), liveBinder().bindThread(ctx, l, t), nil
}
