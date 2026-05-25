package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/cli"
)

func TestInitZshContainsWrapperFunction(t *testing.T) {
	var stdout bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"init", "zsh"},
		Stdout: &stdout,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout.String(), "ccx()") && !strings.Contains(stdout.String(), "function ccx") {
		t.Errorf("missing wrapper function: %q", stdout.String())
	}
}

func TestInitUnknownShellErrors(t *testing.T) {
	var stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"init", "tcsh"},
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code == 0 {
		t.Errorf("expected non-zero exit for unknown shell")
	}
	if !strings.Contains(stderr.String(), "unknown shell") {
		t.Errorf("expected 'unknown shell' in stderr: %q", stderr.String())
	}
}

func TestInitWithClaudeWrapperPosix(t *testing.T) {
	s := runCLI(t, "init", "zsh", "--with-claude-wrapper")
	if !strings.Contains(s, "claude()") {
		t.Errorf("expected claude() wrapper; got:\n%s", s)
	}
	if !strings.Contains(s, "ccx run --") {
		t.Errorf("expected `ccx run --` in wrapper; got:\n%s", s)
	}
}

func TestInitWithoutFlagOmitsWrapper(t *testing.T) {
	s := runCLI(t, "init", "zsh")
	if strings.Contains(s, "claude()") {
		t.Errorf("default init should not include claude wrapper:\n%s", s)
	}
}
