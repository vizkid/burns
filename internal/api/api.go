// Package api serves smithersd's HTTP routes.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/store"
)

// Body limits.
const (
	MaxOTLP       = 4 << 20
	MaxStatusline = 8 << 10
	MaxBind       = 4 << 10
)

// HistoryWindow is how far back GET /usage/history reaches.
const HistoryWindow = 20 * time.Minute

// Server holds what the handlers need.
type Server struct {
	Store  *store.Store
	Config config.Config
	Host   string
	Now    func() time.Time
	Rex    Rex        // nil keeps every claim unverified
	Panes  PaneLister // nil means no pane can be confirmed live
	// ConfigPath is the accounts file that GET and PUT /accounts read and replace.
	ConfigPath string
	// Restart is called after a PUT /accounts write, so smithersd takes the new accounts.
	Restart func()
	// HomeExists reports whether a home is on this host; nil checks the disk.
	HomeExists func(dir string) bool
	// Home is the folder GET /dirs lists under; empty means this user's.
	Home       string
	accountsMu sync.Mutex
}

// Handler returns the route table for the loopback listener. Local tools send a
// loopback Host and no Origin; a web page cannot send both (loopbackOnly).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/logs", s.logs)
	mux.HandleFunc("POST /v1/metrics", s.metrics)
	mux.HandleFunc("POST /statusline", s.statusline)
	mux.HandleFunc("POST /bind", s.bind)
	mux.HandleFunc("GET /block-sessions", s.blockSessions)
	mux.HandleFunc("GET /session-last", s.sessionLast)
	s.reads(mux)
	return loopbackOnly(noBrowser(mux))
}

// RemoteHandler returns the routes for the tailnet listener: the reads and the
// account list, which either app copies to every host. Nothing reachable from
// another device writes usage or binds a session.
func (s *Server) RemoteHandler() http.Handler {
	mux := http.NewServeMux()
	s.reads(mux)
	return noBrowser(mux)
}

// loopbackOnly refuses a Host that does not name this machine. A DNS-rebinding
// page reaches 127.0.0.1 under its own name, and its Host still shows that name.
func loopbackOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func isLoopback(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
}

// noBrowser refuses a request with an Origin header. Only browsers send one, and
// no Burns caller is a web page. A cross-site POST with a text/plain body has no
// preflight to stop it.
func noBrowser(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Header.Values("Origin")) > 0 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (s *Server) reads(mux *http.ServeMux) {
	mux.HandleFunc("GET /usage", s.usage)
	mux.HandleFunc("GET /usage/readings", s.readings)
	mux.HandleFunc("GET /usage/history", s.history)
	mux.HandleFunc("GET /usage/sessions", s.sessions)
	mux.HandleFunc("GET /usage/panes", s.panes)
	mux.HandleFunc("GET /usage/cost", s.cost)
	mux.HandleFunc("GET /models", s.models)
	mux.HandleFunc("GET /dirs", s.dirs)
	mux.HandleFunc("GET /accounts", s.getAccounts)
	mux.HandleFunc("PUT /accounts", s.putAccounts)
}

// readBody reads at most limit bytes. It answers 413 and returns false when the body is larger.
func readBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return nil, false
	}
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return nil, false
	}
	return b, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// fail logs a store error and answers 500 without detail.
func fail(w http.ResponseWriter, what string, err error) {
	log.Printf("ERROR %s: %v", what, err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
