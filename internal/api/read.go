package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/vizkid/burns/internal/store"
)

type windowDoc struct {
	Percent    *float64  `json:"percent"`
	State      string    `json:"state"`
	ResetsAt   time.Time `json:"resets_at"`
	ObservedAt time.Time `json:"observed_at"`
}

type limitDoc struct {
	LimitID string               `json:"limit_id"`
	Windows map[string]windowDoc `json:"windows"`
	Credits *store.Credits       `json:"credits"`
}

type accountDoc struct {
	Provider string     `json:"provider"`
	Account  string     `json:"account"`
	Kind     string     `json:"kind"`
	Limits   []limitDoc `json:"limits"`
}

// usage lists every configured account, in settings order, with its limits.
// A window whose reset has passed shows percent null and state "unknown" (rule 9).
func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	now := s.Now()
	limits, err := s.Store.Limits()
	if err != nil {
		fail(w, "read limits", err)
		return
	}
	accounts := make([]accountDoc, 0, len(s.Config.Accounts))
	for _, a := range s.Config.Accounts {
		doc := accountDoc{Provider: a.Provider, Account: a.Name, Kind: a.Kind, Limits: []limitDoc{}}
		for _, l := range limits {
			if l.Provider != a.Provider || l.Account != a.Name {
				continue
			}
			ld := limitDoc{LimitID: l.LimitID, Windows: map[string]windowDoc{}, Credits: l.Credits}
			for m, win := range l.Windows {
				wd := windowDoc{State: "unknown", ResetsAt: win.ResetsAt, ObservedAt: win.ObservedAt}
				if v, ok := win.Value(now); ok {
					wd.Percent, wd.State = &v, "live"
				}
				ld.Windows[strconv.Itoa(m)] = wd
			}
			doc.Limits = append(doc.Limits, ld)
		}
		accounts = append(accounts, doc)
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "observed_at": now, "accounts": accounts})
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	now := s.Now()
	buckets, err := s.Store.History(now.Add(-HistoryWindow), now.Add(time.Nanosecond))
	if err != nil {
		fail(w, "read history", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "buckets": orEmpty(buckets)})
}

// sessions answers per-session and per-block totals, or one session's turns
// when provider and session are given. since is a Go duration, default 24h,
// clamped to retention.
func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if session := q.Get("session"); session != "" {
		turns, err := s.Store.Turns(q.Get("provider"), session)
		if err != nil {
			fail(w, "read turns", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "turns": orEmpty(turns)})
		return
	}
	window := 24 * time.Hour
	if v := q.Get("since"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			http.Error(w, "since must be a positive duration such as 24h", http.StatusBadRequest)
			return
		}
		window = min(d, store.Retention)
	}
	since := s.Now().Add(-window)
	sessions, err := s.Store.Sessions(since)
	if err != nil {
		fail(w, "read sessions", err)
		return
	}
	blocks, err := s.Store.Blocks(since)
	if err != nil {
		fail(w, "read blocks", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "sessions": orEmpty(sessions), "blocks": orEmpty(blocks)})
}

func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
