package api

import "net/http"

// blockSessions answers a block's open bindings, for `smithers reset`
// (docs/SPEC.md, "Resetting a pane"). Loopback only.
func (s *Server) blockSessions(w http.ResponseWriter, r *http.Request) {
	block := r.URL.Query().Get("block")
	if block == "" {
		http.Error(w, "block is required", http.StatusBadRequest)
		return
	}
	sessions, err := s.Store.BlockSessions(s.Host, block)
	if err != nil {
		fail(w, "read block sessions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": orEmpty(sessions)})
}

// sessionLast answers when a session last made a request, or null.
func (s *Server) sessionLast(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("provider") == "" || q.Get("session") == "" {
		http.Error(w, "provider and session are required", http.StatusBadRequest)
		return
	}
	last, err := s.Store.SessionLast(q.Get("provider"), q.Get("session"))
	if err != nil {
		fail(w, "read session last", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"last": last})
}
