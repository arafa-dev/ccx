package headroom_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/headroom"
)

func TestPressureLevelFromPct(t *testing.T) {
	cases := []struct {
		pct  float64
		want headroom.PressureLevel
	}{
		{0, headroom.PressureNone},
		{74.9, headroom.PressureNone},
		{75, headroom.PressureWarn},
		{89.9, headroom.PressureWarn},
		{90, headroom.PressureSoft},
		{99.9, headroom.PressureSoft},
		{100, headroom.PressureHard},
		{150, headroom.PressureHard},
	}
	for _, tc := range cases {
		got := headroom.PressureLevelFromPct(tc.pct)
		if got != tc.want {
			t.Errorf("PressureLevelFromPct(%v) = %v, want %v", tc.pct, got, tc.want)
		}
	}
}

func TestSoftPenalty(t *testing.T) {
	cases := []struct {
		pct  float64
		want float64
	}{
		{0, 0},
		{75, 0},
		{89, 0},
		{90, 0},
		{95, 10},
		{99, 18},
		{100, 20},
		{200, 20},
	}
	for _, tc := range cases {
		got := headroom.SoftPenalty(tc.pct)
		if got != tc.want {
			t.Errorf("SoftPenalty(%v) = %v, want %v", tc.pct, got, tc.want)
		}
	}
}
