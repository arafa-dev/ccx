package quota_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/quota"
)

type fakeTurnsStore struct {
	turns       map[string]int
	oldestTurns map[string]time.Time
	countErrs   map[string]error
	oldestErrs  map[string]error
	calls       []turnQuery
}

type turnQuery struct {
	kind    string
	profile string
	since   time.Time
	until   time.Time
}

func (s *fakeTurnsStore) QueryTurnsInWindow(_ context.Context, profile string, since, until time.Time) (int, error) {
	key := profileWindowAdapter(profile, until.Sub(since))
	s.calls = append(s.calls, turnQuery{kind: "count", profile: profile, since: since, until: until})
	if err := s.countErrs[key]; err != nil {
		return 0, err
	}
	return s.turns[key], nil
}

func (s *fakeTurnsStore) QueryOldestTurnInWindow(_ context.Context, profile string, since, until time.Time) (time.Time, error) {
	key := profileWindowAdapter(profile, until.Sub(since))
	s.calls = append(s.calls, turnQuery{kind: "oldest", profile: profile, since: since, until: until})
	if err := s.oldestErrs[key]; err != nil {
		return time.Time{}, err
	}
	return s.oldestTurns[key], nil
}

func profileWindowAdapter(profile string, window time.Duration) string {
	if window == quota.FiveHourWindow {
		return profile + ":5h"
	}
	return profile + ":weekly"
}

func TestComputeUsesDefaultCapsWhenOverrideZero(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	store := &fakeTurnsStore{
		turns: map[string]int{
			"work:5h": 142,
		},
		oldestTurns: map[string]time.Time{
			"work:5h": now.Add(-2 * time.Hour),
		},
	}
	computer := quota.Computer{
		Store: store,
		Now: func() time.Time {
			return now
		},
	}
	profile := contracts.Profile{
		Name: "work",
		Limits: contracts.ProfileLimits{
			PlanTier: "max20",
		},
	}

	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For returned error: %v", err)
	}

	if got.Profile != "work" {
		t.Errorf("Profile = %q, want work", got.Profile)
	}
	if got.PlanTier != "max20" {
		t.Errorf("PlanTier = %q, want max20", got.PlanTier)
	}
	if got.Window5h.Used != 142 {
		t.Errorf("Window5h.Used = %d, want 142", got.Window5h.Used)
	}
	if got.Window5h.Cap != 900 {
		t.Errorf("Window5h.Cap = %d, want 900", got.Window5h.Cap)
	}
	wantPct := float64(142) / float64(900) * 100
	if got.Window5h.Pct != wantPct {
		t.Errorf("Window5h.Pct = %v, want %v", got.Window5h.Pct, wantPct)
	}
}

func TestComputeOverridesCapsViaProfileLimits(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	store := &fakeTurnsStore{
		turns: map[string]int{
			"work:5h": 50,
		},
		oldestTurns: map[string]time.Time{
			"work:5h": now.Add(-time.Hour),
		},
	}
	computer := quota.Computer{
		Store: store,
		Now: func() time.Time {
			return now
		},
	}
	profile := contracts.Profile{
		Name: "work",
		Limits: contracts.ProfileLimits{
			PlanTier:    "max20",
			Caps5hTurns: 100,
		},
	}

	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For returned error: %v", err)
	}

	if got.Window5h.Cap != 100 {
		t.Errorf("Window5h.Cap = %d, want 100", got.Window5h.Cap)
	}
	if got.Window5h.Pct != 50 {
		t.Errorf("Window5h.Pct = %v, want 50", got.Window5h.Pct)
	}
}

