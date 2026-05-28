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

// parseOutcome classifies the result of parsing one JSONL line.
type parseOutcome int

const (
	// parseEvent means the line yielded a usable usage/lifecycle event.
	parseEvent parseOutcome = iota
	// parseIgnore means the line was valid JSON but not an event ccx tracks
	// (blank lines and records such as queue-operation, last-prompt, or
	// summary that carry no uuid/timestamp). Expected; skipped quietly.
	parseIgnore
	// parseMalformed means the line was not valid JSON and could not be read.
	parseMalformed
)

// String renders the outcome for log and test messages.
func (o parseOutcome) String() string {
	switch o {
	case parseEvent:
		return "parseEvent"
	case parseIgnore:
		return "parseIgnore"
	case parseMalformed:
		return "parseMalformed"
	default:
		return "parseUnknown"
	}
}

// parseLine parses one JSONL line into a contracts.Event. It returns
// parseEvent with the event on success, parseMalformed when the bytes are not
// valid JSON, and parseIgnore for valid-but-untracked records (blank lines or
// records lacking the type/uuid/timestamp of a usage event). Unknown fields
// are ignored. The project parameter is the URL-decoded parent directory name
// and is assigned to Event.Project.
func parseLine(b []byte, project string) (contracts.Event, parseOutcome) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return contracts.Event{}, parseIgnore
	}

	var r rawLine
	if err := json.Unmarshal(b, &r); err != nil {
		return contracts.Event{}, parseMalformed
	}
	if r.Type == "" || r.UUID == "" {
		// Valid JSON, but a record type ccx does not track (e.g.
		// queue-operation, last-prompt, summary). Skip without warning.
		return contracts.Event{}, parseIgnore
	}

	ts, err := time.Parse(time.RFC3339Nano, r.Timestamp)
	if err != nil {
		// Identifiable event record (has type+uuid) with an unreadable
		// timestamp — genuinely malformed, worth surfacing.
		return contracts.Event{}, parseMalformed
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
	return ev, parseEvent
}

// projectNameFromDir returns the human-readable project name for the given
// directory basename. Claude Code stores project directories with path-escaped
// paths; if decoding fails, the raw name is returned unchanged.
func projectNameFromDir(base string) string {
	decoded, err := url.PathUnescape(base)
	if err != nil {
		return base
	}
	return decoded
}
