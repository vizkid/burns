package rex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrNoBlock means Rex does not know the block: it was closed.
var ErrNoBlock = errors.New("rex: block not found")

// CLI calls the rex command at Path.
type CLI struct{ Path string }

// Foreground returns the pid of the block's foreground process, or 0 when it
// has none. It uses the read-only block `process` method.
func (c CLI) Foreground(ctx context.Context, block string) (int, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Path, "block", "call", "--block", block, "process")
	cmd.Env, cmd.Stderr = serverEnv(), &stderr
	out, err := cmd.Output()
	if err != nil {
		// rex exits 3 for a missing target; other failures say nothing about the block.
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 3 && strings.Contains(stderr.String(), "not found") {
			return 0, ErrNoBlock
		}
		return 0, fmt.Errorf("rex process %s: %w: %s", block, err, strings.TrimSpace(stderr.String()))
	}
	var p struct {
		Foreground *proc `json:"foreground"`
	}
	if err := json.Unmarshal(out, &p); err != nil {
		return 0, fmt.Errorf("rex process %s: %w", block, err)
	}
	if p.Foreground == nil {
		return 0, nil
	}
	return p.Foreground.Pid, nil
}

// Watch runs one `rex events --all --json` stream and calls fn for each event
// that matters to bindings. It returns when the stream ends or ctx is done.
func Watch(ctx context.Context, path string, fn func(Event)) error {
	cmd := exec.CommandContext(ctx, path, "events", "--all", "--json")
	cmd.Env = serverEnv()
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
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

// serverEnv is this process's environment without a pane's Rex identity.
// Inside a pane, REX_SESSION scopes block lookups to that pane's session, and
// blocks in other sessions read as not found.
func serverEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "REX_SESSION=") && !strings.HasPrefix(kv, "REX_BLOCK=") {
			env = append(env, kv)
		}
	}
	return env
}
