package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os/exec"
	"time"

	"github.com/vizkid/burns/internal/usage"
)

// DefaultURL is smithersd's local address.
const DefaultURL = "http://127.0.0.1:9766"

// Timeouts keep a down smithersd from slowing the agent.
const (
	statuslineTimeout = 100 * time.Millisecond
	hookTimeout       = 500 * time.Millisecond
	maxInput          = 64 << 10
)

func version() string { return "smithers " + usage.Version }

func post(url string, body []byte, timeout time.Duration) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
}

// statusline posts stdin to smithersd, then runs the wrapped statusline
// command with the same stdin and passes its output through.
func statusline(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("statusline", flag.ContinueOnError)
	fs.SetOutput(errOut)
	url := fs.String("url", DefaultURL, "smithersd address")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	body, _ := io.ReadAll(io.LimitReader(in, maxInput))
	done := make(chan struct{})
	go func() { post(*url+"/statusline", body, statuslineTimeout); close(done) }()
	code := 0
	if cmd := fs.Args(); len(cmd) > 0 {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Stdin, c.Stdout, c.Stderr = bytes.NewReader(body), out, errOut
		if err := c.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = 1
			}
		}
	}
	<-done
	return code
}

// sessionStart posts the agent session and the Rex block it runs in.
// Outside Rex it does nothing. It prints nothing and always exits 0:
// Claude adds SessionStart stdout to the conversation.
func sessionStart(args []string, in io.Reader, errOut io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("session-start", flag.ContinueOnError)
	fs.SetOutput(errOut)
	url := fs.String("url", DefaultURL, "smithersd address")
	provider := fs.String("provider", "", "claude or codex")
	if err := fs.Parse(args); err != nil {
		return 0
	}
	block := getenv("REX_BLOCK")
	if block == "" {
		return 0
	}
	var hook struct {
		SessionID string `json:"session_id"`
		Source    string `json:"source"`
	}
	raw, _ := io.ReadAll(io.LimitReader(in, maxInput))
	if json.Unmarshal(raw, &hook) != nil || hook.SessionID == "" {
		return 0
	}
	body, _ := json.Marshal(map[string]any{"provider": *provider, "session": hook.SessionID,
		"source": hook.Source, "rex_block": block, "rex_session": getenv("REX_SESSION"), "pids": ancestors()})
	post(*url+"/bind", body, hookTimeout)
	return 0
}
