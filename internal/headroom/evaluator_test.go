package headroom_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
)

func TestConfiguredBudgetsUseMinimumHeadroom(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addUsage("work", now.Add(-1*time.Hour), "free", contracts.Usage{InputTokens: 200})
	store.addUsage("work", now.Add(-48*time.Hour), "free", contracts.Usage{InputTokens: 1800})
	store.addUsage("work", now.Add(-20*24*time.Hour), "cost-7", contracts.Usage{})

	result := evaluate(t, store, []contracts.Profile{profile("work", contracts.ProfileLimits{
		DailyTokenBudget:  1000,
		WeeklyTokenBudget: 10_000,
		MonthlyUSDBudget:  10,
	})})

	got := mustCandidate(t, result, "work")
	if got.HeadroomPercent != 30 {
		t.Fatalf("HeadroomPercent = %.2f, want 30 from monthly budget minimum", got.HeadroomPercent)
	}
	if result.Recommendation == nil || result.Recommendation.Profile != "work" {
		t.Fatalf("recommendation = %+v, want work", result.Recommendation)
	}
}

func TestOverBudgetProfileRanksBelowProfileWithHeadroom(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addUsage("over", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 1200})
	store.addUsage("roomy", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 200})

	result := evaluate(t, store, []contracts.Profile{
		profile("over", contracts.ProfileLimits{DailyTokenBudget: 1000}),
		profile("roomy", contracts.ProfileLimits{DailyTokenBudget: 1000}),
	})

	if result.Recommendation == nil || result.Recommendation.Profile != "roomy" {
		t.Fatalf("recommendation = %+v, want roomy", result.Recommendation)
	}
	if over, roomy := mustCandidate(t, result, "over"), mustCandidate(t, result, "roomy"); over.Score >= roomy.Score {
		t.Fatalf("over score %.2f should be less than roomy score %.2f", over.Score, roomy.Score)
	}
}

func TestHeavilyOverBudgetProfilesRankByActualNegativeHeadroom(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addUsage("less-over", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 3000})
	store.addUsage("more-over", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 10_000})

	result := evaluate(t, store, []contracts.Profile{
		profile("less-over", contracts.ProfileLimits{DailyTokenBudget: 1000}),
		profile("more-over", contracts.ProfileLimits{DailyTokenBudget: 1000, Priority: 50}),
	})

	if result.Recommendation == nil || result.Recommendation.Profile != "less-over" {
		t.Fatalf("recommendation = %+v, want less-over", result.Recommendation)
	}
	less := mustCandidate(t, result, "less-over")
	more := mustCandidate(t, result, "more-over")
	if less.HeadroomPercent != -200 || more.HeadroomPercent != -900 {
		t.Fatalf("headroom = %.2f/%.2f, want -200/-900", less.HeadroomPercent, more.HeadroomPercent)
	}
	if less.Score <= more.Score {
		t.Fatalf("less-over score %.2f should be greater than more-over score %.2f", less.Score, more.Score)
	}
}

func TestRateLimitFailureInsideCooldownExcludesProfileAndReportsCooldown(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addFailure("work", contracts.HookEvent{
		Event:     "StopFailure",
		Timestamp: now.Add(-30 * time.Minute),
		Error:     "rate_limit",
	})

	result := evaluate(t, store, []contracts.Profile{
		profile("work", contracts.ProfileLimits{DailyTokenBudget: 1000, RateLimitCooldown: "2h"}),
	})

	got := mustCandidate(t, result, "work")
	if got.Available {
		t.Fatalf("candidate available = true, want false")
	}
	wantCooldown := now.Add(90 * time.Minute)
	if got.CooldownUntil == nil || !got.CooldownUntil.Equal(wantCooldown) {
		t.Fatalf("CooldownUntil = %v, want %v", got.CooldownUntil, wantCooldown)
	}
	if result.Recommendation != nil {
		t.Fatalf("recommendation = %+v, want nil", result.Recommendation)
	}
}

