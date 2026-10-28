package claude

import (
	"bufio"
	"encoding/json"
	"io"
	"time"

	"github.com/vizkid/burns/internal/tail"
	"github.com/vizkid/burns/internal/usage"
)

// Transcript decodes transcript lines. Turn carries the last prompt ID
// across calls, so the tailer persists the decoder with the file cursor.
type Transcript struct {
	Account string `json:"account"`
	Turn    string `json:"turn"`
}

type transcriptLine struct {
	Type      string    `json:"type"`
	Subtype   string    `json:"subtype"`
	SessionID string    `json:"sessionId"`
	PromptID  string    `json:"promptId"`
	RequestID string    `json:"requestId"`
	AgentID   string    `json:"agentId"`
	Timestamp time.Time `json:"timestamp"`
	Message   *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
			Output     int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	CompactMetadata *struct {
		Trigger    string `json:"trigger"`
		PreTokens  *int64 `json:"preTokens"`
		PostTokens *int64 `json:"postTokens"`
		DurationMs *int64 `json:"durationMs"`
	} `json:"compactMetadata"`
}

// Lines decodes a batch of lines. Requests are deduplicated within the batch
// on requestId, falling back to message.id, keeping the largest total.
// The store deduplicates across batches and files with the same key.
func (d *Transcript) Lines(lines [][]byte) ([]usage.Request, []usage.Compaction) {
	var reqs []usage.Request
	var comps []usage.Compaction
	index := map[string]int{}
	for _, b := range lines {
		var l transcriptLine
		if json.Unmarshal(b, &l) != nil {
			continue
		}
		switch {
		case l.Type == "user" && l.PromptID != "":
			d.Turn = l.PromptID
		case l.Type == "system" && l.Subtype == "compact_boundary" && l.CompactMetadata != nil && l.SessionID != "":
			m := l.CompactMetadata
			comps = append(comps, usage.Compaction{Provider: usage.Claude, Account: d.Account, Session: l.SessionID,
				Turn: d.Turn, Trigger: m.Trigger, PreTokens: m.PreTokens, PostTokens: m.PostTokens,
				DurationMs: m.DurationMs, Source: usage.SourceTranscript, At: l.Timestamp})
		case l.Type == "assistant":
			r, ok := d.request(l)
			if !ok {
				continue
			}
			if i, seen := index[r.Key]; seen {
				if r.Total() > reqs[i].Total() {
					reqs[i] = r
				}
				continue
			}
			index[r.Key] = len(reqs)
			reqs = append(reqs, r)
		}
	}
	return reqs, comps
}

func (d *Transcript) request(l transcriptLine) (usage.Request, bool) {
	if l.Message == nil || l.Message.Usage == nil || l.SessionID == "" {
		return usage.Request{}, false
	}
	key := l.RequestID
	if key == "" {
		key = l.Message.ID
	}
	if key == "" {
		return usage.Request{}, false
	}
	u := l.Message.Usage
	return usage.Request{Provider: usage.Claude, Key: key, Account: d.Account, Session: l.SessionID,
		Agent: l.AgentID, Turn: d.Turn, Model: l.Message.Model, Input: u.Input, CachedInput: u.CacheRead,
		CacheWrite: u.CacheWrite, Output: u.Output, Source: usage.SourceTranscript, At: l.Timestamp}, true
}

// ScanTranscript reads one whole transcript.
func ScanTranscript(r io.Reader, account string) ([]usage.Request, []usage.Compaction, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	var lines [][]byte
	for {
		b, err := br.ReadBytes('\n')
		if len(b) > 0 && len(b) <= tail.MaxLine {
			lines = append(lines, b)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
	}
	d := &Transcript{Account: account}
	reqs, comps := d.Lines(lines)
	return reqs, comps, nil
}
