package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/cli"
)

func TestExecuteHelpShowsAllCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"--help"},
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code != 0 {
		t.Fatalf("--help exit=%d stderr=%q", code, stderr.String())
	}
	want := []string{"profile", "use", "init", "usage", "dashboard", "doctor", "version"}
	got := stdout.String()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("--help missing command %q", w)
		}
	}
}

func TestExecuteVersion(t *testing.T) {
	var stdout bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"version"},
		Stdout: &stdout,
		Build:  cli.BuildInfo{Version: "0.0.0-test"},
	})
	if code != 0 || !strings.Contains(stdout.String(), "0.0.0-test") {
		t.Errorf("version: code=%d out=%q", code, stdout.String())
	}
}
