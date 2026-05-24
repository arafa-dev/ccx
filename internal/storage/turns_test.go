package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestQueryTurnsInWindowCountsStopAndStopFailure(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	events := []contracts.HookEvent{
		{Session: "before-window", Event: "Stop", Timestamp: now.Add(-6 * time.Hour)},
		{Session: "s1", Event: "Stop", Timestamp: now.Add(-4 * time.Hour)},
		{Session: "s2", Event: "Stop", Timestamp: now.Add(-3 * time.Hour)},
		{Session: "s3", Event: "Stop", Timestamp: now.Add(-2 * time.Hour)},
		{Session: "s4", Event: "Stop", Timestamp: now.Add(-time.Hour)},
		{Session: "s5", Event: "StopFailure", Timestamp: now.Add(-30 * time.Minute), Error: "rate_limit"},
		{Session: "after-window", Event: "Stop", Timestamp: now.Add(time.Minute)},
	}
	for _, ev := range events {
		if err := s.InsertHookEvent(ctx, "work", ev); err != nil {
			t.Fatalf("InsertHookEvent(%s): %v", ev.Session, err)
		}
	}

	got, err := s.QueryTurnsInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if got != 5 {
		t.Errorf("turn count = %d, want 5", got)
	}
}

func TestQueryTurnsInWindowExcludesAuthFailures(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	events := []contracts.HookEvent{
		{Session: "s1", Event: "Stop", Timestamp: now.Add(-4 * time.Hour)},
		{Session: "s2", Event: "StopFailure", Timestamp: now.Add(-3 * time.Hour), Error: "authentication_failed"},
		{Session: "s3", Event: "StopFailure", Timestamp: now.Add(-2 * time.Hour), Error: "oauth_org_not_allowed"},
		{Session: "s4", Event: "StopFailure", Timestamp: now.Add(-time.Hour), Error: "rate_limit"},
		{Session: "s5", Event: "StopFailure", Timestamp: now.Add(-30 * time.Minute), Error: "server_error"},
	}
	for _, ev := range events {
		if err := s.InsertHookEvent(ctx, "work", ev); err != nil {
			t.Fatalf("InsertHookEvent(%s): %v", ev.Session, err)
		}
	}

	got, err := s.QueryTurnsInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if got != 3 {
		t.Errorf("turn count = %d, want 3", got)
	}
}

func TestQueryTurnsInWindowEmptyReturnsZero(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	got, err := s.QueryTurnsInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if got != 0 {
		t.Errorf("turn count = %d, want 0", got)
	}
}

func TestQueryTurnsInWindowOtherProfileNotCounted(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")
	mustSaveProfile(t, s, "personal")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	if err := s.InsertHookEvent(ctx, "personal", contracts.HookEvent{
		Session:   "s1",
		Event:     "Stop",
		Timestamp: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("InsertHookEvent: %v", err)
	}

	got, err := s.QueryTurnsInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if got != 0 {
		t.Errorf("turn count = %d, want 0", got)
	}
}

func TestQueryTurnsInWindowCountsStopFailureWithNullError(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	s.ExecSQL(ctx, t, fmt.Sprintf(`
INSERT INTO hook_events (profile_name, session_id, event_name, ts, error)
VALUES ('work', 's1', 'StopFailure', %d, NULL)
`, now.Add(-time.Hour).UnixNano()))

	got, err := s.QueryTurnsInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if got != 1 {
		t.Errorf("turn count = %d, want 1", got)
	}
}

func TestQueryOldestTurnInWindowReturnsOldestCountedTurnTimestampUTC(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	oldest := now.Add(-4 * time.Hour)
	events := []contracts.HookEvent{
		{Session: "before-window", Event: "Stop", Timestamp: now.Add(-6 * time.Hour)},
		{Session: "s1", Event: "Stop", Timestamp: now.Add(-time.Hour)},
		{Session: "s2", Event: "StopFailure", Timestamp: oldest, Error: "rate_limit"},
	}
	for _, ev := range events {
		if err := s.InsertHookEvent(ctx, "work", ev); err != nil {
			t.Fatalf("InsertHookEvent(%s): %v", ev.Session, err)
		}
	}

	got, err := s.QueryOldestTurnInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryOldestTurnInWindow: %v", err)
	}
	want := oldest.UTC()
	if !got.Equal(want) {
		t.Errorf("oldest turn = %v, want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("oldest turn location = %v, want UTC", got.Location())
	}
}

func TestQueryOldestTurnInWindowEmptyReturnsZero(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	got, err := s.QueryOldestTurnInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryOldestTurnInWindow: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("oldest turn = %v, want zero time", got)
	}
}

