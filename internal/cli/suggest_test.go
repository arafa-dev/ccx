package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/profile"
)

func TestProfileSetUpdatesOnlySuppliedFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	out := runCLI(
		t,
		"profile", "set", "work",
		"--label", "Work",
		"--color", "#00ff99",
		"--daily-tokens", "1000",
		"--weekly-tokens", "7000",
		"--monthly-usd", "25.5",
		"--priority", "4",
		"--suggestions", "enabled",
		"--rate-limit-cooldown", "90m",
	)
	if !strings.Contains(out, "Profile 'work' updated") {
		t.Fatalf("set output = %q, want success", out)
	}

	out = runCLI(
		t,
		"profile", "set", "work",
		"--label", "",
		"--color", "",
		"--daily-tokens", "2000",
		"--rate-limit-cooldown", "",
	)
	if !strings.Contains(out, "Profile 'work' updated") {
		t.Fatalf("second set output = %q, want success", out)
	}

	ccxHome, err := platform.CCXHome()
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := profile.NewManager(ccxHome)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := mgr.Get(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Label != "" || stored.Color != "" {
		t.Fatalf("label/color = %q/%q, want cleared", stored.Label, stored.Color)
	}
	if stored.Limits.DailyTokenBudget != 2000 ||
		stored.Limits.WeeklyTokenBudget != 7000 ||
		stored.Limits.MonthlyUSDBudget != 25.5 ||
		stored.Limits.Priority != 4 ||
		stored.Limits.RateLimitCooldown != "" ||
		stored.Limits.SuggestEnabled == nil ||
		!*stored.Limits.SuggestEnabled {
		t.Fatalf("stored limits = %+v, want only supplied fields changed", stored.Limits)
	}

	out = runCLI(t, "suggest", "--json")
	payload := decodeSuggestJSON(t, out)
	work := findSuggestCandidate(t, payload.Candidates, "work")
	if work.Profile != "work" || work.HeadroomPercent != 100 {
		t.Fatalf("work candidate = %+v, want 100%% headroom", work)
	}
	if work.Available != true {
		t.Fatalf("work available = false, want true")
	}
}

func TestProfileSetSuggestionsDisabledExcludesProfileFromSuggest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	workDir := filepath.Join(home, "claude-work")
	otherDir := filepath.Join(home, "claude-other")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", workDir)
	runCLI(t, "profile", "add", "other", "--config-dir", otherDir)
	runCLI(t, "profile", "set", "work", "--daily-tokens", "100000", "--priority", "100", "--suggestions", "disabled")
	runCLI(t, "profile", "set", "other", "--daily-tokens", "1000")

	out := runCLI(t, "suggest", "--json")
	payload := decodeSuggestJSON(t, out)
	if payload.Recommendation == nil || payload.Recommendation.Profile != "other" {
		t.Fatalf("recommendation = %+v, want other", payload.Recommendation)
	}
	work := findSuggestCandidate(t, payload.Candidates, "work")
	if work.Available {
		t.Fatalf("work available = true, want false because suggestions disabled")
	}
}

func TestSuggestJSONShapeIncludesRecommendationAndCandidates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)
	runCLI(t, "profile", "set", "work", "--daily-tokens", "1000")

	out := runCLI(t, "suggest", "--json")
	payload := decodeSuggestJSON(t, out)
	if payload.Recommendation == nil || payload.Recommendation.Profile != "work" {
		t.Fatalf("recommendation = %+v, want work", payload.Recommendation)
	}
	if len(payload.Candidates) != 1 {
		t.Fatalf("candidates length = %d, want 1", len(payload.Candidates))
	}
	candidate := payload.Candidates[0]
	if candidate.Profile == "" || candidate.AuthStatus == "" || len(candidate.Reasons) == 0 {
		t.Fatalf("candidate missing stable fields: %+v", candidate)
	}
}

func decodeSuggestJSON(t *testing.T, out string) headroom.Result {
	t.Helper()
	var payload headroom.Result
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid suggest JSON: %v\n%s", err, out)
	}
	return payload
}

func findSuggestCandidate(t *testing.T, candidates []headroom.Candidate, name string) headroom.Candidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Profile == name {
			return candidate
		}
	}
	t.Fatalf("candidate %q missing from %+v", name, candidates)
	return headroom.Candidate{}
}
