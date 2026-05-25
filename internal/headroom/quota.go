package headroom

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

const (
	quotaFiveHourWindow = 5 * time.Hour
	quotaWeekWindow     = 7 * 24 * time.Hour
)

func (e Evaluator) computeQuota(ctx context.Context, p *contracts.Profile, now time.Time) (window5h, windowWeekly contracts.QuotaWindow, err error) {
	cap5h, capWeekly := quotaEffectiveCaps(&p.Limits)
	since5h, until5h := quotaWindow5hBounds(now)
	sinceWeekly, untilWeekly := quotaWindowWeeklyBounds(now, p.Limits.WeeklyAnchor)

	used5h, err := e.Store.QueryTurnsInWindow(ctx, p.Name, since5h, until5h)
	if err != nil {
		return contracts.QuotaWindow{}, contracts.QuotaWindow{}, fmt.Errorf("5h count for %q: %w", p.Name, err)
	}
	oldest5h, err := e.Store.QueryOldestTurnInWindow(ctx, p.Name, since5h, until5h)
	if err != nil {
		return contracts.QuotaWindow{}, contracts.QuotaWindow{}, fmt.Errorf("oldest 5h for %q: %w", p.Name, err)
	}
	usedWeekly, err := e.Store.QueryTurnsInWindow(ctx, p.Name, sinceWeekly, untilWeekly)
	if err != nil {
		return contracts.QuotaWindow{}, contracts.QuotaWindow{}, fmt.Errorf("weekly count for %q: %w", p.Name, err)
	}
	oldestWeekly, err := e.Store.QueryOldestTurnInWindow(ctx, p.Name, sinceWeekly, untilWeekly)
	if err != nil {
		return contracts.QuotaWindow{}, contracts.QuotaWindow{}, fmt.Errorf("oldest weekly for %q: %w", p.Name, err)
	}

	return contracts.QuotaWindow{
			Used:     used5h,
			Cap:      cap5h,
			Pct:      quotaPct(used5h, cap5h),
			ResetsAt: quotaResetTime5h(oldest5h),
		}, contracts.QuotaWindow{
			Used:     usedWeekly,
			Cap:      capWeekly,
			Pct:      quotaPct(usedWeekly, capWeekly),
			ResetsAt: quotaWeeklyResetTime(now, oldestWeekly, p.Limits.WeeklyAnchor),
		}, nil
}

func quotaTrackingEnabled(planTier string) bool {
	switch planTier {
	case "pro", "max5", "max20":
		return true
	default:
		return false
	}
}

func quotaEffectiveCaps(limits *contracts.ProfileLimits) (turns5h, turnsWeekly int) {
	if !quotaTrackingEnabled(limits.PlanTier) {
		return 0, 0
	}

	turns5h = quotaDefault5hCap(limits.PlanTier)
	if limits.Caps5hTurns > 0 {
		turns5h = limits.Caps5hTurns
	}
	if limits.CapsWeeklyTurns > 0 {
		turnsWeekly = limits.CapsWeeklyTurns
	}

	return turns5h, turnsWeekly
}

func quotaDefault5hCap(planTier string) int {
	switch planTier {
	case "pro":
		return 45
	case "max5":
		return 225
	case "max20":
		return 900
	default:
		return 0
	}
}

func quotaWindow5hBounds(now time.Time) (since, until time.Time) {
	return now.Add(-quotaFiveHourWindow), now
}

func quotaWindowWeeklyBounds(now time.Time, anchor string) (since, until time.Time) {
	weekday, ok := quotaParseWeekday(anchor)
	if !ok {
		return now.Add(-quotaWeekWindow), now
	}

	return quotaMostRecentWeekdayMidnight(now, weekday), now
}

func quotaResetTime5h(oldestInWindow time.Time) time.Time {
	if oldestInWindow.IsZero() {
		return time.Time{}
	}
	return oldestInWindow.Add(quotaFiveHourWindow)
}

func quotaWeeklyResetTime(now, oldest time.Time, anchor string) time.Time {
	if _, ok := quotaParseWeekday(anchor); ok {
		return quotaResetTimeWeeklyAnchored(now, anchor)
	}
	if oldest.IsZero() {
		return time.Time{}
	}
	return oldest.Add(quotaWeekWindow)
}

func quotaResetTimeWeeklyAnchored(now time.Time, anchor string) time.Time {
	weekday, ok := quotaParseWeekday(anchor)
	if !ok {
		return now.Add(quotaWeekWindow)
	}

	mostRecent := quotaMostRecentWeekdayMidnight(now, weekday)
	return mostRecent.Add(quotaWeekWindow)
}

func quotaPct(used, quotaCap int) float64 {
	if quotaCap <= 0 {
		return 0
	}

	pct := float64(used) / float64(quotaCap) * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}

	return pct
}

func quotaParseWeekday(s string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sunday":
		return time.Sunday, true
	case "monday":
		return time.Monday, true
	case "tuesday":
		return time.Tuesday, true
	case "wednesday":
		return time.Wednesday, true
	case "thursday":
		return time.Thursday, true
	case "friday":
		return time.Friday, true
	case "saturday":
		return time.Saturday, true
	default:
		return time.Sunday, false
	}
}

func quotaMostRecentWeekdayMidnight(now time.Time, target time.Weekday) time.Time {
	nowUTC := now.UTC()
	midnight := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
	daysSinceTarget := (int(midnight.Weekday()) - int(target) + 7) % 7

	return midnight.AddDate(0, 0, -daysSinceTarget)
}
