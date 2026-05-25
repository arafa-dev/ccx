package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/scanner"
	"github.com/arafa-dev/ccx/internal/storage"
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

func TestUsageSharedProjectsAttributesBySessionOwner(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	ccxHome := filepath.Join(home, ".ccx")
	sharedRoot := filepath.Join(ccxHome, "shared-projects")
	sessionPath := filepath.Join(sharedRoot, "sample-project", "s1.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o700); err != nil {
		t.Fatal(err)
	}
	session := `{"type":"assistant","uuid":"evt-shared-001","sessionId":"s1","timestamp":` +
		`"` + time.Now().UTC().Format(time.RFC3339) + `",` +
		`"message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":50}}}` + "\n"
	if err := os.WriteFile(sessionPath, []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(home, "claude-work")
	personalDir := filepath.Join(home, "claude-personal")
	for _, dir := range []string{workDir, personalDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(sharedRoot, filepath.Join(dir, "projects")); err != nil {
			t.Skipf("symlink creation unavailable on this host: %v", err)
		}
	}

	mgr, err := profile.NewManager(ccxHome)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []contracts.Profile{
		{Name: "work", ConfigDir: workDir},
		{Name: "personal", ConfigDir: personalDir},
	} {
		if err := mgr.Add(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	store, err := storage.NewStore(ctx, filepath.Join(ccxHome, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, contracts.Profile{Name: "work", ConfigDir: workDir}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, contracts.Profile{Name: "personal", ConfigDir: personalDir}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSessionTelemetry(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "SessionStart",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	out := runCLI(t, "usage", "--since", "365d", "--json")
	var parsed struct {
		Rows []contracts.UsageRow `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if len(parsed.Rows) != 1 {
		t.Fatalf("rows = %+v, want one row attributed to session owner", parsed.Rows)
	}
	if parsed.Rows[0].Profile != "work" {
		t.Fatalf("row profile = %q, want work", parsed.Rows[0].Profile)
	}

	store, err = storage.NewStore(ctx, filepath.Join(ccxHome, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	offset, inode, err := store.GetCursor(ctx, scanner.SharedCursorProfile, sessionPath)
	if err != nil {
		t.Fatalf("GetCursor shared: %v", err)
	}
	if offset == 0 || inode == 0 {
		t.Fatalf("shared cursor = (%d, %d), want persisted non-zero cursor", offset, inode)
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
