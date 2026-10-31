// Command smithers is the helper agents call: it forwards statusline JSON and
// SessionStart hooks to the local smithersd. It never fails or slows the agent.
// `smithers usage` prints the local smithersd's quota and consumption.
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
  smithers launch --provider claude|codex --account NAME [--model M] [--effort E] [--permission-mode P]
  smithers answer-channels --block BLOCK --servers a,b
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
	case "launch":
		return launchAgent(args[1:], in, errOut, getenv)
	case "answer-channels":
		return answerChannels(args[1:], errOut)
	case "version":
		fmt.Fprintln(out, version())
		return 0
	}
	fmt.Fprintln(errOut, usageText)
	return 2
}
