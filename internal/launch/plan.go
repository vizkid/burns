// Package launch builds an agent launch on one host (docs/SPEC.md, "Launch").
// It runs inside the new Rex tab, so it can name that tab's block, and it ends
// by replacing itself with the CLI: a wrapper would stop roar delivery.
package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vizkid/burns/internal/config"
)

// endpoint is the local smithersd, for Claude's telemetry.
const endpoint = "http://localhost:9766"

// Side is what one tool declares for one provider.
type Side struct {
	ChannelServers []string `json:"channel_servers,omitempty"`
	MCP            []MCP    `json:"mcp,omitempty"`
	Hooks          []Hook   `json:"hooks,omitempty"`
}

// MCP is an MCP server a tool needs in every home.
type MCP struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// Hook is a command hook a tool needs in every home.
type Hook struct {
	Event   string `json:"event"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// Tool is one `tools.d` file.
type Tool struct {
	Claude Side `json:"claude"`
	Codex  Side `json:"codex"`
}

// Side returns the tool's declaration for a provider.
func (t Tool) Side(provider string) Side {
	if provider == "claude" {
		return t.Claude
	}
	return t.Codex
}

// LoadTools reads every `*.json` in dir. A bad file is reported and skipped: one
// tool's mistake must not stop a launch. A missing folder means no tools.
func LoadTools(dir string) ([]Tool, []error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, []error{err}
	}
	var tools []Tool
	var errs []error
	for _, path := range paths {
		var t Tool
		data, err := os.ReadFile(path)
		if err == nil {
			err = json.Unmarshal(data, &t)
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		tools = append(tools, t)
	}
	return tools, errs
}

// Channels is every tool's channel servers for a provider, sorted, each once.
func Channels(tools []Tool, provider string) []string {
	var out []string
	for _, t := range tools {
		out = append(out, t.Side(provider).ChannelServers...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Input is what one launch is built from.
type Input struct {
	Account  config.Account
	Home     string
	UserHome string
	Channels []string
	Block    string
	Session  string
}

// Plan is one launch: the program replaces this process.
type Plan struct {
	Provider string
	Program  string
	Args     []string
	Env      map[string]string
	Channels []string // Claude's channels, for the consent dialog
	Daemon   []string // command to run first, program included; empty means none
}

// Build turns an input into a plan. It reads and runs nothing.
func Build(in Input) Plan {
	l := config.Launch{}
	if in.Account.Launch != nil {
		l = *in.Account.Launch
	}
	var p Plan
	if in.Account.Provider == "claude" {
		p = claude(in, l)
	} else {
		p = codex(in, l)
	}
	p.Provider = in.Account.Provider
	p.Env["BURNS_ACCOUNT"] = in.Account.Name
	if c := l.Credential; c != nil {
		cop := []string{"run", "--account", c.Account, "--ref", c.Ref, "--env", c.Env, "--"}
		p.Args = append(append(cop[:len(cop):len(cop)], p.Program), p.Args...)
		if len(p.Daemon) > 0 {
			p.Daemon = append(append([]string{"cop"}, cop...), p.Daemon...)
		}
		p.Program = "cop"
	}
	return p
}

func claude(in Input, l config.Launch) Plan {
	p := Plan{Program: "claude", Env: map[string]string{
		"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
		"OTEL_METRICS_EXPORTER":        "otlp",
		"OTEL_LOGS_EXPORTER":           "otlp",
		"OTEL_EXPORTER_OTLP_PROTOCOL":  "http/json",
		"OTEL_EXPORTER_OTLP_ENDPOINT":  endpoint,
	}}
	// Claude files its login in the Keychain under this path, and the default
	// home's entry has none, so naming the default home would lose its login.
	if in.Home != filepath.Join(in.UserHome, ".claude") {
		p.Env["CLAUDE_CONFIG_DIR"] = in.Home
	}
	if in.Block != "" {
		p.Env["OTEL_RESOURCE_ATTRIBUTES"] = "rex.block=" + in.Block + ",rex.session=" + in.Session
	}
	p.Args = pairs("--model", l.Model, "--effort", l.Effort, "--permission-mode", l.PermissionMode)
	if len(in.Channels) > 0 {
		p.Channels = slices.Compact(slices.Sorted(slices.Values(in.Channels)))
		// The flag takes several values, so it comes last and each is its own argument.
		first := "--dangerously-load-development-channels=server:" + p.Channels[0]
		p.Args = append(p.Args, first)
		for _, s := range p.Channels[1:] {
			p.Args = append(p.Args, "server:"+s)
		}
	}
	return p
}

// codex passes only -m: any -c override, or -p, takes the TUI off the home's
// app-server daemon, and then roar cannot deliver natively.
func codex(in Input, l config.Launch) Plan {
	return Plan{
		Program: "codex",
		Env:     map[string]string{"CODEX_HOME": in.Home},
		Args:    pairs("-m", l.Model),
		Daemon:  []string{"codex", "app-server", "daemon", "start"},
	}
}

func pairs(kv ...string) []string {
	out := []string{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			out = append(out, kv[i], kv[i+1])
		}
	}
	return out
}

// Environ is base with the plan's variables replacing any of the same name.
func (p Plan) Environ(base []string) []string {
	out := make([]string, 0, len(base)+len(p.Env))
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if _, set := p.Env[name]; !set {
			out = append(out, kv)
		}
	}
	for k, v := range p.Env {
		out = append(out, k+"="+v)
	}
	return out
}
