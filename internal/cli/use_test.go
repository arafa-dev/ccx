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

func TestUseEmitsExportForPOSIX(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SHELL", "/bin/zsh")
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"use", "work"},
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "export CLAUDE_CONFIG_DIR=") {
		t.Errorf("missing export CLAUDE_CONFIG_DIR: %q", got)
	}
	if !strings.Contains(got, "export CCX_ACTIVE_PROFILE=") {
		t.Errorf("missing export CCX_ACTIVE_PROFILE: %q", got)
	}
	if !strings.Contains(got, cfgDir) {
		t.Errorf("missing config dir path: %q", got)
	}
}
