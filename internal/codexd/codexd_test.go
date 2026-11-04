package codexd

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPickOneTopLevelTUIThreadInTheFolder(t *testing.T) {
	tui := Thread{ID: "t1", SessionID: "t1", Cwd: "/w/a", Originator: "codex-tui"}
	cases := []struct {
		name    string
		threads []Thread
		want    string
		reason  string
	}{
		{"one match", []Thread{tui}, "t1", ""},
		{"none", nil, "", "no Codex thread started in /w/a"},
		{"other folder", []Thread{{ID: "t2", Cwd: "/w/b", Originator: "codex-tui"}}, "", "no Codex thread started in /w/a"},
		{"subagent", []Thread{{ID: "t3", Cwd: "/w/a", Originator: "codex-tui", Parent: "t0"}}, "", "no Codex thread started in /w/a"},
		{"not the TUI", []Thread{{ID: "t4", Cwd: "/w/a", Originator: "roar"}}, "", "no Codex thread started in /w/a"},
		{"two launches at once", []Thread{tui, {ID: "t5", Cwd: "/w/a", Originator: "codex-tui"}}, "", "2 Codex threads started in /w/a"},
		{"same thread twice", []Thread{tui, tui}, "t1", ""},
		{"match among others", []Thread{{ID: "t2", Cwd: "/w/b", Originator: "codex-tui"}, tui}, "t1", ""},
	}
	for _, c := range cases {
		got, reason := Pick(c.threads, "/w/a")
		if got.ID != c.want || reason != c.reason {
			t.Errorf("%s: got %q %q, want %q %q", c.name, got.ID, reason, c.want, c.reason)
		}
	}
}

func TestSessionFallsBackToTheThread(t *testing.T) {
	if s := (Thread{ID: "t1"}).Session(); s != "t1" {
		t.Errorf("session %q", s)
	}
	if s := (Thread{ID: "t1", SessionID: "s1"}).Session(); s != "s1" {
		t.Errorf("session %q", s)
	}
}

// fakeDaemon serves the daemon's control socket the way Codex lays it out:
// a link in the home to a socket in a short private folder.
func fakeDaemon(t *testing.T, serve func(*websocket.Conn)) string {
	t.Helper()
	short, err := os.MkdirTemp("/tmp", "cxd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(short) })
	os.Chmod(short, 0o700)
	sock := filepath.Join(short, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	up := websocket.Upgrader{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		var init map[string]any
		if c.ReadJSON(&init) != nil || init["method"] != "initialize" {
			return
		}
		c.WriteJSON(map[string]any{"id": init["id"], "result": map[string]any{}})
		serve(c)
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "app-server-control"), 0o700)
	if err := os.Symlink(sock, Socket(home)); err != nil {
		t.Fatal(err)
	}
	return home
}

func started(id, cwd string) map[string]any {
	return map[string]any{"method": "thread/started", "params": map[string]any{"thread": map[string]any{
		"id": id, "sessionId": id, "cwd": cwd, "originator": "codex-tui", "parentThreadId": nil}}}
}

func TestStartedCollectsThreadsUntilTheWindowEnds(t *testing.T) {
	home := fakeDaemon(t, func(c *websocket.Conn) {
		var initialized map[string]any
		c.ReadJSON(&initialized)
		c.WriteJSON(map[string]any{"method": "account/updated", "params": map[string]any{}})
		c.WriteJSON(started("t1", "/w/a"))
		c.WriteJSON(map[string]any{"method": "turn/started", "params": map[string]any{"threadId": "t1"}})
		c.WriteJSON(started("t2", "/w/b"))
		time.Sleep(time.Second)
	})
	ctx := context.Background()
	conn, err := Dial(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	threads := conn.Started(ctx, 300*time.Millisecond)
	if len(threads) != 2 || threads[0].ID != "t1" || threads[1].Cwd != "/w/b" || threads[0].Originator != "codex-tui" {
		t.Fatalf("threads %+v", threads)
	}
}

func TestStartedEndsWhenTheDaemonGoes(t *testing.T) {
	home := fakeDaemon(t, func(c *websocket.Conn) {
		var initialized map[string]any
		c.ReadJSON(&initialized)
		c.WriteJSON(started("t1", "/w/a"))
	})
	conn, err := Dial(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	begin := time.Now()
	threads := conn.Started(context.Background(), 5*time.Second)
	if len(threads) != 1 || time.Since(begin) > 3*time.Second {
		t.Fatalf("threads %+v after %v", threads, time.Since(begin))
	}
}

func TestDialRefusesASocketOthersCanWriteTo(t *testing.T) {
	home := fakeDaemon(t, func(*websocket.Conn) {})
	real, _ := filepath.EvalSymlinks(Socket(home))
	os.Chmod(filepath.Dir(real), 0o777)
	defer os.Chmod(filepath.Dir(real), 0o700)
	if _, err := Dial(context.Background(), home); err == nil {
		t.Fatal("dialled a socket in a folder others can write to")
	}
}

func TestDialFailsWithoutADaemon(t *testing.T) {
	if _, err := Dial(context.Background(), t.TempDir()); err == nil {
		t.Fatal("dialled a home with no daemon")
	}
}
