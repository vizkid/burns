package api

import (
	"net/http"

	"github.com/vizkid/burns/internal/catalog"
)

// models answers what each account can run on this host. It reads the CLIs'
// own files and runs nothing.
func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	seen, err := s.Store.SeenModels()
	if err != nil {
		fail(w, "read models", err)
		return
	}
	entries := []catalog.Entry{}
	for _, a := range s.Config.Accounts {
		home := s.Config.HomeDir(a)
		if a.Provider == "claude" {
			entries = append(entries, catalog.Claude(home, a.Name, seen[[2]string{a.Provider, a.Name}]))
		} else {
			entries = append(entries, catalog.Codex(home, a.Name))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "accounts": entries})
}
