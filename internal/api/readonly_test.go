package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The tailnet listener serves the read routes and the account list only: nothing
// reachable from another device can write usage, bind a session, or post telemetry.
func TestRemoteHandlerServesReadsAndAccounts(t *testing.T) {
	_, s := server(t)
	ts := httptest.NewServer(s.RemoteHandler())
	t.Cleanup(ts.Close)

	for _, path := range []string{"/usage", "/usage/readings", "/usage/history", "/usage/sessions"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d; want 200", path, resp.StatusCode)
		}
	}
	// The account routes are there: a PUT without If-Match is refused, not missing.
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/accounts", strings.NewReader("{}"))
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusPreconditionRequired {
		t.Errorf("PUT /accounts = %v %v; want 428", resp, err)
	}
	for _, path := range []string{"/bind", "/statusline", "/v1/logs", "/v1/metrics"} {
		resp, err := http.Post(ts.URL+path, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s = %d; want 404", path, resp.StatusCode)
		}
	}
}
