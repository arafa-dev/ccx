package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

func makeEvents(n int, base time.Time) []contracts.Event {
	out := make([]contracts.Event, n)
	for i := 0; i < n; i++ {
		out[i] = contracts.Event{
			UUID:      "bench-" + itoa(i),
			SessionID: "bench-session",
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Type:      "assistant",
			Project:   "ccx",
			Model:     "claude-opus-4-7",
			Usage: &contracts.Usage{
				InputTokens: 100, OutputTokens: 50,
				CacheReadTokens: 200, CacheCreateTokens: 25,
			},
		}
	}
	return out
}

func BenchmarkInsertEvents10000(b *testing.B) {
	ctx := context.Background()
	base := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dir := b.TempDir()
		s, err := storage.NewStore(ctx, filepath.Join(dir, "bench.db"))
		if err != nil {
			b.Fatalf("NewStore: %v", err)
		}
		if err := s.Migrate(ctx); err != nil {
			b.Fatalf("Migrate: %v", err)
		}
		if err := s.SaveProfile(ctx, contracts.Profile{
			Name: "bench", ConfigDir: "/p/bench",
			CreatedAt: base, LastUsedAt: base,
		}); err != nil {
			b.Fatalf("SaveProfile: %v", err)
		}
		events := makeEvents(10_000, base)
		b.StartTimer()

		if err := s.InsertEvents(ctx, "bench", events); err != nil {
			b.Fatalf("InsertEvents: %v", err)
		}

		b.StopTimer()
		_ = s.Close()
	}
}

func TestInsertEvents10000UnderOneSecond(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf threshold in -short mode")
	}
	if raceEnabled {
		t.Skip("skipping perf threshold under race detector")
	}

	ctx := context.Background()
	base := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	s, err := storage.NewStore(ctx, filepath.Join(dir, "perf.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := s.SaveProfile(ctx, contracts.Profile{
		Name: "perf", ConfigDir: "/p/perf",
		CreatedAt: base, LastUsedAt: base,
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	events := makeEvents(10_000, base)
	start := time.Now()
	if err := s.InsertEvents(ctx, "perf", events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}
	elapsed := time.Since(start)

	t.Logf("InsertEvents(10000) took %v", elapsed)
	if elapsed > time.Second {
		t.Errorf("InsertEvents(10000) took %v, want <1s", elapsed)
	}
}
