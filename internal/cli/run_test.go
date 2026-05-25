package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/cli"
	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/daemon"
	"github.com/arafa-dev/ccx/internal/storage"
)

func TestRunPrintOnlyEmitsPlanWithoutForking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	binDir := t.TempDir()
	_ = createFakeClaude(t, binDir, 0)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	stdout := runCLI(t, "run", "--print-only", "--profile", "work", "--", "--help")
	if !strings.Contains(stdout, "profile=work") {
		t.Fatalf("stdout missing profile: %q", stdout)
	}
	if !strings.Contains(stdout, "CLAUDE_CONFIG_DIR="+cfgDir) {
		t.Fatalf("stdout missing CLAUDE_CONFIG_DIR: %q", stdout)
	}
	if !strings.Contains(stdout, "CCX_ACTIVE_PROFILE=work") {
		t.Fatalf("stdout missing CCX_ACTIVE_PROFILE: %q", stdout)
	}
	if !strings.Contains(stdout, "args=--help") {
		t.Fatalf("stdout missing args: %q", stdout)
	}
}

func TestRunPrintOnlyQuotesArgs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	binDir := t.TempDir()
	_ = createFakeClaude(t, binDir, 0)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	stdout := runCLI(t, "run", "--quiet", "--print-only", "--profile", "work", "--", "two words")
	if !strings.Contains(stdout, `args="two words"`) {
		t.Fatalf("stdout missing quoted args: %q", stdout)
	}
	if !strings.Contains(stdout, "CLAUDE_CONFIG_DIR="+cfgDir) {
		t.Fatalf("stdout missing CLAUDE_CONFIG_DIR: %q", stdout)
	}
}

func TestRunDefaultRationaleOmitsConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	binDir := t.TempDir()
	_ = createFakeClaude(t, binDir, 0)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	stdout, stderr, code := runCLIResult([]string{"run", "--print-only", "--profile", "work"})
	if code != 0 {
		t.Fatalf("run exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "CLAUDE_CONFIG_DIR="+cfgDir) {
		t.Fatalf("stdout missing print-only config dir: %q", stdout)
	}
	if strings.Contains(stderr, cfgDir) || strings.Contains(stderr, "config_dir=") {
		t.Fatalf("default rationale leaked config dir: %q", stderr)
	}
}

func TestRunNoProfilesErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	_, stderr, code := runCLIResult([]string{"run", "--print-only"})
	if code == 0 {
		t.Fatal("expected non-zero exit with no profiles")
	}
	if !strings.Contains(stderr, "no profiles") {
		t.Fatalf("stderr missing no profiles message: %q", stderr)
	}
}

func TestRunPropagatesChildExitCodeWithoutDefaultErrorLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	binary := createFakeClaude(t, t.TempDir(), 7)
	_, stderr, code := runCLIResult([]string{"run", "--profile", "work", "--claude-binary", binary})
	if code != 7 {
		t.Fatalf("exit code: got %d, want 7; stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, "Error: exit 7") {
		t.Fatalf("stderr contains default error line: %q", stderr)
	}
}

func TestRunLaunchPassesSelectedEnvAndArgsToChild(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	capture := filepath.Join(t.TempDir(), "capture.txt")
	binary := createCaptureClaude(t, t.TempDir(), capture)
	_, stderr, code := runCLIResult([]string{"run", "--quiet", "--profile", "work", "--claude-binary", binary, "--", "alpha", "two words"})
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0; stderr=%q", code, stderr)
	}
	got, err := os.ReadFile(capture) //nolint:gosec // Test reads its own temp capture file.
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"CLAUDE_CONFIG_DIR=" + cfgDir,
		"CCX_ACTIVE_PROFILE=work",
		"ARGS=alpha|two words",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("capture missing %q:\n%s", want, got)
		}
	}
}

