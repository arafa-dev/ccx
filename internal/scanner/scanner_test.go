package scanner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/scanner"
)

func TestNewScannerImplementsContractsScanner(t *testing.T) {
	var _ contracts.Scanner = scanner.NewScanner(scanner.NewMemoryCursorStore())
}

func TestScannerScanReturnsClosedChannelsForMissingDir(t *testing.T) {
	s := scanner.NewScanner(scanner.NewMemoryCursorStore())
	profile := contracts.Profile{Name: "p", ConfigDir: t.TempDir()}

	events, errs := s.Scan(context.Background(), profile)

	count := 0
	for range events {
		count++
	}
	for err := range errs {
		t.Errorf("unexpected error for empty profile: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 events for empty profile, got %d", count)
	}
}

func TestMemoryCursorStoreGetSet(t *testing.T) {
	ctx := context.Background()
	cs := scanner.NewMemoryCursorStore()

	got, err := cs.Get(ctx, "work", "/tmp/a.jsonl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (scanner.Cursor{}) {
		t.Errorf("expected zero cursor for missing key, got %+v", got)
	}

	want := scanner.Cursor{Offset: 42, Inode: 99}
	if err := cs.Set(ctx, "work", "/tmp/a.jsonl", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err = cs.Get(ctx, "work", "/tmp/a.jsonl")
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got != want {
		t.Errorf("Get after Set: got %+v want %+v", got, want)
	}
}

func TestScanSharedAttributesEventsBySession(t *testing.T) {
	ctx := context.Background()
	projectsRoot := t.TempDir()
	writeSharedJSONL(t, projectsRoot, "proj-a", "s1", "evt-work")
	writeSharedJSONL(t, projectsRoot, "proj-b", "s2", "evt-personal")

	s := scanner.NewScanner(scanner.NewMemoryCursorStore())
	events, errs := s.ScanShared(ctx, projectsRoot, staticSessionLookup{
		"s1": "work",
		"s2": "personal",
	})

	got := map[string]string{}
	for ev := range events {
		got[ev.Event.UUID] = ev.Profile
	}
	for err := range errs {
		t.Fatalf("unexpected scan error: %v", err)
	}

	want := map[string]string{
		"evt-work":     "work",
		"evt-personal": "personal",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("attributed profiles = %v, want %v", got, want)
	}
}

func TestScanSharedSkipsUnknownSessionsWithoutError(t *testing.T) {
	ctx := context.Background()
	projectsRoot := t.TempDir()
	path := writeSharedJSONL(t, projectsRoot, "proj-a", "unknown", "evt-unknown")

	cs := scanner.NewMemoryCursorStore()
	s := scanner.NewScanner(cs)
	events, errs := s.ScanShared(ctx, projectsRoot, staticSessionLookup{})

	for ev := range events {
		t.Fatalf("unexpected event for unknown session: %+v", ev)
	}
	for err := range errs {
		t.Fatalf("unexpected scan error: %v", err)
	}
	cursor, err := cs.Get(ctx, scanner.SharedCursorProfile, path)
	if err != nil {
		t.Fatalf("shared cursor get: %v", err)
	}
	if cursor != (scanner.Cursor{}) {
		t.Fatalf("shared cursor = %+v, want zero cursor so unknown session can be retried", cursor)
	}
}

func TestScanSharedUsesSharedCursorSentinel(t *testing.T) {
	ctx := context.Background()
	projectsRoot := t.TempDir()
	path := writeSharedJSONL(t, projectsRoot, "proj-a", "s1", "evt-work")

	cs := scanner.NewMemoryCursorStore()
	s := scanner.NewScanner(cs)
	events, errs := s.ScanShared(ctx, projectsRoot, staticSessionLookup{"s1": "work"})

	count := 0
	for range events {
		count++
	}
	for err := range errs {
		t.Fatalf("first scan error: %v", err)
	}
	if count != 1 {
		t.Fatalf("first scan emitted %d events, want 1", count)
	}
	cursor, err := cs.Get(ctx, scanner.SharedCursorProfile, path)
	if err != nil {
		t.Fatalf("shared cursor get: %v", err)
	}
	if cursor.Offset == 0 || cursor.Inode == 0 {
		t.Fatalf("shared cursor = %+v, want non-zero offset and inode", cursor)
	}

	events, errs = s.ScanShared(ctx, projectsRoot, staticSessionLookup{"s1": "work"})
	for ev := range events {
		t.Fatalf("unexpected event on second scan: %+v", ev)
	}
	for err := range errs {
		t.Fatalf("second scan error: %v", err)
	}
}

type staticSessionLookup map[string]string

func (s staticSessionLookup) ProfileForSession(_ context.Context, sessionID string) (string, bool, error) {
	profile, ok := s[sessionID]
	return profile, ok, nil
}

func writeSharedJSONL(t *testing.T, projectsRoot, project, sessionID, uuid string) string {
	t.Helper()

	path := filepath.Join(projectsRoot, project, sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(
		`{"type":"assistant","uuid":%q,"sessionId":%q,"timestamp":%q,"message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":5}}}`+"\n",
		uuid,
		sessionID,
		time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	)
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