func TestComputeIncludesWeeklyUsageCapsAndRollingReset(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	oldestWeekly := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	store := &fakeTurnsStore{
		turns: map[string]int{
			"work:weekly": 12,
		},
		oldestTurns: map[string]time.Time{
			"work:weekly": oldestWeekly,
		},
	}
	computer := quota.Computer{Store: store, Now: func() time.Time { return now }}
	profile := contracts.Profile{
		Name: "work",
		Limits: contracts.ProfileLimits{
			PlanTier:        "max20",
			CapsWeeklyTurns: 120,
		},
	}

	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For returned error: %v", err)
	}

	if got.WindowWeekly.Used != 12 {
		t.Errorf("WindowWeekly.Used = %d, want 12", got.WindowWeekly.Used)
	}
	if got.WindowWeekly.Cap != 120 {
		t.Errorf("WindowWeekly.Cap = %d, want 120", got.WindowWeekly.Cap)
	}
	if got.WindowWeekly.Pct != 10 {
		t.Errorf("WindowWeekly.Pct = %v, want 10", got.WindowWeekly.Pct)
	}
	if want := oldestWeekly.Add(quota.WeekWindow); !got.WindowWeekly.ResetsAt.Equal(want) {
		t.Errorf("WindowWeekly.ResetsAt = %s, want %s", got.WindowWeekly.ResetsAt, want)
	}
}

func TestComputeUsesAnchoredWeeklyReset(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	store := &fakeTurnsStore{turns: map[string]int{}, oldestTurns: map[string]time.Time{}}
	computer := quota.Computer{Store: store, Now: func() time.Time { return now }}
	profile := contracts.Profile{
		Name: "work",
		Limits: contracts.ProfileLimits{
			PlanTier:     "max20",
			WeeklyAnchor: "monday",
		},
	}

	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For returned error: %v", err)
	}

	want := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	if !got.WindowWeekly.ResetsAt.Equal(want) {
		t.Errorf("anchored weekly reset = %s, want %s", got.WindowWeekly.ResetsAt, want)
	}
}

func TestComputeEmptyPlanTierReturnsZeroCaps(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	store := &fakeTurnsStore{
		turns: map[string]int{
			"work:5h": 50,
		},
		oldestTurns: map[string]time.Time{
			"work:5h": now.Add(-time.Hour),
		},
	}
	computer := quota.Computer{
		Store: store,
		Now: func() time.Time {
			return now
		},
	}
	profile := contracts.Profile{Name: "work"}

	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For returned error: %v", err)
	}

	if got.Window5h.Cap != 0 {
		t.Errorf("Window5h.Cap = %d, want 0", got.Window5h.Cap)
	}
	if got.Window5h.Pct != 0 {
		t.Errorf("Window5h.Pct = %v, want 0", got.Window5h.Pct)
	}
}

func TestComputeEmptyPlanTierIgnoresOverrides(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	store := &fakeTurnsStore{
		turns: map[string]int{
			"work:5h": 50,
		},
		oldestTurns: map[string]time.Time{
			"work:5h": now.Add(-time.Hour),
		},
	}
	computer := quota.Computer{
		Store: store,
		Now: func() time.Time {
			return now
		},
	}
	profile := contracts.Profile{
		Name: "work",
		Limits: contracts.ProfileLimits{
			Caps5hTurns:     100,
			CapsWeeklyTurns: 200,
		},
	}

	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For returned error: %v", err)
	}

	if got.Window5h.Cap != 0 {
		t.Errorf("Window5h.Cap = %d, want 0", got.Window5h.Cap)
	}
	if got.WindowWeekly.Cap != 0 {
		t.Errorf("WindowWeekly.Cap = %d, want 0", got.WindowWeekly.Cap)
	}
	if got.Window5h.Pct != 0 {
		t.Errorf("Window5h.Pct = %v, want 0", got.Window5h.Pct)
	}
	if got.WindowWeekly.Pct != 0 {
		t.Errorf("WindowWeekly.Pct = %v, want 0", got.WindowWeekly.Pct)
	}
}

func TestComputeNoTurnsReturnsZeroFiveHourReset(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	computer := quota.Computer{
		Store: &fakeTurnsStore{turns: map[string]int{}, oldestTurns: map[string]time.Time{}},
		Now:   func() time.Time { return now },
	}
	profile := contracts.Profile{Name: "work", Limits: contracts.ProfileLimits{PlanTier: "max20"}}

	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For returned error: %v", err)
	}
	if !got.Window5h.ResetsAt.IsZero() {
		t.Errorf("Window5h.ResetsAt = %s, want zero time", got.Window5h.ResetsAt)
	}
}

