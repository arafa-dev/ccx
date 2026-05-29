package quotawire

import (
	"context"
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/plandetect"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/quota"
	"github.com/arafa-dev/ccx/internal/storage"
)

// Adapter satisfies server.QuotaProvider by combining a profile manager,
// storage turn queries, and quota.Computer.
type Adapter struct {
	Store    *storage.Store
	Profiles *profile.Manager
}

// Quota returns per-profile ProfileQuota rows for the given profile filter.
// An empty filter means all profiles.
func (a *Adapter) Quota(ctx context.Context, profileFilter string) ([]contracts.ProfileQuota, error) {
	if a.Store == nil {
		return nil, fmt.Errorf("quota store unavailable")
	}
	if a.Profiles == nil {
		return nil, fmt.Errorf("quota profiles unavailable")
	}
	profiles, err := a.Profiles.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list profiles for quota: %w", err)
	}
	if profileFilter != "" {
		filtered := profiles[:0]
		for i := range profiles {
			if profiles[i].Name == profileFilter {
				filtered = append(filtered, profiles[i])
			}
		}
		profiles = filtered
	}
	// Prefer the plan tier detected from each profile's Claude config over any
	// manually configured value; fall back to manual config when detection
	// declines (missing/unknown). This keeps the displayed plan accurate
	// without requiring `ccx profile --plan-tier`.
	for i := range profiles {
		if tier, ok := plandetect.Detect(profiles[i].ConfigDir); ok {
			profiles[i].Limits.PlanTier = tier
		}
	}
	computer := quota.Computer{Store: a.Store}
	rows, failures, err := computer.All(ctx, profiles)
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		if failure := failures[profiles[i].Name]; failure != nil {
			return nil, fmt.Errorf("compute quota for %q: %w", profiles[i].Name, failure)
		}
	}
	if rows == nil {
		rows = []contracts.ProfileQuota{}
	}
	return rows, nil
}
