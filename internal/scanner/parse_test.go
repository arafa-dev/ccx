package scanner

import (
	"testing"
	"time"
)

func TestParseLineAssistantUsage(t *testing.T) {
	line := []byte(`{"type":"assistant","uuid":"u-1","sessionId":"s-1","timestamp":"2026-05-19T12:00:01Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":200}}}`)

	ev, ok := parseLine(line, "my-project")
	if !ok {
		t.Fatalf("parseLine returned ok=false for valid assistant event")
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

	ev, ok := parseLine(line, "proj")
	if !ok {
		t.Fatalf("parseLine returned ok=false for valid user event")
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

func TestParseLineRejectsMalformed(t *testing.T) {
	cases := [][]byte{
		[]byte(``),
		[]byte(`   `),
		[]byte(`not json at all`),
		[]byte(`{`),
		[]byte(`{"type":"assistant"`),
		[]byte(`{"type":123}`),
	}
	for i, c := range cases {
		if _, ok := parseLine(c, "p"); ok {
			t.Errorf("case %d: parseLine returned ok=true for malformed input %q", i, c)
		}
	}
}

func TestParseLineRejectsMissingUUID(t *testing.T) {
	line := []byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`)
	if _, ok := parseLine(line, "p"); ok {
		t.Errorf("parseLine returned ok=true for event with no uuid")
	}
}

func TestParseLineRejectsBadTimestamp(t *testing.T) {
	line := []byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"not-a-time"}`)
	if _, ok := parseLine(line, "p"); ok {
		t.Errorf("parseLine returned ok=true for event with bad timestamp")
	}
}

func TestParseLineIgnoresUnknownFields(t *testing.T) {
	line := []byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"2026-05-19T12:00:00Z","cwd":"/x","gitBranch":"main","parentUuid":"p","extraFutureField":42}`)
	if _, ok := parseLine(line, "p"); !ok {
		t.Errorf("parseLine returned ok=false; unknown fields should be ignored")
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
	got = projectNameFromDir("bad%ZZencoding")
	if got != "bad%ZZencoding" {
		t.Errorf("projectNameFromDir(bad encoding) = %q want raw fallback", got)
	}
}
