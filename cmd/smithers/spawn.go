package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vizkid/burns/internal/config"
	"github.com/vizkid/burns/internal/spawn"
	"github.com/vizkid/rexkit/rex"
)

// trustWatch is how long spawn reads a new Claude pane for its folder-trust
// dialog. The dialog is the first thing Claude shows, within about a second.
const trustWatch = 5 * time.Second

// spawnAgent starts an agent session in the caller's own Rex session, on this
// host or another (docs/SPEC.md, "Agent-started launches").
func spawnAgent(args []string, out, errOut io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	fs.SetOutput(errOut)
	provider := fs.String("provider", "", "claude or codex")
	account := fs.String("account", "", "account name")
	label := fs.String("label", "", "the new pane's label, which roar addresses")
	tab := fs.String("tab", "", "the new tab's label, after the account's emoji (not with --beside)")
	host := fs.String("host", "", "Rex host label; empty is this host")
	beside := fs.Bool("beside", false, "split the caller's own tab (this host only)")
	model := fs.String("model", "", "model, replacing the account's")
	dir := fs.String("dir", "", "folder, relative to the target host's home (default: this folder)")
	wait := fs.Duration("wait", 60*time.Second, "how long to wait for the agent to start")
	base := fs.String("url", DefaultURL, "smithersd address, for the quota warning")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	session, block := getenv("REX_SESSION"), getenv("REX_BLOCK")
	switch {
	case session == "" || block == "":
		fmt.Fprintln(errOut, "smithers spawn runs inside a Rex pane: REX_SESSION and REX_BLOCK are not set")
		return 1
	case *provider != "claude" && *provider != "codex":
		fmt.Fprintln(errOut, "--provider must be claude or codex")
		return 2
	case *account == "" || *label == "":
		fmt.Fprintln(errOut, "--account and --label are required")
		return 2
	case *tab == "" && !*beside:
		fmt.Fprintln(errOut, "--tab is required for a new tab: it names the tab after the account's emoji")
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if *dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		*dir = homeRelative(home, cwd)
	}
	warnQuota(*base, *provider, *account, errOut)
	bin := "rex"
	if d := getenv("REX_BIN_DIR"); d != "" {
		bin = filepath.Join(d, "rex")
	}
	req := spawn.Request{Provider: *provider, Account: *account, Model: *model, Label: *label, Tab: *tab, Dir: *dir,
		Emoji: accountEmoji(home, *provider, *account), Beside: *beside, CallerSession: session, CallerBlock: block,
		Wait: *wait, Every: time.Second, TrustWatch: trustWatch}
	ctx, cancel := context.WithTimeout(context.Background(), *wait+30*time.Second)
	defer cancel()
	res, err := spawn.Spawn(ctx, rex.Client{Bin: bin}, rex.Client{Bin: bin, Server: *host}, req)
	logSpawn(home, req, *host, res, err)
	return report(out, errOut, *host, res, err)
}

// report prints the new pane, or why there is none. Exit 3 means the pane
// exists and its agent has not started yet.
func report(out, errOut io.Writer, host string, res spawn.Result, err error) int {
	where := "this host"
	if host != "" {
		where = host
	}
	if res.Block == "" {
		fmt.Fprintln(errOut, "spawn:", err)
		return 1
	}
	addr := roarAddress(host, res.Label, res.SessionLabel)
	if err != nil {
		fmt.Fprintf(errOut, "spawn: pane %s (%s) opened on %s, but %v.\n", addr, res.Block, where, err)
		switch {
		case errors.Is(err, spawn.ErrTrust):
			fmt.Fprintln(errOut, "Ask the operator to answer it in that pane. Roar messages wait until then.")
			return 4
		case errors.Is(err, spawn.ErrNotStarted):
			fmt.Fprintln(errOut, "Look at the pane: it may show why.")
			return 3
		case errors.Is(err, spawn.ErrTrustUnchecked):
			fmt.Fprintln(errOut, "The agent runs. Look at the pane before relying on it.")
			return 5
		}
		return 1
	}
	created := ""
	if res.Created {
		created = ", in a new session"
	}
	fmt.Fprintf(out, "%s runs in pane %s on %s%s.\nblock %s\nroar address %s\n", res.Agent, res.Label, where, created, res.Block, addr)
	if res.Agent == "codex" {
		fmt.Fprintln(out, "A daemon-backed Codex gets typed roar messages until it runs roar itself.")
	}
	return 0
}

// homeRelative names cwd relative to home when it is inside it, since each
// host has its own home; anything else stays absolute.
func homeRelative(home, cwd string) string {
	rel, err := filepath.Rel(home, cwd)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return cwd
	}
	return rel
}

// launchDir is the folder `smithers launch --dir` changes into on its host.
func launchDir(home, dir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(home, dir)
}

// roarAddress is pane@session, with host: in front for another host.
func roarAddress(host, label, session string) string {
	if host != "" {
		return host + ":" + label + "@" + session
	}
	return label + "@" + session
}

func accountEmoji(home, provider, name string) string {
	cfg, err := config.Load(filepath.Join(config.Dir(home), "accounts.json"), home)
	if err != nil {
		return ""
	}
	a, _ := cfg.Find(provider, name)
	return a.Emoji
}

// warnQuota says so when the account's live window is spent or unknown. Viz,
// 2026-11-03: spawn still launches, since a reset or credits may be in hand.
func warnQuota(base, provider, name string, errOut io.Writer) {
	var u struct{ Accounts []account }
	if err := getJSON(base+"/usage", &u); err != nil {
		fmt.Fprintf(errOut, "warning: quota unknown, smithersd did not answer: %v\n", err)
		return
	}
	for _, a := range u.Accounts {
		if a.Provider != provider || a.Account != name {
			continue
		}
		// Azure has no quota windows by design; a subscription without any is unread here.
		if a.Kind != "azure" && !hasWindows(a) {
			fmt.Fprintf(errOut, "warning: %s %s's quota is unknown on this host; launching anyway\n", provider, name)
			return
		}
		for _, l := range a.Limits {
			for m, w := range l.Windows {
				if w.Percent == nil || *w.Percent >= 100 {
					minutes, _ := strconv.Atoi(m)
					fmt.Fprintf(errOut, "warning: %s %s's %s window is %s; launching anyway\n", provider, name, span(minutes), state(w.Percent))
				}
			}
		}
		return
	}
	fmt.Fprintf(errOut, "warning: smithersd knows no %s account named %s\n", provider, name)
}

func hasWindows(a account) bool {
	for _, l := range a.Limits {
		if len(l.Windows) > 0 {
			return true
		}
	}
	return false
}

func state(p *float64) string {
	if p == nil {
		return "unknown"
	}
	return fmt.Sprintf("at %.0f%%", *p)
}

// logSpawn records who started what in spawn.log, beside launch.log.
func logSpawn(home string, req spawn.Request, host string, res spawn.Result, err error) {
	f, ferr := os.OpenFile(filepath.Join(config.StateDir(home), "spawn.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if ferr != nil {
		return
	}
	defer f.Close()
	outcome := "ok"
	if err != nil {
		outcome = err.Error()
	}
	fmt.Fprintf(f, "%s caller=%s/%s host=%q session=%s block=%s label=%q %s/%s model=%q: %s\n",
		time.Now().Format(time.RFC3339), req.CallerSession, req.CallerBlock, host, res.SessionID, res.Block,
		req.Label, req.Provider, req.Account, req.Model, outcome)
}
