package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/vizkid/burns/internal/claude"
	"github.com/vizkid/burns/internal/otlp"
	"github.com/vizkid/burns/internal/quota"
	"github.com/vizkid/burns/internal/usage"
)

// claudeLimit is the limit ID of Claude's subscription windows.
const claudeLimit = "claude"

// logs stores api_request and compaction events from Claude Code.
func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r, MaxOTLP)
	if !ok {
		return
	}
	evs, err := otlp.ParseLogs(body)
	if err != nil {
		// Answer 200 so the exporter does not retry a body it will never fix.
		log.Printf("WARN otlp logs: %v", err)
		writeJSON(w, http.StatusOK, struct{}{})
		return
	}
	reqs, comps := claude.FromEvents(evs)
	if err := s.Store.AddRequests(reqs); err != nil {
		fail(w, "store requests", err)
		return
	}
	if err := s.Store.AddCompactions(comps); err != nil {
		fail(w, "store compactions", err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

// metrics accepts and drops token metrics: the api_request log event covers them.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	if _, ok := readBody(w, r, MaxOTLP); ok {
		writeJSON(w, http.StatusOK, struct{}{})
	}
}

// statusline stores Claude quota windows. The account comes from the
// transcript path; a post from an unconfigured home is accepted and dropped.
func (s *Server) statusline(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r, MaxStatusline)
	if !ok {
		return
	}
	now := s.Now()
	st, err := claude.ParseStatusline(body, now)
	if err != nil {
		http.Error(w, "bad statusline JSON", http.StatusBadRequest)
		return
	}
	acct, ok := s.Config.ForPath(st.TranscriptPath)
	if !ok || acct.Provider != usage.Claude || len(st.Windows) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if err := s.Store.ApplyReport(acct.Name, claudeLimit, quota.Report{Provider: usage.Claude, Windows: st.Windows}, now); err != nil {
		fail(w, "store statusline", err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type bindRequest struct {
	Provider   string `json:"provider"`
	Session    string `json:"session"`
	RexSession string `json:"rex_session"`
	RexBlock   string `json:"rex_block"`
	Source     string `json:"source"`
}

// bind records a SessionStart hook's claim that a session runs in a Rex block.
func (s *Server) bind(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r, MaxBind)
	if !ok {
		return
	}
	var b bindRequest
	if json.Unmarshal(body, &b) != nil || (b.Provider != usage.Claude && b.Provider != usage.Codex) ||
		b.Session == "" || b.RexBlock == "" {
		http.Error(w, "provider, session, and rex_block are required", http.StatusBadRequest)
		return
	}
	err := s.Store.Bind(usage.Binding{Host: s.Host, Block: b.RexBlock, RexSession: b.RexSession,
		Provider: b.Provider, Session: b.Session, Source: usage.SourceHook + ":" + b.Source, Start: s.Now()})
	if err != nil {
		fail(w, "store binding", err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
