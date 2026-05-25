package cli

import (
	"context"
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/quotamigrate"
	"github.com/arafa-dev/ccx/internal/scanner"
	"github.com/arafa-dev/ccx/internal/sharedscan"
)

type sharedScanner interface {
	ScanShared(context.Context, string, scanner.SessionLookup) (<-chan scanner.AttributedEvent, <-chan error)
}

func ingestSharedAwareProfiles(ctx context.Context, deps *Deps, profiles []contracts.Profile) error {
	if err := saveProfilesBeforeScan(ctx, deps, profiles); err != nil {
		return err
	}
	sharedRoot, sharedProfiles, legacyProfiles, err := cliScanPartitions(profiles)
	if err != nil {
		return err
	}
	if len(sharedProfiles) > 0 {
		if err := ingestSharedEvents(ctx, deps, sharedRoot, sharedProfiles); err != nil {
			return err
		}
	}
	for i := range legacyProfiles {
		if err := ingestProfileEvents(ctx, deps, &legacyProfiles[i]); err != nil {
			return err
		}
	}
	return nil
}

func ingestSharedAwareSuggestProfiles(ctx context.Context, deps *Deps, profiles []contracts.Profile) (map[string]string, error) {
	failures := make(map[string]string)
	if err := saveProfilesBeforeScan(ctx, deps, profiles); err != nil {
		return nil, err
	}
	sharedRoot, sharedProfiles, legacyProfiles, err := cliScanPartitions(profiles)
	if err != nil {
		return nil, err
	}
	if len(sharedProfiles) > 0 {
		if err := ingestSharedEvents(ctx, deps, sharedRoot, sharedProfiles); err != nil {
			for i := range sharedProfiles {
				failures[sharedProfiles[i].Name] = fmt.Sprintf("scan failed: %v", err)
			}
		}
	}
	for i := range legacyProfiles {
		p := &legacyProfiles[i]
		if err := ingestProfileEvents(ctx, deps, p); err != nil {
			failures[p.Name] = fmt.Sprintf("scan failed: %v", err)
		}
	}
	return failures, nil
}

func saveProfilesBeforeScan(ctx context.Context, deps *Deps, profiles []contracts.Profile) error {
	for i := range profiles {
		p := profiles[i]
		if err := deps.Store.SaveProfile(ctx, p); err != nil {
			return fmt.Errorf("saving profile %q before scan: %w", p.Name, err)
		}
	}
	return nil
}

func cliScanPartitions(profiles []contracts.Profile) (sharedRoot string, sharedProfiles, legacyProfiles []contracts.Profile, err error) {
	home, err := platform.CCXHome()
	if err != nil {
		return "", nil, nil, err
	}
	sharedRoot = quotamigrate.SharedProjectsPath(home)
	sharedProfiles, legacyProfiles = sharedscan.PartitionProfiles(sharedRoot, profiles)
	return sharedRoot, sharedProfiles, legacyProfiles, nil
}

func ingestProfileEvents(ctx context.Context, deps *Deps, p *contracts.Profile) error {
	events, errs := deps.Scanner.Scan(ctx, *p)
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
			if err != nil && scanErr == nil {
				scanErr = err
			}
		}
	}
	return scanErr
}

func ingestSharedEvents(ctx context.Context, deps *Deps, sharedRoot string, profiles []contracts.Profile) error {
	shared, ok := deps.Scanner.(sharedScanner)
	if !ok {
		return fmt.Errorf("scanner does not support shared projects")
	}
	lookup, ok := deps.Store.(scanner.SessionLookup)
	if !ok {
		return fmt.Errorf("store does not support session lookup")
	}
	allowed := make(map[string]struct{}, len(profiles))
	for i := range profiles {
		allowed[profiles[i].Name] = struct{}{}
	}

	events, errs := shared.ScanShared(ctx, sharedRoot, lookup)
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
