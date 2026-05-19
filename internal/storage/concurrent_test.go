package storage_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

func TestConcurrentReadsDoNotDeadlock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Use a file-backed DB so WAL mode actually engages (in-memory ignores
	// some pragmas) and so multiple sql.DB connections can run in parallel.
	dir := t.TempDir()
	s, err := storage.NewStore(ctx, filepath.Join(dir, "ccx.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	t0 := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	if err := s.SaveProfile(ctx, contracts.Profile{
		Name: "work", ConfigDir: "/p/work",
		CreatedAt: t0, LastUsedAt: t0,
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	events := make([]contracts.Event, 500)
	for i := range events {
		events[i] = contracts.Event{
			UUID:      "u" + itoa(i),
			SessionID: "s",
			Timestamp: t0.Add(time.Duration(i) * time.Second),
			Type:      "assistant",
			Project:   "ccx",
			Model:     "claude-opus-4-7",
			Usage:     &contracts.Usage{InputTokens: 1, OutputTokens: 1, CacheReadTokens: 1, CacheCreateTokens: 1},
		}
	}
	if err := s.InsertEvents(ctx, "work", events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	const readers = 8
	const reads = 25
	var wg sync.WaitGroup
	errCh := make(chan error, readers*reads)

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < reads; i++ {
				rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
					Range: contracts.TimeRange{
						Start: t0,
						End:   t0.Add(time.Hour),
					},
				})
				if err != nil {
					errCh <- err
					return
				}
				if len(rows) == 0 {
					errCh <- errEmpty
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent read error: %v", err)
	}
}

// itoa is a tiny stdlib-free int-to-string helper to keep the imports clean.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

var errEmpty = errString("expected non-empty rows")

type errString string

func (e errString) Error() string { return string(e) }
