package cli_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupQuotaProfile registers one ccx profile under a fresh HOME and returns
// its config directory. Inline-defined because it is specific to migration tests.
func setupQuotaProfile(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "demo", "--config-dir", cfgDir)

	return cfgDir
}

func TestMigrateDryRunPrintsPlanWithoutTouchingDisk(t *testing.T) {
	profileDir := setupQuotaProfile(t)
	// Remove the projects symlink that `ccx profile add` creates (Task 4)
	// so this test exercises the missing-projects to plan path.
	_ = os.RemoveAll(filepath.Join(profileDir, "projects"))

	out := runCLI(t, "migrate-shared-history", "--dry-run")
	if !strings.Contains(out, "symlink") {
		t.Errorf("expected 'symlink' in plan output; got:\n%s", out)
	}
	if _, err := os.Lstat(filepath.Join(profileDir, "projects")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("dry-run created %s/projects; should not", profileDir)
	}
}

func TestMigrateDryRunDoesNotCreateStateDB(t *testing.T) {
	profileDir := setupQuotaProfile(t)
	home := filepath.Dir(profileDir)
	_ = os.RemoveAll(filepath.Join(profileDir, "projects"))
	if err := os.Remove(filepath.Join(home, ".ccx", "state.db")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}

	runCLI(t, "migrate-shared-history", "--dry-run")

	if _, err := os.Stat(filepath.Join(home, ".ccx", "state.db")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dry-run should not create state.db; stat err=%v", err)
	}
}

func TestMigrateDryRunIgnoresCorruptStateDB(t *testing.T) {
	profileDir := setupQuotaProfile(t)
	home := filepath.Dir(profileDir)
	_ = os.RemoveAll(filepath.Join(profileDir, "projects"))
	if err := os.WriteFile(filepath.Join(home, ".ccx", "state.db"), []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := runCLI(t, "migrate-shared-history", "--dry-run")

	if !strings.Contains(out, "symlink") {
		t.Errorf("expected 'symlink' in plan output; got:\n%s", out)
	}
}

func TestMigrateRejectsUnexpectedArgs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	_, stderr, code := runCLIResult([]string{"migrate-shared-history", "typo"})
	if code == 0 {
		t.Fatal("expected non-zero exit for unexpected arg")
	}
	if !strings.Contains(stderr, "unknown command") && !strings.Contains(stderr, "accepts 0 arg") {
		t.Fatalf("stderr = %q, want argument error", stderr)
	}
}

func TestMigrateApplyCreatesSymlink(t *testing.T) {
	profileDir := setupQuotaProfile(t)
	_ = os.RemoveAll(filepath.Join(profileDir, "projects"))

	stdout, stderr, code := runCLIResult([]string{"migrate-shared-history"})
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
		t.Errorf("expected symlink at %s/projects", profileDir)
	}
}

func isSymlinkUnavailableError(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "create symlink") &&
		(strings.Contains(s, "developer mode") ||
			strings.Contains(s, "elevated shell") ||
			strings.Contains(s, "privilege") ||
			strings.Contains(s, "not supported"))
}
