package profile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestLoadRegistryMissingFile(t *testing.T) {
	dir := t.TempDir()
	r, err := loadRegistry(filepath.Join(dir, "profiles.toml"))
	if err != nil {
		t.Fatalf("loadRegistry on missing file: %v", err)
	}
	if len(r.Profiles) != 0 {
		t.Errorf("missing file should yield empty registry, got %d profiles", len(r.Profiles))
	}
}

func TestAtomicWriteCreatesParentDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ccx-home")
	path := filepath.Join(root, "profiles.toml")

	r := registry{Profiles: []contracts.Profile{{
		Name:       "work",
		ConfigDir:  "/abs/path/work",
		CreatedAt:  time.Now().UTC(),
		LastUsedAt: time.Now().UTC(),
	}}}

	if err := atomicWriteRegistry(path, r); err != nil {
		t.Fatalf("atomicWriteRegistry: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", info.Mode().Perm())
	}

	// .tmp must not linger after a successful write.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .tmp to be absent, got err=%v", err)
	}
}

func TestLoadIgnoresStaleTmpFile(t *testing.T) {
	// Simulate a process crash mid-write: profiles.toml exists with valid
	// content; profiles.toml.tmp exists with junk. loadRegistry must read
	// the real file successfully and not be confused by the leftover .tmp.
	root := t.TempDir()
	path := filepath.Join(root, "profiles.toml")

	good := registry{Profiles: []contracts.Profile{{
		Name:       "personal",
		ConfigDir:  "/abs/path/personal",
		CreatedAt:  time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		LastUsedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}}}
	if err := atomicWriteRegistry(path, good); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	if err := os.WriteFile(path+".tmp", []byte("this is not valid toml ===="), 0o600); err != nil {
		t.Fatalf("seed tmp: %v", err)
	}

	r, err := loadRegistry(path)
	if err != nil {
		t.Fatalf("loadRegistry with stale tmp: %v", err)
	}
	if len(r.Profiles) != 1 || r.Profiles[0].Name != "personal" {
		t.Errorf("expected 1 profile named personal, got %+v", r.Profiles)
	}
}

func TestAtomicWriteOverwrites(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "profiles.toml")

	first := registry{Profiles: []contracts.Profile{{Name: "a", ConfigDir: "/x/a"}}}
	if err := atomicWriteRegistry(path, first); err != nil {
		t.Fatalf("first write: %v", err)
	}
	second := registry{Profiles: []contracts.Profile{{Name: "b", ConfigDir: "/x/b"}}}
	if err := atomicWriteRegistry(path, second); err != nil {
		t.Fatalf("second write: %v", err)
	}

	r, err := loadRegistry(path)
	if err != nil {
		t.Fatalf("loadRegistry: %v", err)
	}
	if len(r.Profiles) != 1 || r.Profiles[0].Name != "b" {
		t.Errorf("expected single profile b after overwrite, got %+v", r.Profiles)
	}
}
