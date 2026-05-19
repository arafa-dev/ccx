package storage_test

import (
	"context"
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
