package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

func TestIngestProfileFlushesBufferedEventsBeforeScannerError(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	wantErr := errors.New("scan failed")
	profile := contracts.Profile{Name: "work", ConfigDir: t.TempDir()}
	deps := &runtimeDeps{
		Store: store,
		Scanner: scriptedScanner{
			event: contracts.Event{
				UUID:      "event-1",
				SessionID: "session-1",
				Timestamp: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
				Type:      "assistant",
				Project:   "ccx",
				Model:     "claude-sonnet-4-6",
				Usage:     &contracts.Usage{InputTokens: 100},
			},
			err: wantErr,
		},
	}

	if err := ingestProfile(ctx, deps, profile); !errors.Is(err, wantErr) {
		t.Fatalf("ingestProfile error = %v, want %v", err, wantErr)
	}
	rows, err := store.QueryUsage(ctx, contracts.UsageQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	if len(rows) != 1 || rows[0].Usage.InputTokens != 100 {
		t.Fatalf("usage rows = %+v, want flushed scanner event", rows)
	}
}

type scriptedScanner struct {
	event contracts.Event
	err   error
}

func (s scriptedScanner) Scan(_ context.Context, _ contracts.Profile) (<-chan contracts.Event, <-chan error) {
	events := make(chan contracts.Event)
	errs := make(chan error)
	go func() {
		defer close(events)
		defer close(errs)
		events <- s.event
		errs <- s.err
	}()
	return events, errs
}
