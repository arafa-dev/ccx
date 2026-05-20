package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func seedUsageFixture(t *testing.T, s *testStoreHandle) {
	t.Helper()
	ctx := context.Background()

	mustSaveProfile(t, s.store, "work")
	mustSaveProfile(t, s.store, "personal")

	day1 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

	workEvents := []contracts.Event{
		// work, ccx, opus, day1 — 2 events
		{
			UUID: "w1", SessionID: "ws1", Timestamp: day1, Type: "assistant",
			Project: "ccx", Model: "claude-opus-4-7",
			Usage: &contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 10},
		},
		{
			UUID: "w2", SessionID: "ws1", Timestamp: day1.Add(time.Hour), Type: "assistant",
			Project: "ccx", Model: "claude-opus-4-7",
			Usage: &contracts.Usage{InputTokens: 50, OutputTokens: 25, CacheReadTokens: 100, CacheCreateTokens: 5},
		},
		// work, ccx, sonnet, day2 — 1 event
		{
			UUID: "w3", SessionID: "ws2", Timestamp: day2, Type: "assistant",
			Project: "ccx", Model: "claude-sonnet-4-6",
			Usage: &contracts.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 20, CacheCreateTokens: 1},
		},
	}
	if err := s.store.InsertEvents(ctx, "work", workEvents); err != nil {
		t.Fatalf("InsertEvents(work): %v", err)
	}

	personalEvents := []contracts.Event{
		{
			UUID: "p1", SessionID: "ps1", Timestamp: day2, Type: "assistant",
			Project: "hobby", Model: "claude-sonnet-4-6",
			Usage: &contracts.Usage{InputTokens: 1, OutputTokens: 1, CacheReadTokens: 1, CacheCreateTokens: 1},
		},
	}
	if err := s.store.InsertEvents(ctx, "personal", personalEvents); err != nil {
		t.Fatalf("InsertEvents(personal): %v", err)
	}
}

type testStoreHandle struct {
	store storeFacade
}

// storeFacade is the subset of *Store used by seedUsageFixture; declared as
// an interface to keep test helpers loosely coupled.
type storeFacade interface {
	SaveProfile(ctx context.Context, p contracts.Profile) error
	InsertEvents(ctx context.Context, profileName string, events []contracts.Event) error
}

func TestQueryUsageGroupsByProfileProjectModelDay(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 19, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}

	// Expect 3 groups: (work, ccx, opus, day1), (work, ccx, sonnet, day2),
	// (personal, hobby, sonnet, day2).
	if len(rows) != 3 {
		for i, r := range rows {
			t.Logf("row[%d] = %+v", i, r)
		}
		t.Fatalf("rows: got %d, want 3", len(rows))
	}

	// The day1 work/opus group should sum the two events.
	for _, r := range rows {
		if r.Profile == "work" && r.Model == "claude-opus-4-7" {
			want := contracts.Usage{InputTokens: 150, OutputTokens: 75, CacheReadTokens: 300, CacheCreateTokens: 15}
			if r.Usage != want {
				t.Errorf("work/opus usage = %+v, want %+v", r.Usage, want)
			}
			if r.SessionCount != 1 {
				t.Errorf("work/opus session_count = %d, want 1", r.SessionCount)
			}
		}
	}
}

func TestQueryUsageProfileFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Profile: "personal",
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	if rows[0].Profile != "personal" {
		t.Errorf("rows[0].Profile = %q, want personal", rows[0].Profile)
	}
}

func TestQueryUsageEmptyProfileReturnsAll(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	profiles := map[string]bool{}
	for _, r := range rows {
		profiles[r.Profile] = true
	}
	if !profiles["work"] || !profiles["personal"] {
		t.Errorf("expected rows for both profiles, got %v", profiles)
	}
}

func TestQueryUsageRangeFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	// Only day2 — day1 events excluded.
	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 19, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	// Expect 2 rows: (work, ccx, sonnet, day2) and (personal, hobby, sonnet, day2).
	if len(rows) != 2 {
		for i, r := range rows {
			t.Logf("row[%d] = %+v", i, r)
		}
		t.Errorf("rows: got %d, want 2", len(rows))
	}
}

func TestQueryUsageProjectFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Project: "hobby",
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	if len(rows) != 1 || rows[0].Project != "hobby" {
		t.Errorf("expected single hobby row, got %+v", rows)
	}
}
