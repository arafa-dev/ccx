//go:build !windows

package quotamigrate_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/arafa-dev/ccx/internal/quotamigrate"
)

func TestApplyRefusesNonRegularSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "projects")
	dst := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(src, "events.pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}

	err := quotamigrate.Apply([]quotamigrate.Step{{
		Action: quotamigrate.ActionMoveContents,
		From:   src,
		To:     dst,
	}})
	if err == nil {
		t.Fatal("Apply should refuse non-regular source files")
	}
	if _, statErr := os.Stat(filepath.Join(dst, "events.pipe")); !os.IsNotExist(statErr) {
		t.Fatalf("non-regular source should not be copied; stat err=%v", statErr)
	}
}
