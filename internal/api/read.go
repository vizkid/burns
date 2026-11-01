package api

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/vizkid/burns/internal/rex"
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
	SetUp    bool       `json:"set_up"`
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
		// An account whose home is missing here is not set up on this host: it lists no
		// limits, so no usage reads as unknown, never zero.
		doc := accountDoc{Provider: a.Provider, Account: a.Name, Kind: a.Kind, SetUp: s.homeExists(s.Config.HomeDir(a)), Limits: []limitDoc{}}
		for _, l := range limits {
			if !doc.SetUp {
				break
			}
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

// readings answers every change of the windows that have not reset, oldest first.
func (s *Server) readings(w http.ResponseWriter, r *http.Request) {
	readings, err := s.Store.Readings(s.Now())
	if err != nil {
		fail(w, "read readings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "readings": orEmpty(readings)})
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

// PaneLister reports the panes that exist now.
type PaneLister interface {
	Panes(ctx context.Context) (map[string]rex.Pane, error)
}

type livePane struct {
	store.Pane
	Session string `json:"session"`
	Label   string `json:"label"`
}

// panes answers the tokens of each pane, per model. Rex decides what is live
// because an open binding can outlive its pane. With no answer from Rex, checked
// is false and the rows are unconfirmed, so a client says the host is unknown.
func (s *Server) panes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.Panes()
	if err != nil {
		fail(w, "read panes", err)
		return
	}
	var live map[string]rex.Pane
	if s.Panes != nil {
		if live, err = s.Panes.Panes(r.Context()); err != nil {
			log.Printf("list panes: %v", err)
			live = nil
		}
	}
	panes := make([]livePane, 0, len(rows))
	for _, p := range rows {
		if live == nil {
			panes = append(panes, livePane{Pane: p})
		} else if pane, ok := live[p.Block]; ok {
			panes = append(panes, livePane{Pane: p, Session: pane.Session, Label: pane.Label})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": s.Host, "checked": live != nil, "panes": panes})
}

func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
