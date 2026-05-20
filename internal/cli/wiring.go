package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/pricing"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/scanner"
	"github.com/arafa-dev/ccx/internal/shell"
	"github.com/arafa-dev/ccx/internal/storage"
)

// Deps holds the live implementations every subcommand may need.
type Deps struct {
	Store    contracts.Store
	Profiles *profile.Manager
	Scanner  contracts.Scanner
	Pricing  contracts.PricingTable
	Shell    contracts.ShellEmitter
}

// Close releases all resources owned by Deps. Safe to call on a zero value.
func (d *Deps) Close() error {
	if d == nil || d.Store == nil {
		return nil
	}
	return d.Store.Close()
}

func buildDeps(ctx context.Context) (*Deps, error) {
	home, err := platform.CCXHome()
	if err != nil {
		return nil, fmt.Errorf("ccx home: %w", err)
	}

	store, err := storage.NewStore(ctx, filepath.Join(home, "state.db"))
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	profMgr, err := profile.NewManager(home)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("profile manager: %w", err)
	}

	priceTab, err := pricing.NewTable()
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("pricing: %w", err)
	}

	return &Deps{
		Store:    store,
		Profiles: profMgr,
		Scanner:  scanner.NewScanner(&storeCursorAdapter{store: store}),
		Pricing:  priceTab,
		Shell:    shell.New(),
	}, nil
}

type storeCursorAdapter struct {
	store contracts.Store
}

func (a *storeCursorAdapter) Get(ctx context.Context, profile, file string) (scanner.Cursor, error) {
	off, ino, err := a.store.GetCursor(ctx, profile, file)
	return scanner.Cursor{Offset: off, Inode: ino}, err
}

func (a *storeCursorAdapter) Set(ctx context.Context, profile, file string, c scanner.Cursor) error {
	return a.store.SetCursor(ctx, profile, file, c.Offset, c.Inode)
}
