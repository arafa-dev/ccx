package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Store queries turn usage within quota windows.
type Store interface {
	QueryTurnsInWindow(ctx context.Context, profile string, since, until time.Time) (int, error)
	QueryOldestTurnInWindow(ctx context.Context, profile string, since, until time.Time) (time.Time, error)
}

// Computer computes profile quota windows from stored turn usage.
type Computer struct {
	Store Store
	Now   func() time.Time
}

func (c Computer) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}

	return time.Now().UTC()
}

// For computes quota usage for one profile.
//
//nolint:gocritic // Profile is passed by value to match the package API.
func (c Computer) For(ctx context.Context, p contracts.Profile) (contracts.ProfileQuota, error) {
	if c.Store == nil {
		return contracts.ProfileQuota{}, fmt.Errorf("quota: Computer.Store is nil")
	}

	now := c.now()
	cap5h, capWeekly := effectiveCaps(p.Limits)
	since5h, until5h := Window5hBounds(now)
	sinceWeekly, untilWeekly := WindowWeeklyBounds(now, p.Limits.WeeklyAnchor)

	used5h, err := c.Store.QueryTurnsInWindow(ctx, p.Name, since5h, until5h)
	if err != nil {
		return contracts.ProfileQuota{}, fmt.Errorf("5h count for %q: %w", p.Name, err)
	}
	oldest5h, err := c.Store.QueryOldestTurnInWindow(ctx, p.Name, since5h, until5h)
	if err != nil {
		return contracts.ProfileQuota{}, fmt.Errorf("oldest 5h for %q: %w", p.Name, err)
	}
	usedWeekly, err := c.Store.QueryTurnsInWindow(ctx, p.Name, sinceWeekly, untilWeekly)
	if err != nil {
		return contracts.ProfileQuota{}, fmt.Errorf("weekly count for %q: %w", p.Name, err)
	}
	oldestWeekly, err := c.Store.QueryOldestTurnInWindow(ctx, p.Name, sinceWeekly, untilWeekly)
	if err != nil {
		return contracts.ProfileQuota{}, fmt.Errorf("oldest weekly for %q: %w", p.Name, err)
	}

	return contracts.ProfileQuota{
		Profile:  p.Name,
		PlanTier: p.Limits.PlanTier,
		Window5h: contracts.QuotaWindow{
			Used:     used5h,
			Cap:      cap5h,
			Pct:      Pct(used5h, cap5h),
			ResetsAt: ResetTime5h(oldest5h),
		},
		WindowWeekly: contracts.QuotaWindow{
			Used:     usedWeekly,
			Cap:      capWeekly,
			Pct:      Pct(usedWeekly, capWeekly),
			ResetsAt: weeklyResetTime(now, oldestWeekly, p.Limits.WeeklyAnchor),
		},
	}, nil
}

// All computes quota usage for every profile, collecting per-profile failures.
func (c Computer) All(ctx context.Context, profiles []contracts.Profile) (results []contracts.ProfileQuota, failures map[string]error, err error) {
	for i := range profiles {
		profile := profiles[i]
		quota, profileErr := c.For(ctx, profile)
		if profileErr != nil {
			if failures == nil {
				failures = make(map[string]error)
			}
			failures[profile.Name] = profileErr
			continue
		}
		results = append(results, quota)
	}

	return results, failures, nil
}

//nolint:gocritic // ProfileLimits is passed by value to match the planned helper shape.
func effectiveCaps(limits contracts.ProfileLimits) (turns5h, turnsWeekly int) {
	if limits.PlanTier == "" {
		return 0, 0
	}

	turns5h, turnsWeekly = DefaultCaps(limits.PlanTier)
	if limits.Caps5hTurns > 0 {
		turns5h = limits.Caps5hTurns
	}
	if limits.CapsWeeklyTurns > 0 {
		turnsWeekly = limits.CapsWeeklyTurns
	}

	return turns5h, turnsWeekly
}

func weeklyResetTime(now, oldest time.Time, anchor string) time.Time {
	if _, ok := parseWeekday(anchor); ok {
		return ResetTimeWeeklyAnchored(now, anchor)
	}

	return ResetTimeWeekly(oldest, anchor)
}
