// Package rex reads the Rex block facts and events that end and check
// bindings. It calls only read-only Rex CLI commands.
package rex

import "encoding/json"

// Kind is the kind of event that can end a binding.
type Kind int

const (
	BlockClosed Kind = iota + 1
	Foreground
	SessionDestroyed
)

// Event is one Rex event that matters to bindings. For Foreground, Pid is 0
// when the block has no foreground process, and Ancestors run from the
// foreground's parent upward.
type Event struct {
	Kind      Kind
	Session   string
	Block     string
	Pid       int
	Ancestors []int
}

type wire struct {
	Type    string `json:"type"`
	Payload struct {
		SessionID string `json:"session_id"`
		Event     struct {
			Type    string `json:"type"`
			Payload struct {
				BlockID string `json:"block_id"`
				Name    string `json:"name"`
				Payload struct {
					Foreground *proc  `json:"foreground"`
					Ancestors  []proc `json:"ancestors"`
				} `json:"payload"`
			} `json:"payload"`
		} `json:"event"`
	} `json:"payload"`
}

type proc struct {
	Pid int `json:"pid"`
}

// ParseEvent decodes one line of `rex events --all --json`. It reports false
// for events bindings ignore and for lines that are not JSON.
func ParseEvent(line []byte) (Event, bool) {
	var w wire
	if json.Unmarshal(line, &w) != nil {
		return Event{}, false
	}
	p := w.Payload
	switch {
	case w.Type == "session_destroyed":
		return Event{Kind: SessionDestroyed, Session: p.SessionID}, true
	case w.Type != "session_event":
		return Event{}, false
	case p.Event.Type == "block_closed":
		return Event{Kind: BlockClosed, Session: p.SessionID, Block: p.Event.Payload.BlockID}, true
	case p.Event.Type == "block_event" && p.Event.Payload.Name == "process_changed":
		b := p.Event.Payload
		e := Event{Kind: Foreground, Session: p.SessionID, Block: b.BlockID}
		if fg := b.Payload.Foreground; fg != nil {
			e.Pid = fg.Pid
		}
		for _, a := range b.Payload.Ancestors {
			e.Ancestors = append(e.Ancestors, a.Pid)
		}
		return e, true
	}
	return Event{}, false
}
