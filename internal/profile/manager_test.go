package profile_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
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

func newTestManager(t *testing.T) *profile.Manager {
	t.Helper()
	root := filepath.Join(t.TempDir(), "ccx-home")
	mgr, err := profile.NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

func makeAbsDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return dir
}

func TestAddPersistsProfile(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg := makeAbsDir(t, "work")

	p := contracts.Profile{
		Name:       "work",
		ConfigDir:  cfg,
		Label:      "Work",
		Color:      "#3B82F6",
		CreatedAt:  time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		LastUsedAt: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
	}
	if err := mgr.Add(ctx, p); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// File should exist with mode 0600.
	info, err := os.Stat(mgr.Path())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestAddRejectsRelativeConfigDir(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: "relative/x"})
	if !errors.Is(err, contracts.ErrInvalidConfigDir) {
		t.Fatalf("expected ErrInvalidConfigDir, got %v", err)
	}
}

func TestAddRejectsEmptyName(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg := makeAbsDir(t, "x")
	err := mgr.Add(ctx, contracts.Profile{Name: "", ConfigDir: cfg})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestAddRejectsDuplicateName(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg1 := makeAbsDir(t, "work")
	cfg2 := makeAbsDir(t, "work2")

	if err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: cfg1}); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: cfg2})
	if !errors.Is(err, contracts.ErrProfileAlreadyExists) {
		t.Fatalf("expected ErrProfileAlreadyExists, got %v", err)
	}
}

func TestAddCreatesMissingConfigDir(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	// ConfigDir does not exist yet - Add should create it.
	cfg := filepath.Join(t.TempDir(), "to-be-created", "work")

	if err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: cfg}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(cfg); err != nil {
		t.Errorf("expected ConfigDir to be created, stat err: %v", err)
	}
}

func TestAddRejectsDuplicateConfigDir(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg := makeAbsDir(t, "shared")

	if err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: cfg}); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	err := mgr.Add(ctx, contracts.Profile{Name: "personal", ConfigDir: cfg})
	if !errors.Is(err, contracts.ErrConfigDirConflict) {
		t.Fatalf("expected ErrConfigDirConflict, got %v", err)
	}
}

func TestGetReturnsProfile(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg := makeAbsDir(t, "work")

	in := contracts.Profile{Name: "work", ConfigDir: cfg}
	if err := mgr.Add(ctx, in); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := mgr.Get(ctx, "work")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "work" || got.ConfigDir != cfg {
		t.Errorf("got = %+v, want name=work config=%q", got, cfg)
	}
}

func TestGetMissingProfileReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	_, err := mgr.Get(ctx, "ghost")
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestGetEmptyNameIsError(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	if _, err := mgr.Get(ctx, ""); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestListReturnsEmptyOnFreshManager(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	got, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %d", len(got))
	}
}

func TestListReturnsAllProfilesSortedByName(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	for _, name := range []string{"work", "alpha", "side"} {
		cfg := makeAbsDir(t, name)
		if err := mgr.Add(ctx, contracts.Profile{Name: name, ConfigDir: cfg}); err != nil {
			t.Fatalf("Add(%s): %v", name, err)
		}
	}

	got, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "side", "work"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("[%d] = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestRemoveDeletesProfile(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg := makeAbsDir(t, "work")
	if err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: cfg}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := mgr.Remove(ctx, "work"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err := mgr.Get(ctx, "work")
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Fatalf("after Remove, Get should return ErrProfileNotFound, got %v", err)
	}
}

func TestRemoveMissingProfileReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	err := mgr.Remove(ctx, "ghost")
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestRemovePreservesOtherProfiles(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfgA := makeAbsDir(t, "a")
	cfgB := makeAbsDir(t, "b")
	if err := mgr.Add(ctx, contracts.Profile{Name: "a", ConfigDir: cfgA}); err != nil {
		t.Fatalf("Add a: %v", err)
	}
	if err := mgr.Add(ctx, contracts.Profile{Name: "b", ConfigDir: cfgB}); err != nil {
		t.Fatalf("Add b: %v", err)
	}

	if err := mgr.Remove(ctx, "a"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "b" {
		t.Errorf("expected [b], got %+v", got)
	}
}
