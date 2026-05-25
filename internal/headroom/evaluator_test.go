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
	store.addWorkFailure(contracts.HookEvent{
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

func TestRateLimitFailuresUseMaxActiveCooldownUntil(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addWorkFailure(contracts.HookEvent{
		Event:     "StopFailure",
		Timestamp: now.Add(-30 * time.Minute),
		Error:     "rate_limit",
	})
	store.addWorkFailure(contracts.HookEvent{
		Event:     "StopFailure",
		Timestamp: now.Add(-2 * time.Hour),
		Error:     "rate_limit",
	})

	result := evaluate(t, store, []contracts.Profile{
		profile("work", contracts.ProfileLimits{DailyTokenBudget: 1000, RateLimitCooldown: "3h"}),
	})

	got := mustCandidate(t, result, "work")
	wantCooldown := now.Add(150 * time.Minute)
	if got.CooldownUntil == nil || !got.CooldownUntil.Equal(wantCooldown) {
		t.Fatalf("CooldownUntil = %v, want later active expiry %v", got.CooldownUntil, wantCooldown)
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
			store.addWorkFailure(contracts.HookEvent{
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

func TestAuthenticationFailureIsResolvedByLaterSuccessfulSession(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addWorkFailure(contracts.HookEvent{
		Event:     "StopFailure",
		Timestamp: now.Add(-2 * time.Hour),
		Error:     "authentication_failed",
	})
	store.addWorkSession(contracts.SessionTelemetry{
		Profile:    "work",
		Session:    "recovered",
		StartedAt:  now.Add(-time.Hour),
		EndedAt:    now.Add(-50 * time.Minute),
		LastSeenAt: now.Add(-50 * time.Minute),
		Status:     "completed",
	})

	result := evaluate(t, store, []contracts.Profile{
		profile("work", contracts.ProfileLimits{DailyTokenBudget: 1000}),
	})
	got := mustCandidate(t, result, "work")
	if !got.Available {
		t.Fatalf("candidate available = false, want recovered auth failure available: %+v", got)
	}
	if result.Recommendation == nil || result.Recommendation.Profile != "work" {
		t.Fatalf("recommendation = %+v, want work", result.Recommendation)
	}
}

func TestAuthenticationFailureIsResolvedByLaterEndedSession(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addWorkFailure(contracts.HookEvent{
		Event:     "StopFailure",
		Timestamp: now.Add(-2 * time.Hour),
		Error:     "authentication_failed",
	})
	store.addWorkSession(contracts.SessionTelemetry{
		Profile:    "work",
		Session:    "recovered",
		StartedAt:  now.Add(-time.Hour),
		EndedAt:    now.Add(-50 * time.Minute),
		LastSeenAt: now.Add(-50 * time.Minute),
		Status:     "ended",
	})

	result := evaluate(t, store, []contracts.Profile{
		profile("work", contracts.ProfileLimits{DailyTokenBudget: 1000}),
	})
	got := mustCandidate(t, result, "work")
	if !got.Available {
		t.Fatalf("candidate available = false, want ended session to recover auth failure: %+v", got)
	}
}

func TestAuthenticationFailuresRespectIncludeUnavailable(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addWorkFailure(contracts.HookEvent{
		Event:     "StopFailure",
		Timestamp: now.Add(-time.Hour),
		Error:     "oauth_org_not_allowed",
	})

	result := evaluateWithOptions(t, store, []contracts.Profile{
		profile("work", contracts.ProfileLimits{DailyTokenBudget: 1000}),
	}, headroom.Options{IncludeUnavailable: true})
	got := mustCandidate(t, result, "work")
	if !got.Available {
		t.Fatalf("candidate available = false with IncludeUnavailable, want true: %+v", got)
	}
	if result.Recommendation == nil || result.Recommendation.Profile != "work" {
		t.Fatalf("recommendation = %+v, want work", result.Recommendation)
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

func TestFakeStoreQueriesTurnsInWindow(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	store.addTurn("work", now.Add(-4*time.Hour))
	store.addTurn("work", now.Add(-time.Hour))
	store.addTurn("personal", now.Add(-time.Hour))
	store.addTurn("work", now.Add(-6*time.Hour))

	since := now.Add(-5 * time.Hour)
	count, err := store.QueryTurnsInWindow(context.Background(), "work", since, now)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	oldest, err := store.QueryOldestTurnInWindow(context.Background(), "work", since, now)
	if err != nil {
		t.Fatalf("QueryOldestTurnInWindow: %v", err)
	}
	if want := now.Add(-4 * time.Hour); !oldest.Equal(want) {
		t.Fatalf("oldest = %s, want %s", oldest, want)
	}
}

func TestEvaluatePopulatesQuotaFieldsWhenPlanTierSet(t *testing.T) {
	cases := []struct {
		tier    string
		wantCap int
	}{
		{tier: "pro", wantCap: 45},
		{tier: "max5", wantCap: 225},
		{tier: "max20", wantCap: 900},
	}
	for _, tc := range cases {
		t.Run(tc.tier, func(t *testing.T) {
			now := testNow()
			store := newFakeStore(now)
			seedTurns(store, tc.tier, 3, 7)

			result := evaluate(t, store, []contracts.Profile{
				profile(tc.tier, contracts.ProfileLimits{PlanTier: tc.tier}),
			})
			c := mustCandidate(t, result, tc.tier)
			if c.Quota5h == nil {
				t.Fatal("Quota5h should be populated when PlanTier is set")
			}
			if c.Quota5h.Used != 3 || c.Quota5h.Cap != tc.wantCap {
				t.Errorf("Quota5h: got %+v, want Used=3 Cap=%d", *c.Quota5h, tc.wantCap)
			}
			if c.QuotaWeekly == nil || c.QuotaWeekly.Used != 7 {
				t.Errorf("QuotaWeekly: got %+v, want Used=7", c.QuotaWeekly)
			}
		})
	}
}

func TestEvaluateOmitsQuotaFieldsWhenPlanTierOptOut(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	seedTurns(store, "no-tier", 1, 1)
	seedTurns(store, "api-dev", 1, 1)

	result := evaluate(t, store, []contracts.Profile{
		profile("no-tier", contracts.ProfileLimits{}),
		profile("api-dev", contracts.ProfileLimits{PlanTier: "api"}),
	})
	for i := range result.Candidates {
		c := &result.Candidates[i]
		if c.Quota5h != nil {
			t.Errorf("%s Quota5h = %+v, want nil for quota opt-out", c.Profile, c.Quota5h)
		}
		if c.QuotaWeekly != nil {
			t.Errorf("%s QuotaWeekly = %+v, want nil for quota opt-out", c.Profile, c.QuotaWeekly)
		}
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

func seedTurns(s *fakeStore, profile string, count5h, countWeekly int) {
	for i := 0; i < count5h; i++ {
		s.addTurn(profile, s.now.Add(-time.Duration(i+1)*time.Minute))
	}
	extra := countWeekly - count5h
	for i := 0; i < extra; i++ {
		s.addTurn(profile, s.now.Add(-6*time.Hour-time.Duration(i+1)*time.Minute))
	}
}

type fakeStore struct {
	now      time.Time
	usage    []usageEvent
	turns    []turnEvent
	failures map[string][]contracts.HookEvent
	sessions map[string][]contracts.SessionTelemetry
	health   map[string]contracts.ProfileHealth
}

type usageEvent struct {
	profile string
	at      time.Time
	model   string
	usage   contracts.Usage
}

type turnEvent struct {
	profile string
	at      time.Time
}

func newFakeStore(now time.Time) *fakeStore {
	return &fakeStore{
		now:      now,
		failures: make(map[string][]contracts.HookEvent),
		sessions: make(map[string][]contracts.SessionTelemetry),
		health:   make(map[string]contracts.ProfileHealth),
	}
}

func (s *fakeStore) addUsage(profile string, at time.Time, model string, usage contracts.Usage) {
	s.usage = append(s.usage, usageEvent{profile: profile, at: at, model: model, usage: usage})
}

func (s *fakeStore) addWorkFailure(ev contracts.HookEvent) {
	ev.Profile = "work"
	s.failures["work"] = append(s.failures["work"], ev)
}

func (s *fakeStore) addWorkSession(session contracts.SessionTelemetry) {
	session.Profile = "work"
	s.sessions["work"] = append(s.sessions["work"], session)
}

func (s *fakeStore) addTurn(profile string, at time.Time) {
	s.turns = append(s.turns, turnEvent{profile: profile, at: at})
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

func (s *fakeStore) QueryTurnsInWindow(_ context.Context, profile string, since, until time.Time) (int, error) {
	n := 0
	for _, turn := range s.turns {
		if turn.profile != profile {
			continue
		}
		if turn.at.Before(since) || turn.at.After(until) {
			continue
		}
		n++
	}
	return n, nil
}

func (s *fakeStore) QueryOldestTurnInWindow(_ context.Context, profile string, since, until time.Time) (time.Time, error) {
	var oldest time.Time
	for _, turn := range s.turns {
		if turn.profile != profile {
			continue
		}
		if turn.at.Before(since) || turn.at.After(until) {
			continue
		}
		if oldest.IsZero() || turn.at.Before(oldest) {
			oldest = turn.at
		}
	}
	return oldest, nil
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

func (s *fakeStore) QuerySessions(_ context.Context, q contracts.SessionQuery) ([]contracts.SessionTelemetry, error) {
	sessions := append([]contracts.SessionTelemetry(nil), s.sessions[q.Profile]...)
	sessions = slices.DeleteFunc(sessions, func(session contracts.SessionTelemetry) bool {
		if q.Status != "" && session.Status != q.Status {
			return true
		}
		return !q.Since.IsZero() && session.LastSeenAt.Before(q.Since)
	})
	slices.SortFunc(sessions, func(a, b contracts.SessionTelemetry) int {
		return b.LastSeenAt.Compare(a.LastSeenAt)
	})
	if q.Limit > 0 && len(sessions) > q.Limit {
		sessions = sessions[:q.Limit]
	}
	return sessions, nil
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
