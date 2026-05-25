package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
)

func TestUsageEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	out := runCLI(t, "usage")
	if !strings.Contains(out, "Total") {
		t.Errorf("expected 'Total' line in usage output: %q", out)
	}
}

func TestUsageJSONShape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	out := runCLI(t, "usage", "--json")
	var parsed struct {
		Rows  []any   `json:"rows"`
		Total float64 `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
}

func TestUsageIngestsEventsForRegisteredProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	projectDir := filepath.Join(cfgDir, "projects", "sample-project")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(projectDir, "session.jsonl")
	session := `{"type":"assistant","uuid":"evt-001","sessionId":"sess-001","timestamp":"2026-05-21T12:00:01Z","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":200}}}` + "\n"
	if err := os.WriteFile(sessionPath, []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := profile.NewManager(filepath.Join(home, ".ccx"))
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add(context.Background(), contracts.Profile{Name: "work", ConfigDir: cfgDir}); err != nil {
		t.Fatal(err)
	}

	out := runCLI(t, "usage", "--since", "365d")
	if !strings.Contains(out, "work") {
		t.Fatalf("usage output missing profile: %q", out)
	}
}

func TestUsageQuotaFlagPrintsHeaders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)
	runCLI(t, "profile", "set", "work", "--plan-tier", "max20")

	out := runCLI(t, "usage", "--quota")
	for _, want := range []string{"PROFILE", "PLAN", "5H WINDOW", "WEEKLY WINDOW"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected header %q in output:\n%s", want, out)
		}
	}
}

func TestProfileSetPlanTierRejectsUnknownTier(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	stdout, stderr, code := runCLIResult([]string{"profile", "set", "work", "--plan-tier", "mxa20"})
	if code == 0 {
		t.Fatalf("profile set succeeded; stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stderr, "--plan-tier") {
		t.Fatalf("stderr = %q, want --plan-tier validation error", stderr)
	}
}
