// Command smithersd collects Claude and Codex usage on one host and serves it.
// It reads agent output and files; it launches nothing and changes no agent configuration.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vizkid/burns/internal/api"
	"github.com/vizkid/burns/internal/billing"
	"github.com/vizkid/burns/internal/bindings"
	"github.com/vizkid/burns/internal/collect"
	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/rex"
	"github.com/vizkid/burns/internal/store"
	"github.com/vizkid/burns/internal/usage"
	"github.com/vizkid/rexkit/rexctl"
)

func main() {
	err := run()
	if errors.Is(err, errRestart) {
		err = restart()
	}
	if err != nil {
		log.Fatal(err)
	}
}

// errRestart ends run after PUT /accounts wrote a new accounts file.
var errRestart = errors.New("accounts changed: restarting")

// restart replaces this process with the same binary and arguments, so it loads the
// new accounts file. It launches no other program.
func restart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	log.Printf("accounts changed: restarting")
	return syscall.Exec(exe, os.Args, os.Environ())
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	addr := flag.String("addr", "127.0.0.1:9766", "listen address")
	cfgPath := flag.String("config", filepath.Join(config.Dir(home), "accounts.json"), "accounts file")
	dbPath := flag.String("db", filepath.Join(config.StateDir(home), "smithers.db"), "SQLite store")
	host := flag.String("host", shortHostname(), "host name reported to readers")
	rexPath := flag.String("rex", lookRex(), "rex CLI that checks and ends bindings; empty disables")
	tailscalePath := flag.String("tailscale", lookTailscale(), "tailscale CLI for the tailnet read listener; empty disables")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("smithersd " + usage.Version)
		return nil
	}

	cfg, err := config.Load(*cfgPath, home)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		return err
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go loop(ctx, &collect.Collector{Store: st, Config: cfg, Now: time.Now}, st)
	if home, err := os.UserHomeDir(); err == nil {
		go (&billing.Poller{Store: st, Config: cfg, Client: http.DefaultClient, URL: billing.UsageURL,
			Token:    billing.StoredToken(home, time.Now),
			CodexURL: billing.CodexUsageURL, CodexLogin: billing.StoredCodexLogin, Now: time.Now}).Run(ctx)
	}

	// A PUT /accounts write stops both listeners; run then returns errRestart once
	// the handlers on both have finished. Most writes arrive over the tailnet.
	var restarting atomic.Bool
	server := &api.Server{Store: st, Config: cfg, ConfigPath: *cfgPath, Host: *host, Now: time.Now,
		Restart: func() { restarting.Store(true); stop() }}
	if *rexPath != "" {
		cli := rex.CLI{Path: *rexPath}
		userHome, _ := os.UserHomeDir()
		ctl := rex.StartControl(ctx, rexctl.LocalEndpoint(os.Getenv, userHome), usage.Version)
		panes := &rex.PaneCache{List: ctl.Panes, Now: time.Now, MaxAge: 2 * time.Minute}
		server.Rex, server.Panes = cli, panes
		go panes.Run(ctx, 15*time.Second, 10*time.Second)
		go (&bindings.Tracker{Store: st, Rex: cli, Host: *host, Now: time.Now}).Run(ctx, *rexPath)
		go server.RunRoar(ctx, api.RoarDir(os.Getenv, userHome))
		log.Printf("checking bindings with %s", *rexPath)
	} else {
		log.Printf("WARN rex not found: bindings stay unverified and open")
	}
	tailnetDone := make(chan struct{})
	if *tailscalePath != "" {
		go func() {
			defer close(tailnetDone)
			serveTailnet(ctx, *tailscalePath, server.RemoteHandler())
		}()
	} else {
		close(tailnetDone)
		log.Printf("WARN tailscale not found: no tailnet listener")
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    8 << 10,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	log.Printf("smithersd %s listening on %s, %d accounts", usage.Version, *addr, len(cfg.Accounts))
	if err := serveUntil(ctx, srv); err != nil {
		return err
	}
	<-tailnetDone
	if restarting.Load() {
		return errRestart
	}
	return nil
}

// recentHours is how far back the minute's roll-up reaches.
const recentHours = 3 * time.Hour

// scanner reads agent homes; *collect.Collector is one.
type scanner interface {
	Scan() error
	Interval() time.Duration
}

// upkeep sweeps and rolls up the store; *store.Store is one.
type upkeep interface {
	Sweep(now time.Time) error
	RollUp(from, now time.Time) error
}

// loop sweeps and rolls up everything once, then scans agent homes at the
// collector's adaptive interval and sweeps old rows once an hour. The startup
// pass runs here, not before the listener: on a loaded host the full roll-up
// takes minutes, and smithersd must answer meanwhile. The single writer queues
// other writes behind it, and reads continue through WAL.
func loop(ctx context.Context, c scanner, st upkeep) {
	if err := st.Sweep(time.Now()); err != nil {
		log.Printf("WARN sweep: %v", err)
	}
	if err := st.RollUp(time.Time{}, time.Now()); err != nil {
		log.Printf("WARN roll up: %v", err)
	}
	sweep := time.NewTicker(time.Hour)
	defer sweep.Stop()
	rollup := time.NewTicker(time.Minute)
	defer rollup.Stop()
	rolled := time.Now()
	lastErr := ""
	for {
		if err := c.Scan(); err != nil && err.Error() != lastErr {
			log.Printf("WARN scan: %v", err)
			lastErr = err.Error()
		} else if err == nil {
			lastErr = ""
		}
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			if err := st.Sweep(time.Now()); err != nil {
				log.Printf("WARN sweep: %v", err)
			}
		case <-rollup.C:
			// The last few hours each minute. Each hour, everything: a late row can be days old.
			now := time.Now()
			from := now.Add(-recentHours)
			if now.Sub(rolled) >= time.Hour {
				from, rolled = time.Time{}, now
			}
			if err := st.RollUp(from, now); err != nil {
				log.Printf("WARN roll up: %v", err)
			}
		case <-time.After(c.Interval()):
		}
	}
}

// lookRex finds rex on PATH. launchd's PATH lacks it, so the install passes --rex.
func lookRex() string {
	p, _ := exec.LookPath("rex")
	return p
}

func shortHostname() string {
	h, _ := os.Hostname()
	return strings.SplitN(h, ".", 2)[0]
}