func TestComputerForNilStoreReturnsError(t *testing.T) {
	_, err := (quota.Computer{}).For(context.Background(), contracts.Profile{Name: "work"})
	if err == nil {
		t.Fatal("For returned nil error, want nil store error")
	}
	if !strings.Contains(err.Error(), "Computer.Store is nil") {
		t.Fatalf("error = %v, want nil store context", err)
	}
}

func TestComputerForWrapsStoreErrors(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	boom := errors.New("boom")
	cases := []struct {
		name       string
		store      *fakeTurnsStore
		wantSubstr string
	}{
		{
			name:       "5h count",
			store:      &fakeTurnsStore{countErrs: map[string]error{"work:5h": boom}},
			wantSubstr: `5h count for "work"`,
		},
		{
			name:       "5h oldest",
			store:      &fakeTurnsStore{oldestErrs: map[string]error{"work:5h": boom}},
			wantSubstr: `oldest 5h for "work"`,
		},
		{
			name: "weekly count",
			store: &fakeTurnsStore{
				turns:     map[string]int{"work:5h": 1},
				countErrs: map[string]error{"work:weekly": boom},
			},
			wantSubstr: `weekly count for "work"`,
		},
		{
			name: "weekly oldest",
			store: &fakeTurnsStore{
				turns:      map[string]int{"work:5h": 1},
				oldestErrs: map[string]error{"work:weekly": boom},
			},
			wantSubstr: `oldest weekly for "work"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			computer := quota.Computer{Store: tc.store, Now: func() time.Time { return now }}
			_, err := computer.For(context.Background(), contracts.Profile{
				Name:   "work",
				Limits: contracts.ProfileLimits{PlanTier: "max20"},
			})
			if err == nil {
				t.Fatal("For returned nil error, want store error")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantSubstr)
			}
			if !errors.Is(err, boom) {
				t.Fatalf("error = %v, want wrapped boom", err)
			}
		})
	}
}

func TestComputerAllCollectsPerProfileFailures(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	store := &fakeTurnsStore{
		turns: map[string]int{"good:5h": 1},
		countErrs: map[string]error{
			"bad:5h": errors.New("bad profile"),
		},
	}
	computer := quota.Computer{Store: store, Now: func() time.Time { return now }}

	results, failures, err := computer.All(context.Background(), []contracts.Profile{
		{Name: "good", Limits: contracts.ProfileLimits{PlanTier: "max20"}},
		{Name: "bad", Limits: contracts.ProfileLimits{PlanTier: "max20"}},
	})
	if err != nil {
		t.Fatalf("All returned top-level error: %v", err)
	}
	if len(results) != 1 || results[0].Profile != "good" {
		t.Fatalf("results = %+v, want only good profile", results)
	}
	if len(failures) != 1 || failures["bad"] == nil {
		t.Fatalf("failures = %+v, want bad profile failure", failures)
	}
}

func TestComputerForUsesExpectedWindowBounds(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	store := &fakeTurnsStore{turns: map[string]int{}, oldestTurns: map[string]time.Time{}}
	computer := quota.Computer{Store: store, Now: func() time.Time { return now }}
	profile := contracts.Profile{
		Name: "work",
		Limits: contracts.ProfileLimits{
			PlanTier:     "max20",
			WeeklyAnchor: "monday",
		},
	}

	if _, err := computer.For(context.Background(), profile); err != nil {
		t.Fatalf("For returned error: %v", err)
	}

	want5hSince := now.Add(-quota.FiveHourWindow)
	wantWeeklySince := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	var saw5h, sawWeekly bool
	for _, call := range store.calls {
		if call.profile != "work" || !call.until.Equal(now) {
			t.Fatalf("unexpected call: %+v", call)
		}
		switch {
		case call.since.Equal(want5hSince):
			saw5h = true
		case call.since.Equal(wantWeeklySince):
			sawWeekly = true
		default:
			t.Fatalf("unexpected since bound: %+v", call)
		}
	}
	if !saw5h {
		t.Fatal("did not query 5h bounds")
	}
	if !sawWeekly {
		t.Fatal("did not query anchored weekly bounds")
	}
}
