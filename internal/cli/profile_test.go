package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/cli"
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