func TestAuthFailExcludesUnlessIncludingUnavailable(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.health["blocked"] = contracts.ProfileHealth{Profile: "blocked", CheckedAt: now, AuthStatus: "fail"}
	store.health["ok"] = contracts.ProfileHealth{Profile: "ok", CheckedAt: now, AuthStatus: "ok"}

	profiles := []contracts.Profile{
		profile("blocked", contracts.ProfileLimits{DailyTokenBudget: 1000, Priority: 50}),
		profile("ok", contracts.ProfileLimits{DailyTokenBudget: 1000}),
	}

	result := evaluate(t, store, profiles)
	if result.Recommendation == nil || result.Recommendation.Profile != "ok" {
		t.Fatalf("recommendation = %+v, want ok", result.Recommendation)
	}
	if got := mustCandidate(t, result, "blocked"); got.Available {
		t.Fatalf("blocked available = true, want false")
	}

	include := evaluateWithOptions(t, store, profiles, headroom.Options{IncludeUnavailable: true})
	if include.Recommendation == nil || include.Recommendation.Profile != "blocked" {
		t.Fatalf("include unavailable recommendation = %+v, want blocked", include.Recommendation)
	}
	if got := mustCandidate(t, include, "blocked"); !got.Available {
		t.Fatalf("blocked available with IncludeUnavailable = false, want true")
	}
}

func TestAuthenticationFailuresExcludeProfile(t *testing.T) {
	now := testNow()
	for _, failure := range []string{"authentication_failed", "oauth_org_not_allowed"} {
		t.Run(failure, func(t *testing.T) {
			store := newFakeStore(now)
			store.addFailure("work", contracts.HookEvent{
				Event:     "StopFailure",
				Timestamp: now.Add(-time.Hour),
				Error:     failure,
			})

			result := evaluate(t, store, []contracts.Profile{
				profile("work", contracts.ProfileLimits{DailyTokenBudget: 1000}),
			})
			got := mustCandidate(t, result, "work")
			if got.Available {
				t.Fatalf("candidate available = true, want false")
			}
			if result.Recommendation != nil {
				t.Fatalf("recommendation = %+v, want nil", result.Recommendation)
			}
		})
	}
}

func TestNoBudgetHeuristicRanksLowerRecentUsageAndHigherPriority(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addUsage("quiet", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 100})
	store.addUsage("busy", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 5000})
	store.addUsage("preferred", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 1000})

	result := evaluate(t, store, []contracts.Profile{
		profile("busy", contracts.ProfileLimits{}),
		profile("preferred", contracts.ProfileLimits{Priority: 2}),
		profile("quiet", contracts.ProfileLimits{}),
	})

	gotOrder := candidateNames(result.Candidates)
	wantOrder := []string{"preferred", "quiet", "busy"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("candidate order = %v, want %v", gotOrder, wantOrder)
	}
	if result.Recommendation == nil || result.Recommendation.Profile != "preferred" {
		t.Fatalf("recommendation = %+v, want preferred", result.Recommendation)
	}
}

func TestTieBreakersUsePriorityThenLowerUsageThenName(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
		store.addUsage(name, now.Add(-20*24*time.Hour), "cost-1", contracts.Usage{})
	}
	store.addUsage("alpha", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 5})
	store.addUsage("bravo", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 10})
	store.addUsage("delta", now.Add(-time.Hour), "free", contracts.Usage{InputTokens: 5})

	result := evaluate(t, store, []contracts.Profile{
		profile("bravo", contracts.ProfileLimits{MonthlyUSDBudget: 10, Priority: 1}),
		profile("charlie", contracts.ProfileLimits{MonthlyUSDBudget: 10, Priority: 2}),
		profile("delta", contracts.ProfileLimits{MonthlyUSDBudget: 10, Priority: 1}),
		profile("alpha", contracts.ProfileLimits{MonthlyUSDBudget: 10, Priority: 1}),
	})

	gotOrder := candidateNames(result.Candidates)
	wantOrder := []string{"charlie", "alpha", "delta", "bravo"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("candidate order = %v, want %v", gotOrder, wantOrder)
	}
}

func evaluate(t *testing.T, store *fakeStore, profiles []contracts.Profile) headroom.Result {
	t.Helper()
	return evaluateWithOptions(t, store, profiles, headroom.Options{})
}

