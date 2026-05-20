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

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code != 0 {
		t.Fatalf("exit %d: stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	return stdout.String()
}
