// Package codexd listens to a Codex home's shared App Server daemon for the
// threads its clients start (docs/SPEC.md, "Binding a Codex launch to its
// pane"). It only listens: it starts no thread and sends no turn.
package codexd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Socket is the control socket of a Codex home's daemon.
func Socket(home string) string {
	return filepath.Join(home, "app-server-control", "app-server-control.sock")
}

// DialTimeout bounds connecting and the initialize call.
const DialTimeout = 3 * time.Second

// maxFrame bounds one notification. A thread/started carries the thread, not its turns.
const maxFrame = 4 << 20

// Thread is the part of a started thread that names where it runs.
type Thread struct {
	ID         string `json:"id"`
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	Parent     string `json:"parentThreadId"`
	Originator string `json:"originator"`
}

// Session is the ID Burns' usage rows carry: Codex's session, else the thread.
func (t Thread) Session() string {
	if t.SessionID != "" {
		return t.SessionID
	}
	return t.ID
}

// tuiOriginator marks a thread the Codex TUI started, tested on 0.166.0.
const tuiOriginator = "codex-tui"

// Pick returns the one top-level TUI thread started in dir, or why there is
// none. Two such threads mean two launches at once, and neither is bound.
func Pick(threads []Thread, dir string) (Thread, string) {
	var found []Thread
	for _, t := range threads {
		if t.Parent != "" || t.Originator != tuiOriginator || !SameDir(t.Cwd, dir) {
			continue
		}
		if len(found) == 0 || found[0].ID != t.ID {
			found = append(found, t)
		}
	}
	switch len(found) {
	case 1:
		return found[0], ""
	case 0:
		return Thread{}, "no Codex thread started in " + dir
	default:
		return Thread{}, fmt.Sprintf("%d Codex threads started in %s", len(found), dir)
	}
}

// SameDir compares folders through their links: /tmp is /private/tmp on macOS.
func SameDir(a, b string) bool {
	if a == b {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// Conn is one listening connection to the daemon.
type Conn struct{ ws *websocket.Conn }

// Dial connects and initializes. It refuses a socket another user owns or can
// replace, because whoever owns it would learn the launch.
func Dial(ctx context.Context, home string) (*Conn, error) {
	// Codex links the socket to a short path under /tmp. Check the real one.
	real, err := filepath.EvalSymlinks(Socket(home))
	if err != nil {
		return nil, err
	}
	if err := private(real, os.ModeSocket); err != nil {
		return nil, err
	}
	if err := private(filepath.Dir(real), os.ModeDir); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, DialTimeout)
	defer cancel()
	d := websocket.Dialer{NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", real)
	}}
	ws, resp, err := d.DialContext(ctx, "ws://localhost/", nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(maxFrame)
	c := &Conn{ws: ws}
	if err := c.initialize(ctx); err != nil {
		ws.Close()
		return nil, err
	}
	return c, nil
}

func (c *Conn) initialize(ctx context.Context) error {
	deadline, _ := ctx.Deadline()
	c.ws.SetWriteDeadline(deadline)
	c.ws.SetReadDeadline(deadline)
	defer c.ws.SetReadDeadline(time.Time{})
	if err := c.ws.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]any{"name": "burns", "version": "1"}}}); err != nil {
		return err
	}
	for {
		var m struct {
			ID    *int `json:"id"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := c.ws.ReadJSON(&m); err != nil {
			return err
		}
		if m.ID == nil || *m.ID != 1 {
			continue
		}
		if m.Error != nil {
			return errors.New("initialize: " + m.Error.Message)
		}
		return c.ws.WriteJSON(map[string]any{"jsonrpc": "2.0", "method": "initialized"})
	}
}

// Started collects every thread/started for the window. complete is false when the
// connection failed before the window ended: a thread may then have started unseen, so the
// caller must not treat the list as everything that started.
func (c *Conn) Started(ctx context.Context, window time.Duration) (threads []Thread, complete bool) {
	end := time.Now().Add(window)
	cut := false
	if d, ok := ctx.Deadline(); ok && d.Before(end) {
		end, cut = d, true
	}
	c.ws.SetReadDeadline(end)
	for {
		var m struct {
			Method string `json:"method"`
			Params struct {
				Thread *Thread `json:"thread"`
			} `json:"params"`
		}
		if err := c.ws.ReadJSON(&m); err != nil {
			// A frame of another shape is skipped. Any other error ends the wait: the
			// connection reads nothing more. Only the window's own deadline completes it.
			var syntax *json.SyntaxError
			var shape *json.UnmarshalTypeError
			if errors.As(err, &syntax) || errors.As(err, &shape) {
				continue
			}
			var ne net.Error
			return threads, !cut && errors.As(err, &ne) && ne.Timeout() && !time.Now().Before(end)
		}
		if m.Method == "thread/started" && m.Params.Thread != nil {
			threads = append(threads, *m.Params.Thread)
		}
	}
}

// Close ends the connection.
func (c *Conn) Close() error { return c.ws.Close() }

func private(path string, kind os.FileMode) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	switch {
	case fi.Mode().Type() != kind:
		return fmt.Errorf("%s is not a %v", path, kind)
	case !ok || int(st.Uid) != os.Getuid():
		return fmt.Errorf("%s is not owned by this user", path)
	case fi.Mode().Perm()&0o022 != 0:
		return fmt.Errorf("%s is writable by others", path)
	}
	return nil
}
