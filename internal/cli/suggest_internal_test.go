package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
)

func TestRenderSuggestIncludesQuotaColumn(t *testing.T) {
	pct := 16.0
	used := 142
	cap := 900
	resets := time.Now().Add(time.Hour)
	result := headroom.Result{
		Recommendation: &headroom.Candidate{
			Profile:         "work",
			Available:       true,
			Score:           89.5,
			HeadroomPercent: 84.0,
			AuthStatus:      "ok",
			Reasons:         []string{"5h turns 142/900 (16%)"},
			Quota5h:         &contracts.QuotaWindow{Used: used, Cap: cap, Pct: pct, ResetsAt: resets},
		},
		Candidates: []headroom.Candidate{
			{
				Profile:         "work",
				Available:       true,
				Score:           89.5,
				HeadroomPercent: 84.0,
				AuthStatus:      "ok",
				Reasons:         []string{"5h turns 142/900 (16%)"},
				Quota5h:         &contracts.QuotaWindow{Used: used, Cap: cap, Pct: pct, ResetsAt: resets},
			},
		},
	}
	var buf bytes.Buffer
	if err := renderSuggest(&buf, result); err != nil {
		t.Fatalf("renderSuggest: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"5H", "142/900", "16%"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestFormatSuggestQuotaWindowDoesNotRoundBelowHardCapTo100(t *testing.T) {
	got := formatSuggestQuotaWindow(&contracts.QuotaWindow{Used: 899, Cap: 900, Pct: 99.888})
	if strings.Contains(got, "100") {
		t.Fatalf("formatSuggestQuotaWindow soft cap = %q, should not round to 100", got)
	}
	if strings.Contains(got, "⛔") {
		t.Fatalf("formatSuggestQuotaWindow soft cap = %q, should not show hard-cap marker", got)
	}
}