func TestRunSuperviseLaunchesSelectedEnvAndArgsToChild(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	capture := filepath.Join(t.TempDir(), "capture.txt")
	binary := createCaptureClaude(t, t.TempDir(), capture)
	_, stderr, code := runCLIResult([]string{"run", "--quiet", "--supervise", "--profile", "work", "--claude-binary", binary, "--", "alpha", "two words"})
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "recommendation stream unavailable") {
		t.Fatalf("stderr = %q, want degraded supervisor warning", stderr)
	}
	got, err := os.ReadFile(capture) //nolint:gosec // Test reads its own temp capture file.
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"CLAUDE_CONFIG_DIR=" + cfgDir,
		"CCX_ACTIVE_PROFILE=work",
		"ARGS=alpha|two words",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("capture missing %q:\n%s", want, got)
		}
	}
}

func TestRunSuperviseRejectsTooLowPollInterval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	_, stderr, code := runCLIResult([]string{"run", "--quiet", "--supervise", "--poll-interval", "10ms", "--profile", "work", "--claude-binary", createFakeClaude(t, t.TempDir(), 0)})
	if code == 0 {
		t.Fatal("expected non-zero exit for too-low poll interval")
	}
	if !strings.Contains(stderr, "at least 250ms") {
		t.Fatalf("stderr = %q, want poll interval floor", stderr)
	}
}

func TestRunExplicitProfileDoesNotScanBeforeLaunch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	binDir := t.TempDir()
	_ = createFakeClaude(t, binDir, 0)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfgDir := filepath.Join(home, "claude-work")
	writeRunUsageJSONL(t, cfgDir)
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	stdout := runCLI(t, "run", "--quiet", "--print-only", "--profile", "work")
	if !strings.Contains(stdout, "profile=work") {
		t.Fatalf("run output = %q, want explicit profile", stdout)
	}
	assertNoRunUsageRows(t, home)
}

func TestRunSkipsScanWhenDaemonRunning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	binDir := t.TempDir()
	_ = createFakeClaude(t, binDir, 0)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfgDir := filepath.Join(home, "claude-work")
	writeRunUsageJSONL(t, cfgDir)
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	root := filepath.Join(home, ".ccx")
	proc := newCLIFakeProcess(root)
	proc.setAlive(2468, true)
	writeDaemonRuntime(t, root, contracts.DaemonStatus{
		PID:            2468,
		Version:        "test",
		Port:           7781,
		URL:            "http://127.0.0.1:7781",
		ExecutablePath: "/bin/ccx",
		Running:        true,
	})

	stdout, stderr, code := runCLIWithRunOptions(cli.Options{
		Args:          []string{"run", "--quiet", "--print-only"},
		Build:         cli.BuildInfo{Version: "test"},
		DaemonRoot:    root,
		DaemonProcess: proc,
		Executable:    "/bin/ccx",
	})
	if code != 0 {
		t.Fatalf("run exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "profile=work") {
		t.Fatalf("run output = %q, want selected profile", stdout)
	}
	assertNoRunUsageRows(t, home)
}

func createFakeClaude(t *testing.T, dir string, exitCode int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "claude.bat")
		if err := os.WriteFile(path, []byte("@echo off\r\nexit /b "+strconv.Itoa(exitCode)+"\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // Test fixture must be executable.
			t.Fatal(err)
		}
		return path
	}

	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit "+strconv.Itoa(exitCode)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // Test fixture must be executable.
		t.Fatal(err)
	}
	return path
}

