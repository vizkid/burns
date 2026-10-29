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
	"syscall"
	"time"

	"github.com/vizkid/burns/internal/api"
	"github.com/vizkid/burns/internal/bindings"
	"github.com/vizkid/burns/internal/collect"
	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/rex"
	"github.com/vizkid/burns/internal/store"
	"github.com/vizkid/burns/internal/usage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	addr := flag.String("addr", "127.0.0.1:9766", "listen address")
	cfgPath := flag.String("config", filepath.Join(home, ".config", "burns", "accounts.json"), "accounts file")
	dbPath := flag.String("db", filepath.Join(stateDir(home), "smithers.db"), "SQLite store")
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
	if err := st.Sweep(time.Now()); err != nil {
		log.Printf("WARN sweep: %v", err)
	}
	go loop(ctx, &collect.Collector{Store: st, Config: cfg, Now: time.Now}, st)

	server := &api.Server{Store: st, Config: cfg, Host: *host, Now: time.Now}
	if *rexPath != "" {
		cli := rex.CLI{Path: *rexPath}
		server.Rex = cli
		go (&bindings.Tracker{Store: st, Rex: cli, Host: *host, Now: time.Now}).Run(ctx, *rexPath)
		log.Printf("checking bindings with %s", *rexPath)
	} else {
		log.Printf("WARN rex not found: bindings stay unverified and open")
	}
	if *tailscalePath != "" {
		go serveTailnet(ctx, *tailscalePath, server.ReadHandler())
	} else {
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
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("smithersd %s listening on %s, %d accounts", usage.Version, *addr, len(cfg.Accounts))
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// loop scans agent homes at the collector's adaptive interval and sweeps old
// rows once an hour.
func loop(ctx context.Context, c *collect.Collector, st *store.Store) {
	sweep := time.NewTicker(time.Hour)
	defer sweep.Stop()
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
		case <-time.After(c.Interval()):
		}
	}
}

func stateDir(home string) string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "burns")
	}
	return filepath.Join(home, ".local", "state", "burns")
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
