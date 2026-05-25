package daemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/pricing"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/quota"
	"github.com/arafa-dev/ccx/internal/quotamigrate"
	"github.com/arafa-dev/ccx/internal/recstream"
	"github.com/arafa-dev/ccx/internal/scanner"
	"github.com/arafa-dev/ccx/internal/sharedscan"
	"github.com/arafa-dev/ccx/internal/storage"
)

var discardLogger = log.New(io.Discard, "", 0)

type runtimeDeps struct {
	Store    *storage.Store
	Profiles *profile.Manager
	Scanner  contracts.Scanner
	Pricing  contracts.PricingTable
}

type daemonSharedScanner interface {
	ScanShared(context.Context, string, scanner.SessionLookup) (<-chan scanner.AttributedEvent, <-chan error)
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
	if profileName == scanner.SharedCursorProfile {
		if err := a.store.SaveProfile(ctx, contracts.Profile{
			Name:      scanner.SharedCursorProfile,
			ConfigDir: filepath.Dir(filepath.Dir(file)),
		}); err != nil {
			return err
		}
	}
	return a.store.SetCursor(ctx, profileName, file, c.Offset, c.Inode)
}

func ingestAllProfiles(ctx context.Context, deps *runtimeDeps) ([]contracts.Profile, error) {
	profiles, err := deps.Profiles.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		p := profiles[i]
		if err := deps.Store.SaveProfile(ctx, p); err != nil {
			return nil, fmt.Errorf("saving profile %q before scan: %w", p.Name, err)
		}
	}

	sharedRoot := quotamigrate.SharedProjectsPath(deps.Profiles.Root())
	sharedProfiles, legacyProfiles := sharedscan.PartitionProfiles(sharedRoot, profiles)
	if len(sharedProfiles) > 0 {
		if err := ingestSharedProfiles(ctx, deps, sharedRoot, sharedProfiles); err != nil {
			return nil, err
		}
	}
	for i := range legacyProfiles {
		if err := ingestProfile(ctx, deps, legacyProfiles[i]); err != nil {
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

func ingestSharedProfiles(ctx context.Context, deps *runtimeDeps, sharedRoot string, profiles []contracts.Profile) error {
	shared, ok := deps.Scanner.(daemonSharedScanner)
	if !ok {
		return fmt.Errorf("scanner does not support shared projects")
	}
	allowed := make(map[string]struct{}, len(profiles))
	for i := range profiles {
		allowed[profiles[i].Name] = struct{}{}
	}

	events, errs := shared.ScanShared(ctx, sharedRoot, deps.Store)
	batches := make(map[string][]contracts.Event)
	flush := func(profile string) error {
		batch := batches[profile]
		if len(batch) == 0 {
			return nil
		}
		if err := deps.Store.InsertEvents(ctx, profile, batch); err != nil {
			return err
		}
		batches[profile] = batch[:0]
		return nil
	}
	flushAll := func() error {
		for profile := range batches {
			if err := flush(profile); err != nil {
				return err
			}
		}
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
				if err := flushAll(); err != nil {
					return err
				}
				continue
			}
			if _, ok := allowed[ev.Profile]; !ok {
				continue
			}
			batches[ev.Profile] = append(batches[ev.Profile], ev.Event)
			if len(batches[ev.Profile]) >= 256 {
				if err := flush(ev.Profile); err != nil {
					return err
				}
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil && scanErr == nil {
				scanErr = err
			}
		}
	}
	return scanErr
}

func observePressure(
	ctx context.Context,
	deps *runtimeDeps,
	computer quota.Computer,
	evaluator headroom.Evaluator,
	sm *recstream.StateMachine,
	hub *recstream.Hub,
	logger *log.Logger,
) {
	logger = nonNilLogger(logger)
	if deps == nil || deps.Profiles == nil || sm == nil || hub == nil {
		return
	}
	profiles, err := deps.Profiles.List(ctx)
	if err != nil {
		logger.Printf("recstream: list profiles: %v", err)
		return
	}
	for i := range profiles {
		p := profiles[i]
		if p.Limits.PlanTier == "" {
			continue
		}
		pq, err := computer.For(ctx, p)
		if err != nil {
			logger.Printf("recstream: quota for %q: %v", p.Name, err)
			continue
		}
		worstPct := pq.Window5h.Pct
		if pq.WindowWeekly.Pct > worstPct {
			worstPct = pq.WindowWeekly.Pct
		}
		emit, level := sm.Observe(p.Name, worstPct)
		if !emit {
			continue
		}
		hub.Publish(contracts.RecommendationEvent{
			Profile:        p.Name,
			Level:          level,
			Reason:         fmt.Sprintf("%s pressure %.0f%%", level, worstPct),
			Suggested:      bestSibling(ctx, evaluator, profiles, p.Name, logger),
			Quota5hPct:     pq.Window5h.Pct,
			QuotaWeeklyPct: pq.WindowWeekly.Pct,
			Timestamp:      time.Now().UTC(),
		})
	}
}

func primePressure(
	ctx context.Context,
	deps *runtimeDeps,
	computer quota.Computer,
	sm *recstream.StateMachine,
	logger *log.Logger,
) {
	logger = nonNilLogger(logger)
	if deps == nil || deps.Profiles == nil || sm == nil {
		return
	}
	profiles, err := deps.Profiles.List(ctx)
	if err != nil {
		logger.Printf("recstream: list profiles: %v", err)
		return
	}
	for i := range profiles {
		p := profiles[i]
		if p.Limits.PlanTier == "" {
			continue
		}
		pq, err := computer.For(ctx, p)
		if err != nil {
			logger.Printf("recstream: quota for %q: %v", p.Name, err)
			continue
		}
		worstPct := pq.Window5h.Pct
		if pq.WindowWeekly.Pct > worstPct {
			worstPct = pq.WindowWeekly.Pct
		}
		sm.Observe(p.Name, worstPct)
	}
}

func bestSibling(
	ctx context.Context,
	evaluator headroom.Evaluator,
	profiles []contracts.Profile,
	exclude string,
	logger *log.Logger,
) string {
	logger = nonNilLogger(logger)
	siblings := make([]contracts.Profile, 0, len(profiles))
	for i := range profiles {
		if profiles[i].Name == exclude {
			continue
		}
		siblings = append(siblings, profiles[i])
	}
	if len(siblings) == 0 {
		return ""
	}
	result, err := evaluator.Evaluate(ctx, siblings, headroom.Options{})
	if err != nil {
		logger.Printf("recstream: sibling evaluator for %q: %v", exclude, err)
		return ""
	}
	if result.Recommendation == nil {
		return ""
	}
	return result.Recommendation.Profile
}

func nonNilLogger(logger *log.Logger) *log.Logger {
	if logger != nil {
		return logger
	}
	return discardLogger
}