func TestQueryOldestTurnInWindowExcludesAuthFailureStopFailure(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	counted := now.Add(-time.Hour)
	events := []contracts.HookEvent{
		{Session: "s1", Event: "StopFailure", Timestamp: now.Add(-4 * time.Hour), Error: "authentication_failed"},
		{Session: "s2", Event: "StopFailure", Timestamp: now.Add(-3 * time.Hour), Error: "oauth_org_not_allowed"},
		{Session: "s3", Event: "Stop", Timestamp: counted},
	}
	for _, ev := range events {
		if err := s.InsertHookEvent(ctx, "work", ev); err != nil {
			t.Fatalf("InsertHookEvent(%s): %v", ev.Session, err)
		}
	}

	got, err := s.QueryOldestTurnInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryOldestTurnInWindow: %v", err)
	}
	if !got.Equal(counted) {
		t.Errorf("oldest turn = %v, want %v", got, counted)
	}
}

func TestQueryOldestTurnInWindowRespectsProfileIsolation(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")
	mustSaveProfile(t, s, "personal")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	if err := s.InsertHookEvent(ctx, "personal", contracts.HookEvent{
		Session:   "s1",
		Event:     "Stop",
		Timestamp: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("InsertHookEvent: %v", err)
	}

	got, err := s.QueryOldestTurnInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryOldestTurnInWindow: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("oldest turn = %v, want zero time", got)
	}
}

func TestQueryTurnsInWindowUsesRollingWindowBoundaries(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	until := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	since := until.Add(-5 * time.Hour)
	events := []contracts.HookEvent{
		{Session: "expired-lower-bound", Event: "Stop", Timestamp: since},
		{Session: "inside-window", Event: "Stop", Timestamp: since.Add(time.Nanosecond)},
		{Session: "upper-bound", Event: "Stop", Timestamp: until},
		{Session: "future", Event: "Stop", Timestamp: until.Add(time.Nanosecond)},
	}
	for _, ev := range events {
		if err := s.InsertHookEvent(ctx, "work", ev); err != nil {
			t.Fatalf("InsertHookEvent(%s): %v", ev.Session, err)
		}
	}

	got, err := s.QueryTurnsInWindow(ctx, "work", since, until)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if got != 2 {
		t.Errorf("turn count = %d, want 2", got)
	}
}

func TestQueryOldestTurnInWindowUsesRollingWindowBoundaries(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	until := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	since := until.Add(-5 * time.Hour)
	inside := since.Add(time.Nanosecond)
	events := []contracts.HookEvent{
		{Session: "expired-lower-bound", Event: "Stop", Timestamp: since},
		{Session: "inside-window", Event: "Stop", Timestamp: inside},
		{Session: "upper-bound", Event: "Stop", Timestamp: until},
		{Session: "future", Event: "Stop", Timestamp: until.Add(time.Nanosecond)},
	}
	for _, ev := range events {
		if err := s.InsertHookEvent(ctx, "work", ev); err != nil {
			t.Fatalf("InsertHookEvent(%s): %v", ev.Session, err)
		}
	}

	got, err := s.QueryOldestTurnInWindow(ctx, "work", since, until)
	if err != nil {
		t.Fatalf("QueryOldestTurnInWindow: %v", err)
	}
	if !got.Equal(inside) {
		t.Errorf("oldest turn = %v, want %v", got, inside)
	}
}
