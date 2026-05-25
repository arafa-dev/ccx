package daemon

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/pricing"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/scanner"
	"github.com/arafa-dev/ccx/internal/storage"
)

type runtimeDeps struct {
	Store    *storage.Store
	Profiles *profile.Manager
	Scanner  contracts.Scanner
	Pricing  contracts.PricingTable
}

func (d *runtimeDeps) Close() error {
	if d == nil || d.Store == nil {
		return nil
	}
	return d.Store.Close()
}

func buildRuntimeDeps(ctx context.Context, root string) (*runtimeDeps, error) {
	store, err := storage.NewStore(ctx, filepath.Join(root, dbFileName))
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	profiles, err := profile.NewManager(root)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("profile manager: %w", err)
	}
	priceTab, err := pricing.NewTable()
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("pricing: %w", err)
	}
	return &runtimeDeps{
		Store:    store,
		Profiles: profiles,
		Scanner:  scanner.NewScanner(&storeCursorAdapter{store: store}),
		Pricing:  priceTab,
	}, nil
}

type storeCursorAdapter struct {
	store contracts.Store
}

func (a *storeCursorAdapter) Get(ctx context.Context, profileName, file string) (scanner.Cursor, error) {
	off, ino, err := a.store.GetCursor(ctx, profileName, file)
	return scanner.Cursor{Offset: off, Inode: ino}, err
}

func (a *storeCursorAdapter) Set(ctx context.Context, profileName, file string, c scanner.Cursor) error {
	return a.store.SetCursor(ctx, profileName, file, c.Offset, c.Inode)
}

func ingestAllProfiles(ctx context.Context, deps *runtimeDeps) ([]contracts.Profile, error) {
	profiles, err := deps.Profiles.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		if err := ingestProfile(ctx, deps, profiles[i]); err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

func ingestProfile(ctx context.Context, deps *runtimeDeps, p contracts.Profile) error { //nolint:gocritic // Profile is a contract value.
	if err := deps.Store.SaveProfile(ctx, p); err != nil {
		return fmt.Errorf("saving profile %q before scan: %w", p.Name, err)
	}
	events, errs := deps.Scanner.Scan(ctx, p)
	batch := make([]contracts.Event, 0, 256)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := deps.Store.InsertEvents(ctx, p.Name, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	var scanErr error
	for events != nil || errs != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				events = nil
				if err := flush(); err != nil {
					return err
				}
				continue
			}
			batch = append(batch, ev)
			if len(batch) >= cap(batch) {
				if err := flush(); err != nil {
					return err
				}
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				if scanErr == nil {
					scanErr = err
				}
			}
		}
	}
	return scanErr
}
