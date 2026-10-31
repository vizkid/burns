package launch

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// State is what a pane's screen shows.
type State int

const (
	// Nothing means no dialog Burns knows.
	Nothing State = iota
	// Answer is the channels dialog, listing exactly the servers Burns asked for.
	Answer
	// Foreign is a channels dialog Burns must not answer.
	Foreign
	// Trust is Claude's folder-trust dialog, which only the operator answers.
	Trust
)

const (
	dialogHead = "WARNING: Loading development channels"
	dialogYes  = "I am using this for local development"
	trustLine  = "trust this folder"
)

// Read classifies a screen against the servers Burns put in the flag. Only the
// exact list, with the cursor on the consent line, is Burns's to answer.
func Read(screen string, want []string) State {
	if !strings.Contains(screen, dialogHead) {
		if strings.Contains(screen, trustLine) {
			return Trust
		}
		return Nothing
	}
	sorted := slices.Sorted(slices.Values(want))
	if slices.Equal(listed(screen), sorted) && onConsent(screen) {
		return Answer
	}
	return Foreign
}

// listed reads the "Channels:" line and any lines it wrapped onto.
func listed(screen string) []string {
	var out []string
	inList := false
	for _, line := range strings.Split(screen, "\n") {
		fields := strings.Fields(strings.Trim(line, " │|"))
		if len(fields) > 0 && fields[0] == "Channels:" {
			inList, fields = true, fields[1:]
		}
		if !inList {
			continue
		}
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "server:") {
			break
		}
		for _, f := range fields {
			out = append(out, strings.TrimPrefix(f, "server:"))
		}
	}
	slices.Sort(out)
	return out
}

func onConsent(screen string) bool {
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "❯") && strings.Contains(line, dialogYes) {
			return true
		}
	}
	return false
}

// Pane is the block a launch watches.
type Pane interface {
	Capture(ctx context.Context, block string) (string, error)
	Enter(ctx context.Context, block string) error
}

// WatchTiming sets a watch's clock. Wait is how long to look for the dialog,
// Limit bounds the whole watch.
type WatchTiming struct {
	Wait, Step, Limit time.Duration
	Now               func() time.Time
	Sleep             func(time.Duration)
}

// Timing is a launch's watch: 60 seconds for the dialog, restarted while the
// trust dialog waits for the operator, who may be asleep.
func Timing() WatchTiming {
	return WatchTiming{Wait: time.Minute, Step: time.Second, Limit: 24 * time.Hour, Now: time.Now, Sleep: time.Sleep}
}

// Watch looks at the pane until the channels dialog settles, presses Enter only
// on an exact match, and returns one line for the log.
func Watch(ctx context.Context, p Pane, block string, want []string, t WatchTiming) string {
	start := t.Now()
	deadline := start.Add(t.Wait)
	for t.Now().Before(deadline) && t.Now().Before(start.Add(t.Limit)) {
		screen, err := p.Capture(ctx, block)
		if err == nil {
			switch Read(screen, want) {
			case Answer:
				if err := p.Enter(ctx, block); err != nil {
					return fmt.Sprintf("%s: channels dialog: %v", block, err)
				}
				return fmt.Sprintf("%s: answered channels dialog for %s", block, strings.Join(want, " "))
			case Foreign:
				return fmt.Sprintf("%s: channels dialog left for the operator: it does not list exactly %s", block, strings.Join(want, " "))
			case Trust:
				deadline = t.Now().Add(t.Wait)
			}
		}
		t.Sleep(t.Step)
	}
	return fmt.Sprintf("%s: no dialog to answer", block)
}
