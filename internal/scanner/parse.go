package scanner

import (
	"bytes"
	"encoding/json"
	"net/url"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// rawLine is the on-disk JSONL shape we care about. Fields we don't use are
// intentionally absent so encoding/json skips them. All fields are pointers
// or omitempty-friendly so we can distinguish "missing" from "zero."
type rawLine struct {
	Type      string  `json:"type"`
	UUID      string  `json:"uuid"`
	SessionID string  `json:"sessionId"`
	Timestamp string  `json:"timestamp"`
	Message   *rawMsg `json:"message,omitempty"`
}

type rawMsg struct {
	Model string    `json:"model,omitempty"`
	Usage *rawUsage `json:"usage,omitempty"`
}

type rawUsage struct {
	InputTokens            int `json:"input_tokens"`
	OutputTokens           int `json:"output_tokens"`
	CacheCreationInputToks int `json:"cache_creation_input_tokens"`
	CacheReadInputToks     int `json:"cache_read_input_tokens"`
}

// parseLine parses one JSONL line into a contracts.Event. It returns ok=false
// for malformed input, missing required fields, or unparsable timestamps.
// Unknown fields are ignored. The project parameter is the URL-decoded
// parent directory name and is assigned to Event.Project.
func parseLine(b []byte, project string) (contracts.Event, bool) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return contracts.Event{}, false
	}

	var r rawLine
	if err := json.Unmarshal(b, &r); err != nil {
		return contracts.Event{}, false
	}
	if r.Type == "" || r.UUID == "" {
		return contracts.Event{}, false
	}

	ts, err := time.Parse(time.RFC3339Nano, r.Timestamp)
	if err != nil {
		return contracts.Event{}, false
	}

	ev := contracts.Event{
		UUID:      r.UUID,
		SessionID: r.SessionID,
		Timestamp: ts.UTC(),
		Type:      r.Type,
		Project:   project,
	}
	if r.Message != nil {
		ev.Model = r.Message.Model
		if r.Message.Usage != nil {
			ev.Usage = &contracts.Usage{
				InputTokens:       r.Message.Usage.InputTokens,
				OutputTokens:      r.Message.Usage.OutputTokens,
				CacheReadTokens:   r.Message.Usage.CacheReadInputToks,
				CacheCreateTokens: r.Message.Usage.CacheCreationInputToks,
			}
		}
	}
	return ev, true
}

// projectNameFromDir returns the human-readable project name for the given
// directory basename. Claude Code stores project directories with URL-encoded
// paths; if decoding fails, the raw name is returned unchanged.
func projectNameFromDir(base string) string {
	decoded, err := url.QueryUnescape(base)
	if err != nil {
		return base
	}
	return decoded
}
