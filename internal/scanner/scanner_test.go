package scanner_test

import (
	"context"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/scanner"
)

func TestNewScannerImplementsContractsScanner(t *testing.T) {
	var _ contracts.Scanner = scanner.NewScanner(scanner.NewMemoryCursorStore())
}

func TestScannerScanReturnsClosedChannelsForMissingDir(t *testing.T) {
	s := scanner.NewScanner(scanner.NewMemoryCursorStore())
	profile := contracts.Profile{Name: "p", ConfigDir: t.TempDir()}

	events, errs := s.Scan(context.Background(), profile)

	count := 0
	for range events {
		count++
	}
	for err := range errs {
		t.Errorf("unexpected error for empty profile: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 events for empty profile, got %d", count)
	}
}

func TestMemoryCursorStoreGetSet(t *testing.T) {
	ctx := context.Background()
	cs := scanner.NewMemoryCursorStore()

	got, err := cs.Get(ctx, "work", "/tmp/a.jsonl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (scanner.Cursor{}) {
		t.Errorf("expected zero cursor for missing key, got %+v", got)
	}

	want := scanner.Cursor{Offset: 42, Inode: 99}
	if err := cs.Set(ctx, "work", "/tmp/a.jsonl", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err = cs.Get(ctx, "work", "/tmp/a.jsonl")
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got != want {
		t.Errorf("Get after Set: got %+v want %+v", got, want)
	}
}
