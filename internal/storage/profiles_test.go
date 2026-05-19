package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSaveAndGetProfileRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	in := contracts.Profile{
		Name:       "work",
		ConfigDir:  "/Users/arafa/.claude-profiles/work",
		Label:      "Work account",
		Color:      "#3B82F6",
		CreatedAt:  time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		LastUsedAt: time.Date(2026, 5, 19, 15, 30, 0, 0, time.UTC),
	}

	if err := s.SaveProfile(ctx, in); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	got, err := s.GetProfile(ctx, "work")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}

	if got.Name != in.Name || got.ConfigDir != in.ConfigDir ||
		got.Label != in.Label || got.Color != in.Color ||
		!got.CreatedAt.Equal(in.CreatedAt) || !got.LastUsedAt.Equal(in.LastUsedAt) {
		t.Errorf("roundtrip mismatch:\n got  %+v\n want %+v", got, in)
	}
}

func TestSaveProfileUpsertOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	t0 := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 5, 19, 18, 0, 0, 0, time.UTC)

	first := contracts.Profile{
		Name: "work", ConfigDir: "/p/work", Label: "old", Color: "#000000",
		CreatedAt: t0, LastUsedAt: t0,
	}
	second := contracts.Profile{
		Name: "work", ConfigDir: "/p/work", Label: "new", Color: "#FFFFFF",
		CreatedAt: t0, LastUsedAt: t1,
	}

	if err := s.SaveProfile(ctx, first); err != nil {
		t.Fatalf("first SaveProfile: %v", err)
	}
	if err := s.SaveProfile(ctx, second); err != nil {
		t.Fatalf("second SaveProfile: %v", err)
	}

	got, err := s.GetProfile(ctx, "work")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Label != "new" || got.Color != "#FFFFFF" || !got.LastUsedAt.Equal(t1) {
		t.Errorf("upsert did not overwrite:\n got  %+v\n want label=new color=#FFFFFF lastUsed=%v", got, t1)
	}
}

func TestGetProfileNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.GetProfile(ctx, "nope")
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Errorf("GetProfile(nope) err = %v, want ErrProfileNotFound", err)
	}
}
