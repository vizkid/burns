package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// A PUT /accounts arrives over the tailnet and calls for a restart while its own
// handler runs. The restart must wait for that handler, or the app loses the answer.
func TestServeUntilWaitsForHandlers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	srv := &http.Server{Addr: addr, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fmt.Fprint(w, "written")
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- serveUntil(ctx, srv) }()
	answer := make(chan string, 1)
	go func() {
		for range 100 {
			r, err := http.Get("http://" + addr + "/")
			if err != nil {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			answer <- string(b)
			return
		}
		answer <- "no connection"
	}()
	<-entered
	cancel()
	select {
	case <-served:
		t.Fatal("serveUntil returned while a handler ran")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-served; err != nil {
		t.Fatal(err)
	}
	if got := <-answer; got != "written" {
		t.Fatalf("answer = %q", got)
	}
}

func TestServeUntilReportsAListenFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := serveUntil(context.Background(), &http.Server{Addr: ln.Addr().String()}); err == nil {
		t.Fatal("want the bind error")
	}
}
