package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/scanner"
)

// stageProfile copies fixture files into a temporary <ConfigDir>/projects/<proj>/ tree
// and returns the staged profile.
func stageProfile(t *testing.T, fixtureToFilename map[string]string) contracts.Profile {
	t.Helper()
	dir := t.TempDir()
	for fixture, rel := range fixtureToFilename {
		src := filepath.Join("testdata", "fixtures", fixture)
		data, err := os.ReadFile(src) // #nosec G304 -- fixture names are controlled by tests.
		if err != nil {
			t.Fatalf("read fixture %q: %v", fixture, err)
		}
		dst := filepath.Join(dir, "projects", rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil { // #nosec G703 -- dst is under t.TempDir.
			t.Fatalf("write fixture: %v", err)
		}
	}
	return contracts.Profile{Name: "test", ConfigDir: dir}
}

func TestScannerGoldenSampleSession(t *testing.T) {
	profile := stageProfile(t, map[string]string{
		"sample-session.jsonl": filepath.Join("home%2Fu%2Fproj", "sess-001.jsonl"),
	})

	s := scanner.NewScanner(scanner.NewMemoryCursorStore())
	events, errs := s.Scan(context.Background(), profile)

	var got []contracts.Event
	for ev := range events {
		got = append(got, ev)
	}
	for err := range errs {
		t.Errorf("unexpected error: %v", err)
	}

	want := []contracts.Event{
		{
			UUID: "01H7Z8AAAA", SessionID: "sess-001", Type: "user", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		},
		{
			UUID: "01H7Z8AAAB", SessionID: "sess-001", Type: "assistant", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 0, 1, 0, time.UTC),
			Model:     "claude-opus-4-7",
			Usage:     &contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 10},
		},
		{
			UUID: "01H7Z8AAAC", SessionID: "sess-001", Type: "user", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 1, 0, 0, time.UTC),
		},
		{
			UUID: "01H7Z8AAAD", SessionID: "sess-001", Type: "assistant", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 1, 2, 0, time.UTC),
			Model:     "claude-sonnet-4-6",
			Usage:     &contracts.Usage{InputTokens: 200, OutputTokens: 80, CacheReadTokens: 500, CacheCreateTokens: 0},
		},
		{
			UUID: "01H7Z8AAAE", SessionID: "sess-001", Type: "tool_result", Project: "home/u/proj",
			Timestamp: time.Date(2026, 5, 19, 12, 1, 3, 0, time.UTC),
		},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i := range want {
		if !eventEqual(got[i], want[i]) {
			t.Errorf("event[%d] mismatch:\n got  %+v usage=%v\n want %+v usage=%v",
				i, got[i], usageDeref(got[i].Usage), want[i], usageDeref(want[i].Usage))
		}
	}
}

func TestScannerSkipsMalformedAndUnknown(t *testing.T) {
	profile := stageProfile(t, map[string]string{
		"malformed-session.jsonl": filepath.Join("proj", "sess-002.jsonl"),
	})

	s := scanner.NewScanner(scanner.NewMemoryCursorStore())
	events, errs := s.Scan(context.Background(), profile)

	var uuids []string
	for ev := range events {
		uuids = append(uuids, ev.UUID)
	}
	for err := range errs {
		t.Errorf("unexpected error: %v", err)
	}

	want := []string{"01H7Z8BBBA", "01H7Z8BBBB", "01H7Z8BBBC", "01H7Z8BBBE"}
	if len(uuids) != len(want) {
		t.Fatalf("got uuids %v, want %v", uuids, want)
	}
	for i := range want {
		if uuids[i] != want[i] {
			t.Errorf("uuid[%d] = %q want %q", i, uuids[i], want[i])
		}
	}
}

func eventEqual(a, b contracts.Event) bool {
	if a.UUID != b.UUID || a.SessionID != b.SessionID || a.Type != b.Type ||
		a.Project != b.Project || a.Model != b.Model || !a.Timestamp.Equal(b.Timestamp) {
		return false
	}
	if (a.Usage == nil) != (b.Usage == nil) {
		return false
	}
	if a.Usage != nil && *a.Usage != *b.Usage {
		return false
	}
	return true
}

func usageDeref(u *contracts.Usage) contracts.Usage {
	if u == nil {
		return contracts.Usage{}
	}
	return *u
}

func TestScannerIncrementalScanReturnsNothingNewOnSecondPass(t *testing.T) {
	profile := stageProfile(t, map[string]string{
		"sample-session.jsonl": filepath.Join("home%2Fu%2Fproj", "sess-001.jsonl"),
	})

	cs := scanner.NewMemoryCursorStore()
	s := scanner.NewScanner(cs)

	events1, errs1 := s.Scan(context.Background(), profile)
	first := 0
	for range events1 {
		first++
	}
	for err := range errs1 {
		t.Fatalf("first pass error: %v", err)
	}
	if first != 5 {
		t.Fatalf("first pass got %d events, want 5", first)
	}

	events2, errs2 := s.Scan(context.Background(), profile)
	second := 0
	for ev := range events2 {
		second++
		t.Errorf("unexpected event on second pass: %+v", ev)
	}
	for err := range errs2 {
		t.Fatalf("second pass error: %v", err)
	}
	if second != 0 {
		t.Fatalf("second pass got %d events, want 0", second)
	}
}

func TestScannerReScansWhenInodeChanges(t *testing.T) {
	profile := stageProfile(t, map[string]string{
		"sample-session.jsonl": filepath.Join("home%2Fu%2Fproj", "sess-001.jsonl"),
	})

	cs := scanner.NewMemoryCursorStore()
	// Pre-poison the cursor with a non-zero inode that won't match the real file.
	jsonlPath := filepath.Join(profile.ConfigDir, "projects", "home%2Fu%2Fproj", "sess-001.jsonl")
	if err := cs.Set(context.Background(), profile.Name, jsonlPath, scanner.Cursor{Offset: 999999, Inode: 1}); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}

	s := scanner.NewScanner(cs)
	events, errs := s.Scan(context.Background(), profile)

	count := 0
	for range events {
		count++
	}
	for err := range errs {
		t.Fatalf("scan error: %v", err)
	}
	if count != 5 {
		t.Errorf("inode-change rescan got %d events, want 5", count)
	}
}
