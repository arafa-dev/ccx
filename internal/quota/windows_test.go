package quota_test

import (
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/quota"
)

func TestWindow5hBounds(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)

	gotSince, gotUntil := quota.Window5hBounds(now)
	wantSince := now.Add(-5 * time.Hour)

	if !gotSince.Equal(wantSince) {
		t.Errorf("Window5hBounds since = %s, want %s", gotSince, wantSince)
	}
	if !gotUntil.Equal(now) {
		t.Errorf("Window5hBounds until = %s, want %s", gotUntil, now)
	}
}

func TestWindowWeeklyBoundsRolling(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)

	gotSince, gotUntil := quota.WindowWeeklyBounds(now, "rolling")
	wantSince := now.Add(-7 * 24 * time.Hour)

	if !gotSince.Equal(wantSince) {
		t.Errorf("WindowWeeklyBounds rolling since = %s, want %s", gotSince, wantSince)
	}
	if !gotUntil.Equal(now) {
		t.Errorf("WindowWeeklyBounds rolling until = %s, want %s", gotUntil, now)
	}
}

func TestWindowWeeklyBoundsMondayAnchored(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)

	gotSince, gotUntil := quota.WindowWeeklyBounds(now, "monday")
	wantSince := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)

	if !gotSince.Equal(wantSince) {
		t.Errorf("WindowWeeklyBounds monday since = %s, want %s", gotSince, wantSince)
	}
	if !gotUntil.Equal(now) {
		t.Errorf("WindowWeeklyBounds monday until = %s, want %s", gotUntil, now)
	}
}

func TestWindowWeeklyBoundsEmptyAnchorDefaultsRolling(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)

	gotSince, gotUntil := quota.WindowWeeklyBounds(now, "")
	wantSince := now.Add(-7 * 24 * time.Hour)

	if !gotSince.Equal(wantSince) {
		t.Errorf("WindowWeeklyBounds empty anchor since = %s, want %s", gotSince, wantSince)
	}
	if !gotUntil.Equal(now) {
		t.Errorf("WindowWeeklyBounds empty anchor until = %s, want %s", gotUntil, now)
	}
}

func TestResetTime5h(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	oldest := now.Add(-4 * time.Hour)

	got := quota.ResetTime5h(oldest)
	want := oldest.Add(5 * time.Hour)

	if !got.Equal(want) {
		t.Errorf("ResetTime5h = %s, want %s", got, want)
	}
}

func TestResetTime5hZeroOldestReturnsZero(t *testing.T) {
	got := quota.ResetTime5h(time.Time{})
	if !got.IsZero() {
		t.Errorf("ResetTime5h zero oldest = %s, want zero time", got)
	}
}

func TestResetTimeWeeklyRolling(t *testing.T) {
	oldest := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)

	got := quota.ResetTimeWeekly(oldest, "rolling")
	want := oldest.Add(7 * 24 * time.Hour)

	if !got.Equal(want) {
		t.Errorf("ResetTimeWeekly rolling = %s, want %s", got, want)
	}
}

func TestResetTimeWeeklyZeroOldestReturnsZero(t *testing.T) {
	got := quota.ResetTimeWeekly(time.Time{}, "rolling")
	if !got.IsZero() {
		t.Errorf("ResetTimeWeekly zero oldest = %s, want zero time", got)
	}
}

func TestResetTimeWeeklyAnchoredUsesProvidedTimestamp(t *testing.T) {
	oldest := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)

	got := quota.ResetTimeWeekly(oldest, "monday")
	want := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)

	if !got.Equal(want) {
		t.Errorf("ResetTimeWeekly anchored = %s, want %s", got, want)
	}
}

func TestResetTimeWeeklyAnchoredMonday(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)

	got := quota.ResetTimeWeeklyAnchored(now, "monday")
	want := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)

	if !got.Equal(want) {
		t.Errorf("ResetTimeWeeklyAnchored monday = %s, want %s", got, want)
	}
}

func TestPct(t *testing.T) {
	cases := []struct {
		name string
		used int
		cap  int
		want float64
	}{
		{name: "zero used", used: 0, cap: 100, want: 0},
		{name: "half used", used: 50, cap: 100, want: 50},
		{name: "full used", used: 100, cap: 100, want: 100},
		{name: "clamps high", used: 150, cap: 100, want: 100},
		{name: "zero cap", used: 10, cap: 0, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quota.Pct(tc.used, tc.cap)
			if got != tc.want {
				t.Errorf("Pct(%d, %d) = %v, want %v", tc.used, tc.cap, got, tc.want)
			}
		})
	}
}
