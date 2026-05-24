package quota_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/quota"
)

func TestDefaultCapsKnownTiers(t *testing.T) {
	cases := []struct {
		tier       string
		want5h     int
		wantWeekly int
	}{
		{"pro", 45, 0},
		{"max5", 225, 0},
		{"max20", 900, 0},
		{"api", 0, 0},
		{"", 0, 0},
	}
	for _, tc := range cases {
		got5h, gotWeekly := quota.DefaultCaps(tc.tier)
		if got5h != tc.want5h {
			t.Errorf("DefaultCaps(%q).5h = %d, want %d", tc.tier, got5h, tc.want5h)
		}
		if gotWeekly != tc.wantWeekly {
			t.Errorf("DefaultCaps(%q).weekly = %d, want %d", tc.tier, gotWeekly, tc.wantWeekly)
		}
	}
}

func TestDefaultCapsUnknownTierReturnsZero(t *testing.T) {
	got5h, gotWeekly := quota.DefaultCaps("bedrock-premium")
	if got5h != 0 || gotWeekly != 0 {
		t.Errorf("unknown tier: got (%d, %d), want (0, 0)", got5h, gotWeekly)
	}
}
