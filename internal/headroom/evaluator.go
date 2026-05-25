// Package headroom ranks profiles for advisory profile selection.
package headroom

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

const (
	defaultRateLimitCooldown = 5 * time.Hour
	failureLookback          = 30 * 24 * time.Hour
)

// Store is the subset of storage needed to evaluate profile headroom.
type Store interface {
	QueryUsage(ctx context.Context, q contracts.UsageQuery) ([]contracts.UsageRow, error)
	QuerySessions(ctx context.Context, q contracts.SessionQuery) ([]contracts.SessionTelemetry, error)
	QueryRecentFailures(ctx context.Context, profileName string, since time.Time) ([]contracts.HookEvent, error)
	GetProfileHealth(ctx context.Context, profileName string) (contracts.ProfileHealth, error)

	// QueryTurnsInWindow returns the number of completed turns for a profile
	// in the given interval.
	QueryTurnsInWindow(ctx context.Context, profileName string, since, until time.Time) (int, error)

	// QueryOldestTurnInWindow returns the timestamp of the earliest turn for a
	// profile inside the interval, or the zero time if none.
	QueryOldestTurnInWindow(ctx context.Context, profileName string, since, until time.Time) (time.Time, error)
}

// Options controls evaluation gates.
type Options struct {
	IncludeUnavailable bool
	UnavailableReasons map[string]string
}

// Evaluator computes advisory rankings from profile configuration and recent telemetry.
type Evaluator struct {
	Store          Store
	Pricing        contracts.PricingTable
	Now            func() time.Time
	CheckConfigDir func(path string) error
}

// Result is the JSON-stable shape returned by ccx suggest.
type Result struct {
	Recommendation *Candidate  `json:"recommendation,omitempty"`
	Candidates     []Candidate `json:"candidates"`
	Error          string      `json:"error,omitempty"`
}

// Candidate captures one evaluated profile.
type Candidate struct {
	Profile         string                 `json:"profile"`
	Available       bool                   `json:"available"`
	Score           float64                `json:"score"`
	HeadroomPercent float64                `json:"headroom_percent"`
	AuthStatus      string                 `json:"auth_status"`
	CooldownUntil   *time.Time             `json:"cooldown_until,omitempty"`
	Reasons         []string               `json:"reasons"`
	Priority        int                    `json:"priority"`
	Tokens24h       int                    `json:"tokens_24h"`
	Tokens7d        int                    `json:"tokens_7d"`
	USD30d          float64                `json:"usd_30d"`
	Quota5h         *contracts.QuotaWindow `json:"quota_5h,omitempty"`
	QuotaWeekly     *contracts.QuotaWindow `json:"quota_weekly,omitempty"`
}

// Evaluate scores every supplied profile and chooses the highest-ranked available candidate.
func (e Evaluator) Evaluate(ctx context.Context, profiles []contracts.Profile, opts Options) (Result, error) {
	if e.Store == nil {
		return Result{}, errors.New("headroom: store is nil")
	}
	if e.Pricing == nil {
		return Result{}, errors.New("headroom: pricing table is nil")
	}

	now := e.now()
	candidates := make([]Candidate, 0, len(profiles))
	for i := range profiles {
		candidate, err := e.evaluateProfile(ctx, &profiles[i], now, opts)
		if err != nil {
			return Result{}, err
		}
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return lessCandidate(&candidates[i], &candidates[j])
	})
	result := Result{Candidates: candidates}
	for i := range candidates {
		if candidates[i].Available {
			rec := candidates[i]
			result.Recommendation = &rec
			break
		}
	}
	return result, nil
}

