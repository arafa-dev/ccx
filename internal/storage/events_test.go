package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

//nolint:unparam // Later aggregation tests seed multiple profile names.
func mustSaveProfile(t *testing.T, s interface {
	SaveProfile(ctx context.Context, p contracts.Profile) error
}, name string,
) {
	t.Helper()
	t0 := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	if err := s.SaveProfile(context.Background(), contracts.Profile{
		Name: name, ConfigDir: "/p/" + name,
		CreatedAt: t0, LastUsedAt: t0,
	}); err != nil {
		t.Fatalf("SaveProfile(%q): %v", name, err)
	}
}

func TestInsertEventsEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if err := s.InsertEvents(ctx, "work", nil); err != nil {
		t.Errorf("InsertEvents(nil): %v", err)
	}
	if err := s.InsertEvents(ctx, "work", []contracts.Event{}); err != nil {
		t.Errorf("InsertEvents(empty): %v", err)
	}
}

func TestInsertEventsRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	events := []contracts.Event{
		{
			UUID: "e1", SessionID: "s1", Timestamp: ts, Type: "assistant",
			Project: "ccx", Model: "claude-opus-4-7",
			Usage: &contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 25},
		},
		{
			UUID: "e2", SessionID: "s1", Timestamp: ts.Add(time.Second), Type: "assistant",
			Project: "ccx", Model: "claude-opus-4-7",
			Usage: &contracts.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 20, CacheCreateTokens: 1},
		},
	}

	if err := s.InsertEvents(ctx, "work", events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	got := s.CountEvents(ctx, t, "work")
	if got != 2 {
		t.Errorf("event count: got %d, want 2", got)
	}
}

func TestInsertEventsDuplicateUUIDIgnored(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	events := []contracts.Event{
		{UUID: "dup", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
		{UUID: "dup", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
		{UUID: "uniq", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
	}

	if err := s.InsertEvents(ctx, "work", events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	got := s.CountEvents(ctx, t, "work")
	if got != 2 {
		t.Errorf("event count after dedup: got %d, want 2", got)
	}
}

func TestInsertEventsDedupsByUUIDKeepingMaxOutput(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "p")

	ts := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	mk := func(out int) contracts.Event {
		return contracts.Event{
			UUID:      "msg_123", // same identity (post-Task-1 parser behavior)
			SessionID: "s1",
			Timestamp: ts,
			Type:      "assistant",
			Project:   "proj",
			Model:     "claude-opus-4-8",
			Usage:     &contracts.Usage{InputTokens: 3, OutputTokens: out, CacheCreateTokens: 31695},
		}
	}
	// Insert partial-output rows first, then the final complete one.
	if err := s.InsertEvents(ctx, "p", []contracts.Event{mk(8), mk(8), mk(326)}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{Profile: "p"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	var totalOut, totalIn int
	for _, r := range rows {
		totalOut += r.Usage.OutputTokens
		totalIn += r.Usage.InputTokens
	}
	if totalOut != 326 {
		t.Fatalf("want output 326 (one response, max output), got %d", totalOut)
	}
	if totalIn != 3 {
		t.Fatalf("want input 3 (counted once), got %d", totalIn)
	}
}

func TestInsertEventsRescanIsSafe(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	events := []contracts.Event{
		{UUID: "a", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
		{UUID: "b", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
	}

	if err := s.InsertEvents(ctx, "work", events); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := s.InsertEvents(ctx, "work", events); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	got := s.CountEvents(ctx, t, "work")
	if got != 2 {
		t.Errorf("event count after rescan: got %d, want 2", got)
	}
}

func TestInsertEventsNilUsageStoresZero(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	events := []contracts.Event{
		{UUID: "e1", SessionID: "s1", Timestamp: ts, Type: "user", Project: "p", Model: "", Usage: nil},
	}

	if err := s.InsertEvents(ctx, "work", events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	if got := s.CountEvents(ctx, t, "work"); got != 1 {
		t.Errorf("event count: got %d, want 1", got)
	}
}
