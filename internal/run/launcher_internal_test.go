package run

import (
	"os"
	"os/exec"
	"testing"
)

// TestApplyStdioFallsBackToParentFiles pins the fix for the lost-TTY bug: a
// LaunchSpec with nil *os.File stdio fields must leave cmd.Stdin/out/err set to
// the concrete os.Stdin/out/err files (so os/exec forwards their fds and a TTY
// is preserved), not a non-nil interface wrapping a typed-nil *os.File.
func TestApplyStdioFallsBackToParentFiles(t *testing.T) {
	cmd := exec.Command("true")
	applyStdio(cmd, &LaunchSpec{})

	if cmd.Stdin != os.Stdin {
		t.Errorf("Stdin = %#v, want os.Stdin", cmd.Stdin)
	}
	if cmd.Stdout != os.Stdout {
		t.Errorf("Stdout = %#v, want os.Stdout", cmd.Stdout)
	}
	if cmd.Stderr != os.Stderr {
		t.Errorf("Stderr = %#v, want os.Stderr", cmd.Stderr)
	}
}

// TestApplyStdioPrefersSpecFiles verifies explicit spec files override the
// parent fallbacks.
func TestApplyStdioPrefersSpecFiles(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })

	cmd := exec.Command("true")
	applyStdio(cmd, &LaunchSpec{Stdin: r, Stdout: w, Stderr: w})

	if cmd.Stdin != r {
		t.Errorf("Stdin = %#v, want spec.Stdin", cmd.Stdin)
	}
	if cmd.Stdout != w {
		t.Errorf("Stdout = %#v, want spec.Stdout", cmd.Stdout)
	}
	if cmd.Stderr != w {
		t.Errorf("Stderr = %#v, want spec.Stderr", cmd.Stderr)
	}
}
