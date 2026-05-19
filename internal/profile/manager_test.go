package profile_test

import (
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/profile"
)

func TestNewManagerCreatesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ccx-home")

	mgr, err := profile.NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr == nil {
		t.Fatal("NewManager returned nil manager")
	}
	if got := mgr.Root(); got != root {
		t.Errorf("Root() = %q, want %q", got, root)
	}
	if got := mgr.Path(); got != filepath.Join(root, "profiles.toml") {
		t.Errorf("Path() = %q, want %q", got, filepath.Join(root, "profiles.toml"))
	}
}

func TestNewManagerRejectsEmptyRoot(t *testing.T) {
	if _, err := profile.NewManager(""); err == nil {
		t.Fatal("NewManager(\"\") should return an error")
	}
}