func (e Evaluator) evaluateProfile(ctx context.Context, p *contracts.Profile, now time.Time, opts Options) (Candidate, error) {
	c := Candidate{
		Profile:    p.Name,
		Available:  true,
		AuthStatus: "unknown",
		Priority:   p.Limits.Priority,
	}

	if p.Limits.SuggestEnabled != nil && !*p.Limits.SuggestEnabled {
		c.Available = false
		c.Reasons = append(c.Reasons, "suggestions disabled")
	}
	if err := e.checkConfigDir(p.ConfigDir); err != nil {
		c.Available = false
		c.Reasons = append(c.Reasons, fmt.Sprintf("config dir inaccessible: %v", err))
	}
	if reason := opts.UnavailableReasons[p.Name]; reason != "" {
		c.Available = false
		c.Reasons = append(c.Reasons, reason)
	}

	health, haveHealth, err := e.profileHealth(ctx, p.Name)
	if err != nil {
		return Candidate{}, err
	}
	if haveHealth && health.AuthStatus != "" {
		c.AuthStatus = health.AuthStatus
	}
	if c.AuthStatus == "fail" {
		if opts.IncludeUnavailable {
			c.Reasons = append(c.Reasons, "auth health fail included by --include-unavailable")
		} else {
			c.Available = false
			c.Reasons = append(c.Reasons, "auth health fail")
		}
	}

	usage, err := e.usage(ctx, p.Name, now)
	if err != nil {
		return Candidate{}, err
	}
	c.Tokens24h = usage.tokens24h
	c.Tokens7d = usage.tokens7d
	c.USD30d = usage.usd30d

	if quotaTrackingEnabled(p.Limits.PlanTier) {
		q5h, qWeekly, err := e.computeQuota(ctx, p, now)
		if err != nil {
			return Candidate{}, err
		}
		c.Quota5h = &q5h
		c.QuotaWeekly = &qWeekly
	}

	failures, err := e.Store.QueryRecentFailures(ctx, p.Name, now.Add(-failureLookback))
	if err != nil {
		return Candidate{}, fmt.Errorf("querying recent failures for %q: %w", p.Name, err)
	}
	sessions, err := e.recentSessions(ctx, p.Name, now)
	if err != nil {
		return Candidate{}, err
	}
	failurePenalty := e.applyFailureGates(&c, p, failures, sessions, health, haveHealth, now, opts.IncludeUnavailable)

	c.HeadroomPercent = headroomPercent(&p.Limits, usage)
	if c.Quota5h != nil && PressureLevelFromPct(c.Quota5h.Pct) >= PressureSoft {
		if h := 100 - c.Quota5h.Pct; h < c.HeadroomPercent {
			c.HeadroomPercent = round2(h)
		}
	}
	if c.QuotaWeekly != nil && PressureLevelFromPct(c.QuotaWeekly.Pct) >= PressureSoft {
		if h := 100 - c.QuotaWeekly.Pct; h < c.HeadroomPercent {
			c.HeadroomPercent = round2(h)
		}
	}
	if hasBudget(&p.Limits) {
		c.Reasons = append(c.Reasons, budgetReasons(&p.Limits, usage)...)
	} else {
		c.Reasons = append(c.Reasons, "no explicit budgets; using recent usage heuristic")
	}
	if p.Limits.Priority != 0 {
		c.Reasons = append(c.Reasons, fmt.Sprintf("priority %+d", p.Limits.Priority))
	}
	healthPenalty := authHealthPenalty(c.AuthStatus)
	if failurePenalty > 0 {
		c.Reasons = append(c.Reasons, fmt.Sprintf("recent failure penalty %.0f", failurePenalty))
	}
	if c.AuthStatus == "ok" {
		c.Reasons = append(c.Reasons, "auth ok")
	} else if healthPenalty > 0 {
		c.Reasons = append(c.Reasons, fmt.Sprintf("auth health %s penalty %.0f", c.AuthStatus, healthPenalty))
	}
	if len(c.Reasons) == 0 {
		c.Reasons = append(c.Reasons, "available")
	}

	c.Score = c.HeadroomPercent + float64(p.Limits.Priority) - failurePenalty - healthPenalty
	return c, nil
}

