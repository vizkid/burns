// Command smithers is the helper agents call: it forwards statusline JSON and
// SessionStart hooks to the local smithersd. It never fails or slows the agent.
// `smithers usage` prints the local smithersd's quota and consumption, and
// `smithers panes` its live panes with their compactions.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)) }

const usageText = `usage:
  smithers statusline [--url URL] [-- COMMAND ARGS...]
  smithers hook session-start --provider claude|codex [--url URL]
  smithers usage [--since 24h] [--url URL]
  smithers panes [--url URL]
  smithers spawn --provider claude|codex --account NAME --label L --tab T [--host H] [--beside] [--anchor P] [--pair LIST] [--model M] [--dir D] [--wait 60s]
  smithers reset PANE --checkpoint PATH --task TEXT [--account NAME] [--wait 120s]
  smithers launch --provider claude|codex --account NAME [--model M] [--effort E] [--permission-mode P] [--dir D]
  smithers answer-channels --block BLOCK --servers a,b
  smithers bind-codex --home H --block B --session S --dir D --pid P
  smithers version`

func run(args []string, in io.Reader, out, errOut io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, usageText)
		return 2
	}
	switch args[0] {
	case "statusline":
		return statusline(args[1:], in, out, errOut)
	case "hook":
		if len(args) > 1 && args[1] == "session-start" {
			return sessionStart(args[2:], in, errOut, getenv)
		}
	case "usage":
		return printUsage(args[1:], out, errOut)
	case "spawn":
		return spawnAgent(args[1:], out, errOut, getenv)
	case "reset":
		return resetPane(args[1:], out, errOut, getenv)
	case "panes":
		return printPanes(args[1:], out, errOut)
	case "launch":
		return launchAgent(args[1:], in, errOut, getenv)
	case "answer-channels":
		return answerChannels(args[1:], errOut)
	case "bind-codex":
		return bindCodex(args[1:], errOut)
	case "version":
		fmt.Fprintln(out, version())
		return 0
	}
	fmt.Fprintln(errOut, usageText)
	return 2
}
