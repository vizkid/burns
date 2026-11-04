package rex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"

	kit "github.com/vizkid/rexkit/rex"
)

// ErrNoBlock means Rex does not know the block: it was closed.
var ErrNoBlock = errors.New("rex: block not found")

// CLI calls the rex command at Path, through rexkit, without this process's pane
// identity: inside a pane, REX_SESSION scopes block lookups to that pane's
// session, and blocks in other sessions read as not found.
type CLI struct{ Path string }

func (c CLI) kit() kit.Client {
	return kit.Client{Bin: c.Path, Env: kit.WithoutPane(os.Environ())}
}

// Foreground returns the pid of the block's foreground process, or 0 when it
// has none. It uses the read-only block `process` method.
func (c CLI) Foreground(ctx context.Context, block string) (int, error) {
	p, err := c.kit().Process(ctx, "", block)
	if err != nil {
		// rex exits 3 for a missing target; other failures say nothing about the block.
		if errors.Is(err, kit.ErrNotFound) {
			return 0, ErrNoBlock
		}
		return 0, fmt.Errorf("rex process %s: %w", block, err)
	}
	if p.Foreground == nil {
		return 0, nil
	}
	return p.Foreground.PID, nil
}

// ForegroundIn returns the agent the block's foreground runs ("" for none) and its folder.
func (c CLI) ForegroundIn(ctx context.Context, block string) (agent, cwd string, err error) {
	p, err := c.kit().Process(ctx, "", block)
	if err != nil {
		if errors.Is(err, kit.ErrNotFound) {
			return "", "", ErrNoBlock
		}
		return "", "", fmt.Errorf("rex process %s: %w", block, err)
	}
	if p.Foreground == nil {
		return "", "", nil
	}
	return p.Foreground.Agent(), p.Foreground.Cwd, nil
}

// Capture returns the block's screen and scrollback as text.
func (c CLI) Capture(ctx context.Context, block string) (string, error) {
	out, err := c.kit().Run(ctx, "capture", "-b", block, "--trim")
	return string(out), err
}

// Enter presses Enter in the block.
func (c CLI) Enter(ctx context.Context, block string) error {
	_, err := c.kit().Run(ctx, "send", "-b", block, "\r")
	return err
}

// Watch runs one `rex events --all --json` stream and calls fn for each event
// that matters to bindings. ready runs once the stream's process has started.
// It returns when the stream ends or ctx is done.
func Watch(ctx context.Context, path string, ready func(), fn func(Event)) error {
	cmd := CLI{Path: path}.kit().Command(ctx, "events", "--all", "--json")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	ready()
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		if e, ok := ParseEvent(sc.Bytes()); ok {
			fn(e)
		}
	}
	scanErr := sc.Err()
	if scanErr != nil {
		cmd.Process.Kill() // an unreadable stream would otherwise block Wait
	}
	if err := cmd.Wait(); err != nil && scanErr == nil {
		return err
	}
	return scanErr
}
