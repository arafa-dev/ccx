package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

func TestMigrateCreatesTables(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, tbl := range []string{"profiles", "events", "scan_cursors", "schema_version", "hook_events", "sessions", "profile_health"} {
		if !s.TableExists(ctx, t, tbl) {
			t.Errorf("expected table %q to exist after Migrate", tbl)
		}
	}
	for _, idx := range []string{"hook_events_profile_ts", "hook_events_session", "sessions_profile_seen", "sessions_status"} {
		if !s.IndexExists(ctx, t, idx) {
			t.Errorf("expected index %q to exist after Migrate", idx)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("third Migrate: %v", err)
	}
}

func TestMigrateSeedsSchemaVersion(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	got := s.SchemaVersion(ctx, t)
	if got != 3 {
		t.Errorf("schema_version: got %d want 3", got)
	}
}

func TestMigrateUpgradesV1DatabaseAndResetsUsageData(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.ExecSQL(ctx, t, storage.SchemaSQL())
	if got := s.SchemaVersion(ctx, t); got != 1 {
		t.Fatalf("seed schema_version: got %d want 1", got)
	}

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	s.ExecSQL(ctx, t, fmt.Sprintf(
		`INSERT INTO profiles (name, config_dir, created_at, last_used_at) VALUES ('work', '/p/work', %d, %d)`,
		ts.UnixNano(),
		ts.UnixNano(),
	))
	if err := s.InsertEvents(ctx, "work", []contracts.Event{{
		UUID:      "event-1",
		SessionID: "session-1",
		Timestamp: ts,
		Type:      "assistant",
		Project:   "ccx",
		Model:     "claude-opus-4-7",
	}}); err != nil {
		t.Fatalf("InsertEvents on v1 DB: %v", err)
	}

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if got := s.SchemaVersion(ctx, t); got != 3 {
		t.Errorf("schema_version after upgrade: got %d want 3", got)
	}
	if got := s.CountEvents(ctx, t, "work"); got != 0 {
		t.Errorf("event count after upgrade: got %d want 0", got)
	}
	p, err := s.GetProfile(ctx, "work")
	if err != nil {
		t.Fatalf("GetProfile after upgrade: %v", err)
	}
	if p.ConfigDir != "/p/work" {
		t.Errorf("profile config_dir after upgrade: got %q want /p/work", p.ConfigDir)
	}
	for _, tbl := range []string{"hook_events", "sessions", "profile_health"} {
		if !s.TableExists(ctx, t, tbl) {
			t.Errorf("expected table %q to exist after v1 upgrade", tbl)
		}
	}
}

func TestMigrateV3ResetsEventsForRescan(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t) // runs Migrate to currentSchemaVersion
	if err := s.SaveProfile(ctx, contracts.Profile{Name: "p", ConfigDir: "/tmp/p"}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	// Simulate legacy double-counted rows keyed by line uuid.
	if err := s.InsertEvents(ctx, "p", []contracts.Event{
		{
			UUID: "line-a", SessionID: "s", Timestamp: time.Now().UTC(), Type: "assistant",
			Usage: &contracts.Usage{OutputTokens: 8},
		},
		{
			UUID: "line-b", SessionID: "s", Timestamp: time.Now().UTC(), Type: "assistant",
			Usage: &contracts.Usage{OutputTokens: 8},
		},
	}); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	// Force the version backwards to 2 and re-run Migrate to exercise v3.
	s.ExecSQL(ctx, t, `DELETE FROM schema_version; INSERT INTO schema_version (version) VALUES (2);`)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	profile, ok, err := s.ProfileForSession(ctx, "s")
	if err != nil {
		t.Fatalf("profile for legacy session: %v", err)
	}
	if !ok || profile != "p" {
		t.Fatalf("v3 must preserve legacy session ownership before clearing events; got %q,%v", profile, ok)
	}
	if n := s.CountEvents(ctx, t, "p"); n != 0 {
		t.Fatalf("v3 must clear events for re-scan; got %d rows", n)
	}
	if cur := s.CountScanCursors(ctx, t); cur != 0 {
		t.Fatalf("v3 must clear scan_cursors to force a full re-scan; got %d", cur)
	}
}

func TestMigrateExistingV2DatabaseAddsProfileLimitColumns(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.ExecSQL(ctx, t, storage.SchemaSQL())
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if got := s.SchemaVersion(ctx, t); got != 3 {
		t.Fatalf("schema_version after first migrate: got %d want 3", got)
	}

	// Simulate a DB created by an older migration before profile limits were
	// persisted. Migrate must be safe to run again and repair the schema.
	s.DropProfileLimitColumns(ctx, t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("repair Migrate: %v", err)
	}

	suggest := true
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	in := contracts.Profile{
		Name:       "work",
		ConfigDir:  "/p/work",
		CreatedAt:  now,
		LastUsedAt: now,
		Limits: contracts.ProfileLimits{
			DailyTokenBudget:  10,
			WeeklyTokenBudget: 20,
			MonthlyUSDBudget:  30.5,
			Priority:          7,
			SuggestEnabled:    &suggest,
			RateLimitCooldown: "45m",
		},
	}
	if err := s.SaveProfile(ctx, in); err != nil {
		t.Fatalf("SaveProfile after repaired migrate: %v", err)
	}
	got, err := s.GetProfile(ctx, "work")
	if err != nil {
		t.Fatalf("GetProfile after repaired migrate: %v", err)
	}
	if got.Limits.DailyTokenBudget != in.Limits.DailyTokenBudget ||
		got.Limits.WeeklyTokenBudget != in.Limits.WeeklyTokenBudget ||
		got.Limits.MonthlyUSDBudget != in.Limits.MonthlyUSDBudget ||
		got.Limits.Priority != in.Limits.Priority ||
		got.Limits.RateLimitCooldown != in.Limits.RateLimitCooldown ||
		got.Limits.SuggestEnabled == nil || !*got.Limits.SuggestEnabled {
		t.Errorf("limits after repaired migrate mismatch:\n got  %+v\n want %+v", got.Limits, in.Limits)
	}
}
