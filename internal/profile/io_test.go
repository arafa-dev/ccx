package profile

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestAtomicWriteSyncsToDisk(t *testing.T) {
	source, err := os.ReadFile("io.go")
	if err != nil {
		t.Fatalf("read io.go: %v", err)
	}
	requireOrderedSource(
		t, string(source),
		"os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)",
		".Write(data)",
		".Sync()",
		".Close()",
		"os.Rename(tmp, path)",
		"syncParentDir(path)",
	)

	root := filepath.Join(t.TempDir(), "ccx-home")
	path := filepath.Join(root, "profiles.toml")
	r := registry{Profiles: []contracts.Profile{{Name: "work", ConfigDir: "/x/work"}}}
	if err := atomicWriteRegistry(path, r); err != nil {
		t.Fatalf("atomicWriteRegistry: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("successful atomic write left tmp file, err=%v", err)
	}

	mgr, err := NewManager(filepath.Join(t.TempDir(), "manager-root"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx := context.Background()
	if err := mgr.Add(ctx, contracts.Profile{Name: "first", ConfigDir: filepath.Join(t.TempDir(), "first")}); err != nil {
		t.Fatalf("Add first: %v", err)
	}
	if _, err := os.Stat(mgr.Path() + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("first Add left tmp file, err=%v", err)
	}
	if err := mgr.Add(ctx, contracts.Profile{Name: "second", ConfigDir: filepath.Join(t.TempDir(), "second")}); err != nil {
		t.Fatalf("Add second: %v", err)
	}
	if _, err := os.Stat(mgr.Path() + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("second Add left tmp file, err=%v", err)
	}
}

func requireOrderedSource(t *testing.T, source string, tokens ...string) {
	t.Helper()
	offset := 0
	for _, token := range tokens {
		idx := strings.Index(source[offset:], token)
		if idx < 0 {
			t.Fatalf("source does not contain %q after byte offset %d", token, offset)
		}
		offset += idx + len(token)
	}
}
