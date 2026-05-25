package quotamigrate_test

import (
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/quotamigrate"
)

func TestSharedProjectsPath(t *testing.T) {
	got := quotamigrate.SharedProjectsPath("/home/x/.ccx")
	want := filepath.Join("/home/x/.ccx", "shared-projects")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProfileProjectsPath(t *testing.T) {
	got := quotamigrate.ProfileProjectsPath("/home/x/.claude-profiles/work")
	want := filepath.Join("/home/x/.claude-profiles/work", "projects")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
