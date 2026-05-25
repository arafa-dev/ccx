package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
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
