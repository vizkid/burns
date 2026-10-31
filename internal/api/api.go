// Package api serves smithersd's HTTP routes.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
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
	Rex    Rex // nil keeps every claim unverified
	// ConfigPath is the accounts file that GET and PUT /accounts read and replace.
	ConfigPath string
	// Restart is called after a PUT /accounts write, so smithersd takes the new accounts.
	Restart func()
	// HomeExists reports whether a home is on this host; nil checks the disk.
	HomeExists func(dir string) bool
	accountsMu sync.Mutex
}

// Handler returns the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/logs", s.logs)
	mux.HandleFunc("POST /v1/metrics", s.metrics)
	mux.HandleFunc("POST /statusline", s.statusline)
	mux.HandleFunc("POST /bind", s.bind)
	s.reads(mux)
	return mux
}

// RemoteHandler returns the routes for the tailnet listener: the reads and the
// account list, which either app copies to every host. Nothing reachable from
// another device writes usage or binds a session.
func (s *Server) RemoteHandler() http.Handler {
	mux := http.NewServeMux()
	s.reads(mux)
	return mux
}

func (s *Server) reads(mux *http.ServeMux) {
	mux.HandleFunc("GET /usage", s.usage)
	mux.HandleFunc("GET /usage/readings", s.readings)
	mux.HandleFunc("GET /usage/history", s.history)
	mux.HandleFunc("GET /usage/sessions", s.sessions)
	mux.HandleFunc("GET /models", s.models)
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
