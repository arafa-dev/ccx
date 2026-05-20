package storage_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/storage"
)

func TestNewStoreInMemory(t *testing.T) {
	ctx := context.Background()

	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if s == nil {
		t.Fatal("NewStore returned nil *Store with nil error")
	}
}

func TestNewStoreFileBacked(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := dir + "/ccx.db"

	s, err := storage.NewStore(ctx, path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestNewStoreFileBackedAppliesPragmas(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if got := s.PragmaString(ctx, t, "journal_mode"); got != "wal" {
		t.Errorf("PRAGMA journal_mode = %q, want wal", got)
	}
	if got := s.PragmaInt(ctx, t, "foreign_keys"); got != 1 {
		t.Errorf("PRAGMA foreign_keys = %d, want 1", got)
	}
	if got := s.PragmaInt(ctx, t, "busy_timeout"); got != 5000 {
		t.Errorf("PRAGMA busy_timeout = %d, want 5000", got)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
