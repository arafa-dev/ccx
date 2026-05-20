package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var buf []byte
	for _, l := range lines {
		buf = append(buf, l...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestReadFileStreamsFromOffsetZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects", "proj", "s-1.jsonl")
	writeJSONL(
		t, path,
		`{"type":"user","uuid":"u-1","sessionId":"s-1","timestamp":"2026-05-19T12:00:00Z"}`,
		`{"type":"assistant","uuid":"u-2","sessionId":"s-1","timestamp":"2026-05-19T12:00:01Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":3,"cache_read_input_tokens":4}}}`,
	)

	out := make(chan contracts.Event, 4)
	end, _, err := readFile(context.Background(), path, "custom-project", Cursor{}, out)
	close(out)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if end == 0 {
		t.Errorf("expected non-zero end offset")
	}

	var got []contracts.Event
	for ev := range out {
		got = append(got, ev)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].UUID != "u-1" || got[1].UUID != "u-2" {
		t.Errorf("event order wrong: %+v", got)
	}
	if got[0].Project != "custom-project" || got[1].Project != "custom-project" {
		t.Errorf("project names wrong: %+v", got)
	}
}

func TestReadFileSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects", "proj", "s-1.jsonl")
	writeJSONL(
		t, path,
		`{"type":"user","uuid":"u-1","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`,
		`garbage line not json`,
		``,
		`{"type":"future-type","uuid":"u-2","sessionId":"s","timestamp":"2026-05-19T12:00:01Z"}`,
		`{"type":"user","uuid":"u-3","sessionId":"s","timestamp":"2026-05-19T12:00:02Z"}`,
	)

	out := make(chan contracts.Event, 8)
	if _, _, err := readFile(context.Background(), path, "proj", Cursor{}, out); err != nil {
		t.Fatalf("readFile: %v", err)
	}
	close(out)

	var uuids []string
	for ev := range out {
		uuids = append(uuids, ev.UUID)
	}
	want := []string{"u-1", "u-2", "u-3"}
	if len(uuids) != len(want) {
		t.Fatalf("got uuids %v want %v", uuids, want)
	}
	for i := range want {
		if uuids[i] != want[i] {
			t.Fatalf("uuids[%d] = %q want %q", i, uuids[i], want[i])
		}
	}
}

func TestReadFileResumesFromCursor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects", "proj", "s-1.jsonl")
	writeJSONL(
		t, path,
		`{"type":"user","uuid":"u-1","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`,
		`{"type":"user","uuid":"u-2","sessionId":"s","timestamp":"2026-05-19T12:00:01Z"}`,
	)

	out := make(chan contracts.Event, 4)
	end, inode, err := readFile(context.Background(), path, "proj", Cursor{}, out)
	close(out)
	if err != nil {
		t.Fatalf("readFile 1: %v", err)
	}
	drainCount := 0
	for range out {
		drainCount++
	}
	if drainCount != 2 {
		t.Fatalf("first pass got %d events, want 2", drainCount)
	}

	out2 := make(chan contracts.Event, 4)
	end2, _, err := readFile(context.Background(), path, "proj", Cursor{Offset: end, Inode: inode}, out2)
	close(out2)
	if err != nil {
		t.Fatalf("readFile 2: %v", err)
	}
	if end2 != end {
		t.Errorf("end after no-op resume = %d, want %d", end2, end)
	}
	for ev := range out2 {
		t.Errorf("unexpected event on second pass: %+v", ev)
	}
}

func TestReadFileResetsOnInodeChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects", "proj", "s-1.jsonl")
	writeJSONL(
		t, path,
		`{"type":"user","uuid":"u-1","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`,
	)

	out := make(chan contracts.Event, 4)
	if _, _, err := readFile(context.Background(), path, "proj", Cursor{Offset: 999999, Inode: 1}, out); err != nil {
		t.Fatalf("readFile: %v", err)
	}
	close(out)

	count := 0
	for range out {
		count++
	}
	if count != 1 {
		t.Errorf("expected 1 event after inode-mismatch reset, got %d", count)
	}
}
