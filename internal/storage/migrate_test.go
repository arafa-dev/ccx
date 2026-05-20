package storage_test

import (
	"context"
	"testing"

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

	for _, tbl := range []string{"profiles", "events", "scan_cursors", "schema_version"} {
		if !s.TableExists(ctx, t, tbl) {
			t.Errorf("expected table %q to exist after Migrate", tbl)
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
	if got != 1 {
		t.Errorf("schema_version: got %d want 1", got)
	}
}
