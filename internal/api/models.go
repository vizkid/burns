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
		over := catalog.Override{}
		if a.Launch != nil {
			over = catalog.Override{Model: a.Launch.Model, Effort: a.Launch.Effort}
		}
		if a.Provider == "claude" {
			m := seen[[2]string{a.Provider, a.Name}]
			entries = append(entries, catalog.Claude(home, a.Name, m.Models, m.Thread, over))
		} else {
			entries = append(entries, catalog.Codex(home, a.Name, over))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "accounts": entries})
}
