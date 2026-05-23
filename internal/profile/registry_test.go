package profile

import (
	"reflect"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestRegistryEncodeDecodeRoundtrip(t *testing.T) {
	in := registry{
		Profiles: []contracts.Profile{
			{
				Name:       "work",
				ConfigDir:  "/home/u/.claude-profiles/work",
				Label:      "Work",
				Color:      "#3B82F6",
				CreatedAt:  time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
				LastUsedAt: time.Date(2026, 5, 19, 15, 30, 0, 0, time.UTC),
			},
			{
				Name:       "personal",
				ConfigDir:  "/home/u/.claude-profiles/personal",
				CreatedAt:  time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
				LastUsedAt: time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
			},
		},
	}

	data, err := encodeRegistry(in)
	if err != nil {
		t.Fatalf("encodeRegistry: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("encodeRegistry returned empty bytes")
	}

	out, err := decodeRegistry(data)
	if err != nil {
		t.Fatalf("decodeRegistry: %v", err)
	}

	if len(out.Profiles) != len(in.Profiles) {
		t.Fatalf("len mismatch: got %d, want %d", len(out.Profiles), len(in.Profiles))
	}
	for i := range in.Profiles {
		if !out.Profiles[i].CreatedAt.Equal(in.Profiles[i].CreatedAt) {
			t.Errorf("profile[%d] CreatedAt: got %v, want %v", i, out.Profiles[i].CreatedAt, in.Profiles[i].CreatedAt)
		}
		if !out.Profiles[i].LastUsedAt.Equal(in.Profiles[i].LastUsedAt) {
			t.Errorf("profile[%d] LastUsedAt: got %v, want %v", i, out.Profiles[i].LastUsedAt, in.Profiles[i].LastUsedAt)
		}
		// Compare the time-independent fields.
		a, b := in.Profiles[i], out.Profiles[i]
		a.CreatedAt, a.LastUsedAt = time.Time{}, time.Time{}
		b.CreatedAt, b.LastUsedAt = time.Time{}, time.Time{}
		if a != b {
			t.Errorf("profile[%d] mismatch:\n got  %+v\n want %+v", i, b, a)
		}
	}
}

func TestDecodeRegistryEmptyBytes(t *testing.T) {
	out, err := decodeRegistry(nil)
	if err != nil {
		t.Fatalf("decodeRegistry(nil): %v", err)
	}
	if len(out.Profiles) != 0 {
		t.Errorf("empty input should yield 0 profiles, got %d", len(out.Profiles))
	}
}

func TestRegistryRoundTripsProfileLimits(t *testing.T) {
	suggest := false
	in := registry{
		Profiles: []contracts.Profile{
			{
				Name:      "work",
				ConfigDir: "/home/u/.claude-profiles/work",
				Limits: contracts.ProfileLimits{
					DailyTokenBudget:  100000,
					WeeklyTokenBudget: 500000,
					MonthlyUSDBudget:  250.75,
					Priority:          -5,
					SuggestEnabled:    &suggest,
					RateLimitCooldown: "1h30m",
				},
			},
		},
	}

	data, err := encodeRegistry(in)
	if err != nil {
		t.Fatalf("encodeRegistry: %v", err)
	}

	out, err := decodeRegistry(data)
	if err != nil {
		t.Fatalf("decodeRegistry: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("registry limits roundtrip mismatch:\n got  %+v\n want %+v\nTOML:\n%s", out, in, data)
	}
	if out.Profiles[0].Limits.SuggestEnabled == nil {
		t.Fatalf("SuggestEnabled pointer was not preserved")
	}
	if *out.Profiles[0].Limits.SuggestEnabled {
		t.Fatalf("SuggestEnabled = true, want false")
	}
}
