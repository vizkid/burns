package api

import (
	"context"
	"errors"
	"testing"

	"github.com/vizkid/burns/internal/rex"
	"github.com/vizkid/burns/internal/usage"
)

type fakePanes struct {
	panes map[string]rex.Pane
	err   error
}

func (f fakePanes) Panes(context.Context) (map[string]rex.Pane, error) { return f.panes, f.err }

type panesDoc struct {
	Checked bool `json:"checked"`
	Panes   []struct {
		Block   string `json:"block"`
		Session string `json:"session"`
		Label   string `json:"label"`
	} `json:"panes"`
}

func TestPanesRouteKeepsLivePanesAndNamesThem(t *testing.T) {
	ts, s := server(t)
	seedPanes(t, s)
	s.Panes = fakePanes{panes: map[string]rex.Pane{"block:live": {Session: "work", Label: "cap"}}}
	var doc panesDoc
	getJSON(t, ts.URL+"/usage/panes", &doc)
	if !doc.Checked || len(doc.Panes) != 1 || doc.Panes[0].Block != "block:live" || doc.Panes[0].Session != "work" || doc.Panes[0].Label != "cap" {
		t.Fatalf("doc = %+v", doc)
	}
}

func TestPanesRouteSaysWhenRexDidNotAnswer(t *testing.T) {
	ts, s := server(t)
	seedPanes(t, s)
	s.Panes = fakePanes{err: errors.New("no rex")}
	var doc panesDoc
	getJSON(t, ts.URL+"/usage/panes", &doc)
	if doc.Checked {
		t.Fatalf("doc = %+v; want checked false", doc)
	}
}

func seedPanes(t *testing.T, s *Server) {
	t.Helper()
	for _, block := range []string{"block:live", "block:dead"} {
		b := usage.Binding{Host: "h", Block: block, RexSession: "session:r", Provider: usage.Claude, Session: block, Source: "startup", Start: s.Now().Add(-1e9)}
		if err := s.Store.Bind(b); err != nil {
			t.Fatal(err)
		}
		r := usage.Request{Provider: usage.Claude, Key: block, Account: "alpha", Session: block, Turn: "p", Model: "claude-opus-5-5", Output: 5, Source: "test", At: s.Now()}
		if err := s.Store.AddRequests([]usage.Request{r}); err != nil {
			t.Fatal(err)
		}
	}
}
