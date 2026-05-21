package storage_test

import (
	"context"
	"errors"
	"reflect"
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

func TestSaveGetAndListProfileRoundTripLimits(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	suggest := false
	t0 := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	in := contracts.Profile{
		Name:       "work",
		ConfigDir:  "/p/work",
		CreatedAt:  t0,
		LastUsedAt: t0,
		Limits: contracts.ProfileLimits{
			DailyTokenBudget:  100000,
			WeeklyTokenBudget: 500000,
			MonthlyUSDBudget:  250.75,
			Priority:          -4,
			SuggestEnabled:    &suggest,
			RateLimitCooldown: "2h30m",
		},
	}

	if err := s.SaveProfile(ctx, in); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	got, err := s.GetProfile(ctx, "work")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if !reflect.DeepEqual(got.Limits, in.Limits) {
		t.Errorf("GetProfile limits mismatch:\n got  %+v\n want %+v", got.Limits, in.Limits)
	}

	list, err := s.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListProfiles len = %d, want 1", len(list))
	}
	if !reflect.DeepEqual(list[0].Limits, in.Limits) {
		t.Errorf("ListProfiles limits mismatch:\n got  %+v\n want %+v", list[0].Limits, in.Limits)
	}
	if list[0].Limits.SuggestEnabled == nil {
		t.Fatalf("ListProfiles SuggestEnabled = nil, want pointer to false")
	}
	if *list[0].Limits.SuggestEnabled {
		t.Fatalf("ListProfiles SuggestEnabled = true, want false")
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

func TestListProfilesEmptyReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	got, err := s.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListProfiles on empty store: got %d profiles, want 0", len(got))
	}
}

func TestListProfilesSortedByName(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	t0 := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		p := contracts.Profile{
			Name: name, ConfigDir: "/p/" + name,
			CreatedAt: t0, LastUsedAt: t0,
		}
		if err := s.SaveProfile(ctx, p); err != nil {
			t.Fatalf("SaveProfile(%q): %v", name, err)
		}
	}

	got, err := s.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListProfiles: got %d profiles, want 3", len(got))
	}
	want := []string{"alpha", "bravo", "charlie"}
	for i, p := range got {
		if p.Name != want[i] {
			t.Errorf("profile[%d].Name = %q, want %q", i, p.Name, want[i])
		}
	}
}

func TestDeleteProfileRemovesRow(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	t0 := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	p := contracts.Profile{
		Name: "work", ConfigDir: "/p/work",
		CreatedAt: t0, LastUsedAt: t0,
	}
	if err := s.SaveProfile(ctx, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	if err := s.DeleteProfile(ctx, "work"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}

	if _, err := s.GetProfile(ctx, "work"); !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Errorf("after delete, GetProfile err = %v, want ErrProfileNotFound", err)
	}
}

func TestDeleteProfileUnknownReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	err := s.DeleteProfile(ctx, "ghost")
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Errorf("DeleteProfile(ghost) err = %v, want ErrProfileNotFound", err)
	}
}
