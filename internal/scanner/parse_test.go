package scanner

import (
	"testing"
	"time"
)

func TestParseLineAssistantUsage(t *testing.T) {
	line := []byte(`{"type":"assistant","uuid":"u-1","sessionId":"s-1","timestamp":"2026-05-19T12:00:01Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":200}}}`)

	ev, outcome := parseLine(line, "my-project")
	if outcome != parseEvent {
		t.Fatalf("parseLine returned outcome=%v for valid assistant event", outcome)
	}
	if ev.Type != "assistant" {
		t.Errorf("Type = %q want %q", ev.Type, "assistant")
	}
	if ev.UUID != "u-1" {
		t.Errorf("UUID = %q want %q", ev.UUID, "u-1")
	}
	if ev.SessionID != "s-1" {
		t.Errorf("SessionID = %q want %q", ev.SessionID, "s-1")
	}
	if ev.Project != "my-project" {
		t.Errorf("Project = %q want %q", ev.Project, "my-project")
	}
	if ev.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q want %q", ev.Model, "claude-opus-4-7")
	}
	want := time.Date(2026, 5, 19, 12, 0, 1, 0, time.UTC)
	if !ev.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v want %v", ev.Timestamp, want)
	}
	if ev.Usage == nil {
		t.Fatalf("Usage is nil; want non-nil")
	}
	if ev.Usage.InputTokens != 100 || ev.Usage.OutputTokens != 50 ||
		ev.Usage.CacheCreateTokens != 10 || ev.Usage.CacheReadTokens != 200 {
		t.Errorf("Usage = %+v want {100 50 200 10}", *ev.Usage)
	}
}

func TestParseLineUserNoUsage(t *testing.T) {
	line := []byte(`{"type":"user","uuid":"u-2","sessionId":"s-1","timestamp":"2026-05-19T12:00:00Z","message":{"content":[{"type":"text","text":"hi"}]}}`)

	ev, outcome := parseLine(line, "proj")
	if outcome != parseEvent {
		t.Fatalf("parseLine returned outcome=%v for valid user event", outcome)
	}
	if ev.Type != "user" {
		t.Errorf("Type = %q want user", ev.Type)
	}
	if ev.Usage != nil {
		t.Errorf("Usage should be nil for user event, got %+v", *ev.Usage)
	}
	if ev.Model != "" {
		t.Errorf("Model should be empty for user event, got %q", ev.Model)
	}
}

func TestParseLineFlagsBrokenJSON(t *testing.T) {
	cases := [][]byte{
		[]byte(`not json at all`),
		[]byte(`{`),
		[]byte(`{"type":"assistant"`),
		[]byte(`{"type":123}`),
	}
	for i, c := range cases {
		if _, outcome := parseLine(c, "p"); outcome != parseMalformed {
			t.Errorf("case %d: broken JSON got outcome=%v, want parseMalformed for %q", i, outcome, c)
		}
	}
}

// TestParseLineIgnoresNonEventRecords pins the fix for the scanner-noise bug:
// valid JSON records that Claude Code writes but that carry no usage event
// (queue-operation, last-prompt, summary, blank lines) must be classified as
// parseIgnore, never parseMalformed, so they are skipped without a WARN.
func TestParseLineIgnoresNonEventRecords(t *testing.T) {
	cases := [][]byte{
		[]byte(``),
		[]byte(`   `),
		[]byte(`{"type":"queue-operation","operation":"enqueue","timestamp":"2026-04-22T15:57:07.677Z","sessionId":"s-1","content":"hi"}`),
		[]byte(`{"type":"last-prompt","lastPrompt":"hi","sessionId":"s-1"}`),
		[]byte(`{"type":"summary","summary":"x","leafUuid":"l-1"}`),
	}
	for i, c := range cases {
		if _, outcome := parseLine(c, "p"); outcome == parseMalformed {
			t.Errorf("case %d: non-event record wrongly flagged malformed: %q", i, c)
		}
		if _, outcome := parseLine(c, "p"); outcome == parseEvent {
			t.Errorf("case %d: non-event record wrongly parsed as event: %q", i, c)
		}
	}
}

func TestParseLineRejectsMissingUUID(t *testing.T) {
	line := []byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`)
	if _, outcome := parseLine(line, "p"); outcome == parseEvent {
		t.Errorf("parseLine returned parseEvent for event with no uuid")
	}
}

func TestParseLineRejectsBadTimestamp(t *testing.T) {
	line := []byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"not-a-time"}`)
	if _, outcome := parseLine(line, "p"); outcome == parseEvent {
		t.Errorf("parseLine returned parseEvent for event with bad timestamp")
	}
}

func TestParseLineIgnoresUnknownFields(t *testing.T) {
	line := []byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"2026-05-19T12:00:00Z","cwd":"/x","gitBranch":"main","parentUuid":"p","extraFutureField":42}`)
	if _, outcome := parseLine(line, "p"); outcome != parseEvent {
		t.Errorf("parseLine returned outcome=%v; unknown fields should be ignored", outcome)
	}
}

func TestProjectNameFromDirURLDecoded(t *testing.T) {
	got := projectNameFromDir("-Users-arafa-Developer-ccx")
	if got != "-Users-arafa-Developer-ccx" {
		t.Errorf("projectNameFromDir(plain) = %q want unchanged", got)
	}
	got = projectNameFromDir("home%2Fuser%2Fproj")
	if got != "home/user/proj" {
		t.Errorf("projectNameFromDir(encoded) = %q want decoded", got)
	}
	got = projectNameFromDir("my+project")
	if got != "my+project" {
		t.Errorf("projectNameFromDir(literal plus) = %q want plus preserved", got)
	}
	got = projectNameFromDir("C%2B%2B")
	if got != "C++" {
		t.Errorf("projectNameFromDir(encoded plus) = %q want decoded pluses", got)
	}
	got = projectNameFromDir("space%20here")
	if got != "space here" {
		t.Errorf("projectNameFromDir(encoded space) = %q want decoded space", got)
	}
	got = projectNameFromDir("bad%ZZencoding")
	if got != "bad%ZZencoding" {
		t.Errorf("projectNameFromDir(bad encoding) = %q want raw fallback", got)
	}
}
