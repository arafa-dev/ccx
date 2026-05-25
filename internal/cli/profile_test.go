package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/cli"
	profilepkg "github.com/arafa-dev/ccx/internal/profile"
)

func TestProfileAddListRm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}

	out := runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)
	if !strings.Contains(out, "Profile 'work' added") {
		t.Errorf("add output: %q", out)
	}

	out = runCLI(t, "profile", "list")
	if !strings.Contains(out, "work") {
		t.Errorf("list missing 'work': %q", out)
	}

	out = runCLI(t, "profile", "rm", "work")
	if !strings.Contains(out, "removed") {
		t.Errorf("rm output: %q", out)
	}
}

func TestProfileAddCreatesSharedSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profileDir := filepath.Join(home, "claude-demo")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLIResult([]string{"profile", "add", "demo", "--config-dir", profileDir})
	if code != 0 {
		if isSymlinkUnavailableError(stderr) {
			t.Skipf("symlink creation unavailable on this host: %s", stderr)
		}
		t.Fatalf("exit %d: stdout=%q stderr=%q", code, stdout, stderr)
	}

	info, err := os.Lstat(filepath.Join(profileDir, "projects"))
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("ccx profile add should create symlink at %s/projects", profileDir)
	}
	target, err := os.Readlink(filepath.Join(profileDir, "projects"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if !strings.HasSuffix(target, "shared-projects") {
		t.Errorf("symlink target = %q, want suffix shared-projects", target)
	}
}

func TestProfileAddStoresQuotaFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profileDir := filepath.Join(home, "claude-demo")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}

	runCLI(
		t,
		"profile", "add", "demo",
		"--config-dir", profileDir,
		"--plan-tier", "max5",
		"--weekly-anchor", "monday",
		"--caps-5h-turns", "1",
		"--caps-weekly-turns", "7",
	)

	mgr, err := profilepkg.NewManager(filepath.Join(home, ".ccx"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.Get(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Limits.PlanTier != "max5" {
		t.Errorf("PlanTier = %q, want max5", got.Limits.PlanTier)
	}
	if got.Limits.WeeklyAnchor != "monday" {
		t.Errorf("WeeklyAnchor = %q, want monday", got.Limits.WeeklyAnchor)
	}
	if got.Limits.Caps5hTurns != 1 {
		t.Errorf("Caps5hTurns = %d, want 1", got.Limits.Caps5hTurns)
	}
	if got.Limits.CapsWeeklyTurns != 7 {
		t.Errorf("CapsWeeklyTurns = %d, want 7", got.Limits.CapsWeeklyTurns)
	}
}

func TestProfileSetUpdatesQuotaFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profileDir := filepath.Join(home, "claude-demo")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}

	runCLI(t, "profile", "add", "demo", "--config-dir", profileDir)
	runCLI(
		t,
		"profile", "set", "demo",
		"--plan-tier", "max20",
		"--weekly-anchor", "friday",
		"--caps-5h-turns", "3",
		"--caps-weekly-turns", "21",
	)

	mgr, err := profilepkg.NewManager(filepath.Join(home, ".ccx"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.Get(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Limits.PlanTier != "max20" {
		t.Errorf("PlanTier = %q, want max20", got.Limits.PlanTier)
	}
	if got.Limits.WeeklyAnchor != "friday" {
		t.Errorf("WeeklyAnchor = %q, want friday", got.Limits.WeeklyAnchor)
	}
	if got.Limits.Caps5hTurns != 3 {
		t.Errorf("Caps5hTurns = %d, want 3", got.Limits.Caps5hTurns)
	}
	if got.Limits.CapsWeeklyTurns != 21 {
		t.Errorf("CapsWeeklyTurns = %d, want 21", got.Limits.CapsWeeklyTurns)
	}

	runCLI(t, "profile", "set", "demo", "--weekly-anchor", "rolling", "--caps-5h-turns", "0")
	got, err = mgr.Get(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Limits.WeeklyAnchor != "" {
		t.Errorf("WeeklyAnchor after rolling = %q, want empty rolling default", got.Limits.WeeklyAnchor)
	}
	if got.Limits.Caps5hTurns != 0 {
		t.Errorf("Caps5hTurns after clear = %d, want 0", got.Limits.Caps5hTurns)
	}
	if got.Limits.CapsWeeklyTurns != 21 {
		t.Errorf("CapsWeeklyTurns changed unexpectedly to %d", got.Limits.CapsWeeklyTurns)
	}
}

func TestProfileQuotaFlagsValidateInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profileDir := filepath.Join(home, "claude-demo")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLIResult([]string{"profile", "add", "demo", "--config-dir", profileDir, "--weekly-anchor", "someday"})
	if code == 0 {
		t.Fatalf("profile add should reject invalid weekly anchor; stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "--weekly-anchor") {
		t.Fatalf("stderr = %q, want --weekly-anchor validation", stderr)
	}

	runCLI(t, "profile", "add", "demo", "--config-dir", profileDir)
	_, stderr, code = runCLIResult([]string{"profile", "set", "demo", "--caps-5h-turns", "-1"})
	if code == 0 {
		t.Fatalf("profile set should reject negative caps; stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "--caps-5h-turns") {
		t.Fatalf("stderr = %q, want --caps-5h-turns validation", stderr)
	}
}

func TestProfileAddDoesNotRegisterWhenSharedSymlinkPlanFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profileDir := filepath.Join(home, "claude-demo")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "projects"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, code := runCLIResult([]string{"profile", "add", "demo", "--config-dir", profileDir})
	if code == 0 {
		t.Fatal("expected profile add to fail when projects path is not a directory")
	}

	out := runCLI(t, "profile", "list")
	if strings.Contains(out, "demo") {
		t.Fatalf("profile should not be registered after migration planning failure:\n%s", out)
	}
}

func TestProfileAddRollsBackRegistrationWhenSharedSymlinkApplyFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profileDir := filepath.Join(home, "claude-demo")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ccx"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ccx", "shared-projects"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, code := runCLIResult([]string{"profile", "add", "demo", "--config-dir", profileDir})
	if code == 0 {
		t.Fatal("expected profile add to fail when shared-projects cannot be created")
	}

	out := runCLI(t, "profile", "list")
	if strings.Contains(out, "demo") {
		t.Fatalf("profile should not be registered after migration apply failure:\n%s", out)
	}
}

func TestProfileListReportsActiveProfileErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)
	t.Setenv("CCX_ACTIVE_PROFILE", "missing")

	_, stderr, code := runCLIResult([]string{"profile", "list"})
	if code == 0 {
		t.Fatalf("expected non-zero exit for missing active profile")
	}
	if !strings.Contains(stderr, "profile \"missing\"") {
		t.Fatalf("expected missing active profile error, got %q", stderr)
	}
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, code := runCLIResult(args)
	if code != 0 {
		t.Fatalf("exit %d: stdout=%q stderr=%q", code, stdout, stderr)
	}
	return stdout
}

func runCLIResult(args []string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	return stdout.String(), stderr.String(), code
}
