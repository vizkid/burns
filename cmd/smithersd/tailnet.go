package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"time"

	"github.com/vizkid/burns/internal/tailnet"
)

// Tailnet listener settings (docs/SPEC.md, "Tailnet listener").
const (
	tailnetPort  = "9766"
	tailnetRetry = 30 * time.Second
	whoisTTL     = 5 * time.Minute
)

// serveTailnet serves reads on this node's tailnet address until ctx ends. While
// Tailscale is down, or the listener fails, it tries again every 30 seconds.
func serveTailnet(ctx context.Context, path string, reads http.Handler) {
	cli := tailnet.CLI{Path: path}
	lastErr := ""
	for {
		err := serveTailnetOnce(ctx, cli, reads)
		if ctx.Err() != nil {
			return
		}
		if err != nil && err.Error() != lastErr {
			log.Printf("WARN tailnet listener: %v; retrying every %s", err, tailnetRetry)
			lastErr = err.Error()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(tailnetRetry):
		}
	}
}

func serveTailnetOnce(ctx context.Context, cli tailnet.CLI, reads http.Handler) error {
	lookup, cancel := context.WithTimeout(ctx, 10*time.Second)
	ip, login, err := cli.Self(lookup)
	cancel()
	if err != nil {
		return err
	}
	gate := &tailnet.Gate{Whois: cli, Login: login, TTL: whoisTTL, Now: time.Now}
	addr := net.JoinHostPort(ip, tailnetPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           gate.Wrap(reads),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    8 << 10,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	stop := context.AfterFunc(ctx, func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	})
	defer stop()
	log.Printf("tailnet listener on %s for %s", addr, login)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// lookTailscale finds tailscale on PATH. launchd's PATH lacks it, so the install passes --tailscale.
func lookTailscale() string {
	p, _ := exec.LookPath("tailscale")
	return p
}
