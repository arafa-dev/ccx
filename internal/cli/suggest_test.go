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

func TestSuggestBestEffortIngestsAndEvaluatesInaccessibleProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	validDir := filepath.Join(home, "claude-valid")
	badDir := filepath.Join(home, "claude-bad")
	if err := os.MkdirAll(filepath.Join(validDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(badDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "valid", "--config-dir", validDir)
	runCLI(t, "profile", "add", "bad", "--config-dir", badDir)
	runCLI(t, "profile", "set", "valid", "--daily-tokens", "1000")
	runCLI(t, "profile", "set", "bad", "--daily-tokens", "1000", "--priority", "100")
	if err := os.RemoveAll(badDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runCLI(t, "suggest", "--json")
	payload := decodeSuggestJSON(t, out)
	if payload.Recommendation == nil || payload.Recommendation.Profile != "valid" {
		t.Fatalf("recommendation = %+v, want valid", payload.Recommendation)
	}
	bad := findSuggestCandidate(t, payload.Candidates, "bad")
	if bad.Available {
		t.Fatalf("bad candidate available = true, want false")
	}
	if !candidateHasReason(bad, "config dir inaccessible") && !candidateHasReason(bad, "scan failed") {
		t.Fatalf("bad candidate reasons = %v, want config dir or scan failure", bad.Reasons)
	}
}

func TestSuggestEmptyRegistryReportsNoProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	stdout, stderr, code := runCLIResult([]string{"suggest", "--json"})
	if code == 0 {
		t.Fatalf("suggest --json exit = 0, want non-zero for empty registry")
	}
	payload := decodeSuggestJSON(t, stdout)
	if payload.Error != "no profiles registered" {
		t.Fatalf("json error = %q, want no profiles registered; stderr=%q", payload.Error, stderr)
	}
	if len(payload.Candidates) != 0 {
		t.Fatalf("candidates = %+v, want empty", payload.Candidates)
	}

	_, stderr, code = runCLIResult([]string{"suggest"})
	if code == 0 {
		t.Fatalf("suggest exit = 0, want non-zero for empty registry")
	}
	if !strings.Contains(stderr, "no profiles registered") {
		t.Fatalf("stderr = %q, want no profiles registered", stderr)
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

func candidateHasReason(candidate headroom.Candidate, want string) bool {
	for _, reason := range candidate.Reasons {
		if strings.Contains(reason, want) {
			return true
		}
	}
	return false
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
