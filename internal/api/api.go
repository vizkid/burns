// Package api serves smithersd's HTTP routes.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
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
}

// Handler returns the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/logs", s.logs)
	mux.HandleFunc("POST /v1/metrics", s.metrics)
	mux.HandleFunc("POST /statusline", s.statusline)
	mux.HandleFunc("POST /bind", s.bind)
	mux.HandleFunc("GET /usage", s.usage)
	mux.HandleFunc("GET /usage/history", s.history)
	mux.HandleFunc("GET /usage/sessions", s.sessions)
	return mux
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