func writeRunUsageJSONL(t *testing.T, cfgDir string) {
	t.Helper()
	projectDir := filepath.Join(cfgDir, "projects", "proj")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","uuid":"run-scan-test","sessionId":"s-1","timestamp":"2026-05-25T12:00:00Z","message":{"model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, "s-1.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertNoRunUsageRows(t *testing.T, home string) {
	t.Helper()
	ctx := context.Background()
	store, err := storage.NewStore(ctx, filepath.Join(home, ".ccx", "state.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	rows, err := store.QueryUsage(ctx, contracts.UsageQuery{
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("usage rows = %+v, want none before ccx run launch", rows)
	}
}

func runCLIWithRunOptions(opts cli.Options) (string, string, int) {
	var stdout, stderr bytes.Buffer
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	code := cli.Run(context.Background(), opts)
	return stdout.String(), stderr.String(), code
}

type runFakeProcess struct {
	mu         sync.Mutex
	alive      map[int]bool
	identities map[int]string
}

func newCLIFakeProcess(_ string) *runFakeProcess {
	return &runFakeProcess{
		alive:      map[int]bool{},
		identities: map[int]string{},
	}
}

func (f *runFakeProcess) Alive(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive[pid]
}

func (f *runFakeProcess) Matches(int, string) bool {
	return true
}

func (f *runFakeProcess) Identity(pid int) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if identity, ok := f.identities[pid]; ok {
		return identity, true
	}
	return runTestProcessIdentity(pid), true
}

func (f *runFakeProcess) StartDetached(context.Context, *daemon.StartProcessSpec) (int, error) {
	return 0, fmt.Errorf("unexpected daemon start")
}

func (f *runFakeProcess) Terminate(pid int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alive[pid] = false
	return nil
}

func (f *runFakeProcess) setAlive(pid int, alive bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alive[pid] = alive
	f.identities[pid] = runTestProcessIdentity(pid)
}

func writeDaemonRuntime(t *testing.T, root string, status contracts.DaemonStatus) {
	t.Helper()
	paths := daemon.RuntimePaths(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if status.Running && status.StartToken == "" {
		status.StartToken = "test-token"
	}
	if status.Running && status.ProcessIdentity == "" && status.PID > 0 {
		status.ProcessIdentity = runTestProcessIdentity(status.PID)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.StatusPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PIDPath, []byte(strconv.Itoa(status.PID)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if status.Running && status.StartToken != "" {
		lockData, err := json.Marshal(map[string]any{
			"token":            status.StartToken,
			"pid":              status.PID,
			"process_identity": status.ProcessIdentity,
			"created_at":       time.Now().UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths.LockPath, append(lockData, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func runTestProcessIdentity(pid int) string {
	return "pid:" + strconv.Itoa(pid)
}

func createCaptureClaude(t *testing.T, dir, capture string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "claude.bat")
		body := fmt.Sprintf(
			"@echo off\r\n"+
				"echo CLAUDE_CONFIG_DIR=%%CLAUDE_CONFIG_DIR%%> %q\r\n"+
				"echo CCX_ACTIVE_PROFILE=%%CCX_ACTIVE_PROFILE%%>> %q\r\n"+
				"setlocal enabledelayedexpansion\r\n"+
				"set ARGS=\r\n"+
				":loop\r\n"+
				"if \"%%~1\"==\"\" goto done\r\n"+
				"if defined ARGS (set ARGS=!ARGS!^|%%~1) else (set ARGS=%%~1)\r\n"+
				"shift\r\n"+
				"goto loop\r\n"+
				":done\r\n"+
				"echo ARGS=!ARGS!>> %q\r\n",
			capture, capture, capture,
		)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // Test fixture must be executable.
			t.Fatal(err)
		}
		return path
	}

	path := filepath.Join(dir, "claude")
	body := fmt.Sprintf(
		"#!/bin/sh\n"+
			"printf 'CLAUDE_CONFIG_DIR=%%s\\n' \"$CLAUDE_CONFIG_DIR\" > %q\n"+
			"printf 'CCX_ACTIVE_PROFILE=%%s\\n' \"$CCX_ACTIVE_PROFILE\" >> %q\n"+
			"old_ifs=$IFS\n"+
			"IFS='|'\n"+
			"printf 'ARGS=%%s\\n' \"$*\" >> %q\n"+
			"IFS=$old_ifs\n",
		capture, capture, capture,
	)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // Test fixture must be executable.
		t.Fatal(err)
	}
	return path
}