func (e Evaluator) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e Evaluator) checkConfigDir(path string) error {
	if e.CheckConfigDir != nil {
		return e.CheckConfigDir(path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	return nil
}

func (e Evaluator) profileHealth(ctx context.Context, name string) (contracts.ProfileHealth, bool, error) {
	health, err := e.Store.GetProfileHealth(ctx, name)
	if err == nil {
		return health, true, nil
	}
	if errors.Is(err, contracts.ErrProfileNotFound) {
		return contracts.ProfileHealth{}, false, nil
	}
	return contracts.ProfileHealth{}, false, fmt.Errorf("getting profile health for %q: %w", name, err)
}

type usageSummary struct {
	tokens24h int
	tokens7d  int
	usd30d    float64
}

func (e Evaluator) usage(ctx context.Context, profile string, now time.Time) (usageSummary, error) {
	tokens24h, err := e.tokensInWindow(ctx, profile, now.Add(-24*time.Hour), now)
	if err != nil {
		return usageSummary{}, err
	}
	tokens7d, err := e.tokensInWindow(ctx, profile, now.Add(-7*24*time.Hour), now)
	if err != nil {
		return usageSummary{}, err
	}
	usd30d, err := e.usdInWindow(ctx, profile, now.Add(-30*24*time.Hour), now)
	if err != nil {
		return usageSummary{}, err
	}
	return usageSummary{tokens24h: tokens24h, tokens7d: tokens7d, usd30d: usd30d}, nil
}

func (e Evaluator) tokensInWindow(ctx context.Context, profile string, start, end time.Time) (int, error) {
	rows, err := e.Store.QueryUsage(ctx, contracts.UsageQuery{
		Profile: profile,
		Range:   contracts.TimeRange{Start: start, End: end},
	})
	if err != nil {
		return 0, fmt.Errorf("querying usage for %q: %w", profile, err)
	}
	var total int
	for _, row := range rows {
		total += row.Usage.TotalTokens()
	}
	return total, nil
}

func (e Evaluator) usdInWindow(ctx context.Context, profile string, start, end time.Time) (float64, error) {
	rows, err := e.Store.QueryUsage(ctx, contracts.UsageQuery{
		Profile: profile,
		Range:   contracts.TimeRange{Start: start, End: end},
	})
	if err != nil {
		return 0, fmt.Errorf("querying usage for %q: %w", profile, err)
	}
	var total float64
	for _, row := range rows {
		cost, err := e.Pricing.Cost(row.Model, row.Day, row.Usage)
		if err != nil {
			return 0, fmt.Errorf("pricing usage for %q model %q: %w", profile, row.Model, err)
		}
		total += cost
	}
	return total, nil
}

func (e Evaluator) recentSessions(ctx context.Context, profile string, now time.Time) ([]contracts.SessionTelemetry, error) {
	sessions, err := e.Store.QuerySessions(ctx, contracts.SessionQuery{
		Profile: profile,
		Since:   now.Add(-failureLookback),
		Limit:   200,
	})
	if err != nil {
		return nil, fmt.Errorf("querying sessions for %q: %w", profile, err)
	}
	return sessions, nil
}

func (e Evaluator) applyFailureGates(
	c *Candidate,
	p *contracts.Profile,
	failures []contracts.HookEvent,
	sessions []contracts.SessionTelemetry,
	health contracts.ProfileHealth,
	haveHealth bool,
	now time.Time,
	includeUnavailable bool,
) float64 {
	var penalty float64
	for i := range failures {
		failure := &failures[i]
		switch failure.Error {
		case "rate_limit":
			cooldown := rateLimitCooldown(&p.Limits)
			until := failure.Timestamp.Add(cooldown)
			if now.Before(until) {
				c.Available = false
				if c.CooldownUntil == nil || until.After(*c.CooldownUntil) {
					c.CooldownUntil = &until
				}
				c.Reasons = append(c.Reasons, "rate limit cooldown active")
			} else {
				penalty = max(penalty, 10)
			}
		case "authentication_failed", "oauth_org_not_allowed":
			resolved := authFailureResolved(failure, sessions, health, haveHealth)
			if !resolved {
				if includeUnavailable {
					c.Reasons = append(c.Reasons, failure.Error+" unresolved included by --include-unavailable")
					penalty = max(penalty, 25)
				} else {
					c.Available = false
					c.Reasons = append(c.Reasons, failure.Error+" unresolved")
				}
			} else {
				penalty = max(penalty, 10)
			}
		default:
			penalty = max(penalty, 10)
		}
	}
	return penalty
}

func authFailureResolved(
	failure *contracts.HookEvent,
	sessions []contracts.SessionTelemetry,
	health contracts.ProfileHealth,
	haveHealth bool,
) bool {
	if failure == nil {
		return false
	}
	if haveHealth && health.AuthStatus == "ok" && health.CheckedAt.After(failure.Timestamp) {
		return true
	}
	for i := range sessions {
		if sessionResolvesAuthFailure(&sessions[i], failure.Timestamp) {
			return true
		}
	}
	return false
}

func sessionResolvesAuthFailure(session *contracts.SessionTelemetry, failureAt time.Time) bool {
	if session == nil || isAuthFailure(session.FailureError) {
		return false
	}
	switch session.Status {
	case "completed", "ended":
		return session.EndedAt.After(failureAt) || session.LastSeenAt.After(failureAt)
	case "running":
		return session.StartedAt.After(failureAt) || session.LastSeenAt.After(failureAt)
	default:
		return false
	}
}

func isAuthFailure(errorCode string) bool {
	return errorCode == "authentication_failed" || errorCode == "oauth_org_not_allowed"
}

func rateLimitCooldown(limits *contracts.ProfileLimits) time.Duration {
	if limits.RateLimitCooldown == "" {
		return defaultRateLimitCooldown
	}
	d, err := time.ParseDuration(limits.RateLimitCooldown)
	if err != nil {
		return defaultRateLimitCooldown
	}
	return d
}

func authHealthPenalty(status string) float64 {
	switch status {
	case "ok":
		return 0
	case "fail":
		return 25
	default:
		return 1
	}
}

func headroomPercent(limits *contracts.ProfileLimits, usage usageSummary) float64 {
	if !hasBudget(limits) {
		return round2(clamp(100-(float64(usage.tokens24h)/1000), 0, 100))
	}

	minHeadroom := math.Inf(1)
	if limits.DailyTokenBudget > 0 {
		minHeadroom = min(minHeadroom, 1-(float64(usage.tokens24h)/float64(limits.DailyTokenBudget)))
	}
	if limits.WeeklyTokenBudget > 0 {
		minHeadroom = min(minHeadroom, 1-(float64(usage.tokens7d)/float64(limits.WeeklyTokenBudget)))
	}
	if limits.MonthlyUSDBudget > 0 {
		minHeadroom = min(minHeadroom, 1-(usage.usd30d/limits.MonthlyUSDBudget))
	}
	return round2(min(minHeadroom*100, 100))
}

func hasBudget(limits *contracts.ProfileLimits) bool {
	return limits.DailyTokenBudget > 0 || limits.WeeklyTokenBudget > 0 || limits.MonthlyUSDBudget > 0
}

func budgetReasons(limits *contracts.ProfileLimits, usage usageSummary) []string {
	reasons := make([]string, 0, 3)
	if limits.DailyTokenBudget > 0 {
		reasons = append(reasons, fmt.Sprintf("daily tokens %d/%d", usage.tokens24h, limits.DailyTokenBudget))
	}
	if limits.WeeklyTokenBudget > 0 {
		reasons = append(reasons, fmt.Sprintf("weekly tokens %d/%d", usage.tokens7d, limits.WeeklyTokenBudget))
	}
	if limits.MonthlyUSDBudget > 0 {
		reasons = append(reasons, fmt.Sprintf("monthly cost $%.2f/$%.2f", usage.usd30d, limits.MonthlyUSDBudget))
	}
	return reasons
}

func lessCandidate(a, b *Candidate) bool {
	if a.Available != b.Available {
		return a.Available
	}
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if a.Tokens24h != b.Tokens24h {
		return a.Tokens24h < b.Tokens24h
	}
	return a.Profile < b.Profile
}

func clamp(v, lower, upper float64) float64 {
	return math.Max(lower, math.Min(upper, v))
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