func evaluateWithOptions(t *testing.T, store *fakeStore, profiles []contracts.Profile, opts headroom.Options) headroom.Result {
	t.Helper()
	evaluator := headroom.Evaluator{
		Store:   store,
		Pricing: fakePricing{"cost-1": 1, "cost-7": 7},
		Now:     func() time.Time { return store.now },
		CheckConfigDir: func(path string) error {
			if path == "/inaccessible" {
				return errors.New("permission denied")
			}
			return nil
		},
	}
	result, err := evaluator.Evaluate(context.Background(), profiles, opts)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return result
}

func mustCandidate(t *testing.T, result headroom.Result, name string) headroom.Candidate {
	t.Helper()
	idx := slices.IndexFunc(result.Candidates, func(c headroom.Candidate) bool {
		return c.Profile == name
	})
	if idx < 0 {
		t.Fatalf("candidate %q missing from %+v", name, result.Candidates)
	}
	return result.Candidates[idx]
}

func candidateNames(candidates []headroom.Candidate) []string {
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		names = append(names, c.Profile)
	}
	return names
}

func profile(name string, limits contracts.ProfileLimits) contracts.Profile {
	return contracts.Profile{
		Name:      name,
		ConfigDir: "/profiles/" + name,
		Limits:    limits,
	}
}

func testNow() time.Time {
	return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
}

type fakeStore struct {
	now      time.Time
	usage    []usageEvent
	failures map[string][]contracts.HookEvent
	health   map[string]contracts.ProfileHealth
}

type usageEvent struct {
	profile string
	at      time.Time
	model   string
	usage   contracts.Usage
}

func newFakeStore(now time.Time) *fakeStore {
	return &fakeStore{
		now:      now,
		failures: make(map[string][]contracts.HookEvent),
		health:   make(map[string]contracts.ProfileHealth),
	}
}

func (s *fakeStore) addUsage(profile string, at time.Time, model string, usage contracts.Usage) {
	s.usage = append(s.usage, usageEvent{profile: profile, at: at, model: model, usage: usage})
}

func (s *fakeStore) addFailure(profile string, ev contracts.HookEvent) {
	ev.Profile = profile
	s.failures[profile] = append(s.failures[profile], ev)
}

func (s *fakeStore) QueryUsage(_ context.Context, q contracts.UsageQuery) ([]contracts.UsageRow, error) {
	var rows []contracts.UsageRow
	for _, ev := range s.usage {
		if q.Profile != "" && ev.profile != q.Profile {
			continue
		}
		if !q.Range.Start.IsZero() && ev.at.Before(q.Range.Start) {
			continue
		}
		if !q.Range.End.IsZero() && ev.at.After(q.Range.End) {
			continue
		}
		rows = append(rows, contracts.UsageRow{
			Profile: ev.profile,
			Model:   ev.model,
			Day:     ev.at,
			Usage:   ev.usage,
		})
	}
	return rows, nil
}

func (s *fakeStore) QueryRecentFailures(_ context.Context, profileName string, since time.Time) ([]contracts.HookEvent, error) {
	failures := append([]contracts.HookEvent(nil), s.failures[profileName]...)
	failures = slices.DeleteFunc(failures, func(ev contracts.HookEvent) bool {
		return ev.Timestamp.Before(since)
	})
	slices.SortFunc(failures, func(a, b contracts.HookEvent) int {
		return b.Timestamp.Compare(a.Timestamp)
	})
	return failures, nil
}

func (s *fakeStore) QuerySessions(context.Context, contracts.SessionQuery) ([]contracts.SessionTelemetry, error) {
	return nil, nil
}

func (s *fakeStore) GetProfileHealth(_ context.Context, profileName string) (contracts.ProfileHealth, error) {
	if health, ok := s.health[profileName]; ok {
		return health, nil
	}
	return contracts.ProfileHealth{}, contracts.ErrProfileNotFound
}

type fakePricing map[string]float64

func (p fakePricing) Cost(model string, _ time.Time, _ contracts.Usage) (float64, error) {
	return p[model], nil
}

func (p fakePricing) LastUpdated() time.Time { return time.Time{} }
