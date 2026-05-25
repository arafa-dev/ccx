package run_test

import (
	"context"
	"os/exec"
	"runtime"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/run"
)

func TestLocateClaudeUsesPATH(t *testing.T) {
	got, err := run.LocateClaude(run.Options{LookPath: func(name string) (string, error) {
		if name != "claude" {
			t.Fatalf("LookPath called with %q, want claude", name)
		}
		return "/usr/local/bin/claude", nil
	}})
	if err != nil {
		t.Fatalf("LocateClaude: %v", err)
	}
	if got != "/usr/local/bin/claude" {
		t.Errorf("got %q, want /usr/local/bin/claude", got)
	}
}

func TestLocateClaudeMissingReturnsErrNotFound(t *testing.T) {
	_, err := run.LocateClaude(run.Options{LookPath: func(string) (string, error) {
		return "", exec.ErrNotFound
	}})
	if err == nil {
		t.Fatal("expected error when claude is missing")
	}
}

func TestLocateClaudeRespectsBinaryOverride(t *testing.T) {
	got, err := run.LocateClaude(run.Options{
		BinaryPath: "/opt/custom/claude",
		LookPath: func(string) (string, error) {
			t.Fatal("override should bypass LookPath")
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("LocateClaude: %v", err)
	}
	if got != "/opt/custom/claude" {
		t.Errorf("got %q, want override", got)
	}
}

func TestBuildEnvSetsExpectedVars(t *testing.T) {
	profile := contracts.Profile{Name: "work", ConfigDir: "/p/work"}
	env := run.BuildEnv(profile, []string{"PATH=/usr/bin", "HOME=/Users/x"})
	hasConfig := false
	hasActive := false
	for _, e := range env {
		if e == "CLAUDE_CONFIG_DIR=/p/work" {
			hasConfig = true
		}
		if e == "CCX_ACTIVE_PROFILE=work" {
			hasActive = true
		}
	}
	if !hasConfig {
		t.Error("expected CLAUDE_CONFIG_DIR in env")
	}
	if !hasActive {
		t.Error("expected CCX_ACTIVE_PROFILE in env")
	}
}

func TestBuildEnvOverwritesExistingValues(t *testing.T) {
	profile := contracts.Profile{Name: "work", ConfigDir: "/new"}
	env := run.BuildEnv(profile, []string{"CLAUDE_CONFIG_DIR=/old", "PATH=/x"})
	for _, e := range env {
		if e == "CLAUDE_CONFIG_DIR=/old" {
			t.Errorf("expected /old to be overwritten; got %q", e)
		}
	}
}

func TestLaunchReturnsExitCodeOfChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}
	exit, err := run.Launch(context.Background(), run.LaunchSpec{
		BinaryPath: "/bin/sh",
		Args:       []string{"-c", "exit 7"},
		Env:        []string{"PATH=/usr/bin:/bin"},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if exit != 7 {
		t.Errorf("exit code: got %d, want 7", exit)
	}
}
