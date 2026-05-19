# ccx Phase 1 A1 — `internal/profile/` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the TOML-backed profile registry at `~/.ccx/profiles.toml` with full CRUD, validation, atomic writes, and active-profile detection — backed by tests, with zero cross-imports beyond `internal/contracts`.

**Architecture:** A single `ProfileManager` type owns the file at `<root>/profiles.toml` where `<root>` is `~/.ccx` by default but injectable for tests. All mutating methods rewrite the whole file atomically (`profiles.toml.tmp` → `os.Rename`). All methods take `context.Context` and return errors wrapped with `%w` around the sentinels in `internal/contracts`. Active profile resolution reads `CCX_ACTIVE_PROFILE` and `CLAUDE_CONFIG_DIR` from the environment via an injectable `os.LookupEnv`-shaped function so tests can drive it through `t.Setenv`.

**Tech Stack:** Go 1.22+, `github.com/pelletier/go-toml/v2`, stdlib only. No other dependencies.

**Spec reference:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — Section 6 (Profile switching mechanism) and Section 11.2 (worktree A1).

**Worktree:** `feat/profile` — created off `main` after Phase 0 is merged.

**Exit criteria:**
- `go build ./internal/profile/...` succeeds
- `go test -race -count=1 ./internal/profile/...` passes (including `t.Parallel` and `t.Setenv` cases)
- `golangci-lint run ./internal/profile/...` reports zero issues
- `gofumpt -l ./internal/profile/` produces no output
- `ProfileManager` exposes: `Add`, `Get`, `List`, `Remove`, `MarkUsed`, `Active` — all `context.Context`-aware
- All sentinel errors from `internal/contracts` used in this package are detectable via `errors.Is`
- PR opened against `main` with CI green

---

## Pre-flight

Confirm Phase 0 is merged on `main` and the contracts file contains every symbol referenced by this plan.

```bash
cd /Users/arafa/Developer/ccx
git fetch --all
git checkout main
git pull --ff-only origin main
git log --oneline -1                                     # → latest commit is on main
test -f internal/contracts/types.go                      # → exists (Profile)
test -f internal/contracts/errors.go                     # → exists (sentinels)
grep -q "ErrProfileNotFound"        internal/contracts/errors.go
grep -q "ErrProfileAlreadyExists"   internal/contracts/errors.go
grep -q "ErrInvalidConfigDir"       internal/contracts/errors.go
grep -q "ErrConfigDirConflict"      internal/contracts/errors.go
grep -q "ErrNoActiveProfile"        internal/contracts/errors.go
grep -q "type Profile struct"       internal/contracts/types.go
```

If any check fails, stop and request that Phase 0 be completed/merged first.

Create the worktree and switch to it. All subsequent commands run inside the worktree.

```bash
git worktree add ../ccx-profile -b feat/profile main
cd ../ccx-profile
go build ./...                                           # → success, baseline green
go test ./...                                            # → success, baseline green
```

**Conventions for this plan:**
- All Go code uses tabs for indentation (gofumpt enforced).
- All commit messages follow `type(scope): subject`. Scope for this worktree is `profile`.
- Every task ends with exactly one commit.
- Run `go test -race -count=1 ./internal/profile/...` before every commit.
- The package may import `context`, `errors`, `fmt`, `os`, `path/filepath`, `sort`, `strings`, `sync`, `time`, plus `github.com/pelletier/go-toml/v2` and `github.com/arafa-dev/ccx/internal/contracts`. Nothing else.

---

## Task 1: Add `pelletier/go-toml/v2` dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add the dependency**

Run:
```bash
go get github.com/pelletier/go-toml/v2@latest
go mod tidy
```

- [ ] **Step 2: Verify the module graph**

Run:
```bash
go list -m github.com/pelletier/go-toml/v2
```
Expected: prints a single line like `github.com/pelletier/go-toml/v2 v2.x.y`.

Run:
```bash
go build ./...
```
Expected: success, no output.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build(profile): add pelletier/go-toml/v2 dependency"
```

---

## Task 2: Skeleton `ProfileManager` with constructor (TDD)

**Files:**
- Modify: `internal/profile/doc.go` (replace stub from Phase 0)
- Create: `internal/profile/manager.go`
- Create: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/profile/manager_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `profile.NewManager` undefined.

- [ ] **Step 3: Write minimal implementation**

Replace `internal/profile/doc.go` with:

```go
// Package profile manages the ccx profile registry (~/.ccx/profiles.toml).
// It implements profile CRUD, validation, atomic file writes, and active-
// profile detection. All ProfileManager methods take context.Context. The
// registry file is rewritten atomically on every mutation (write to
// profiles.toml.tmp, then os.Rename).
package profile
```

Create `internal/profile/manager.go`:

```go
package profile

import (
	"errors"
	"path/filepath"
	"sync"
)

// fileName is the canonical name of the registry file inside the ccx root.
const fileName = "profiles.toml"

// tmpSuffix is the suffix used by atomic writes. The temp file is renamed
// over fileName on successful flush.
const tmpSuffix = ".tmp"

// Manager owns the profile registry at <root>/profiles.toml. All mutating
// methods rewrite the whole file atomically. The zero Manager is not usable;
// always construct via NewManager.
type Manager struct {
	root string
	mu   sync.Mutex
}

// NewManager returns a Manager rooted at the given directory (typically
// ~/.ccx). The directory does not need to exist yet; it is created lazily by
// the first mutating call.
func NewManager(root string) (*Manager, error) {
	if root == "" {
		return nil, errors.New("profile: root path is empty")
	}
	return &Manager{root: root}, nil
}

// Root returns the registry root directory.
func (m *Manager) Root() string {
	return m.root
}

// Path returns the absolute path to the registry file.
func (m *Manager) Path() string {
	return filepath.Join(m.root, fileName)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/doc.go internal/profile/manager.go internal/profile/manager_test.go
git commit -m "feat(profile): add Manager skeleton with NewManager constructor"
```

---

## Task 3: TOML roundtrip via internal registry struct (TDD)

**Files:**
- Create: `internal/profile/registry.go`
- Create: `internal/profile/registry_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/profile/registry_test.go`:

```go
package profile

import (
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestRegistryEncodeDecodeRoundtrip(t *testing.T) {
	in := registry{
		Profiles: []contracts.Profile{
			{
				Name:       "work",
				ConfigDir:  "/home/u/.claude-profiles/work",
				Label:      "Work",
				Color:      "#3B82F6",
				CreatedAt:  time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
				LastUsedAt: time.Date(2026, 5, 19, 15, 30, 0, 0, time.UTC),
			},
			{
				Name:       "personal",
				ConfigDir:  "/home/u/.claude-profiles/personal",
				CreatedAt:  time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
				LastUsedAt: time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
			},
		},
	}

	data, err := encodeRegistry(in)
	if err != nil {
		t.Fatalf("encodeRegistry: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("encodeRegistry returned empty bytes")
	}

	out, err := decodeRegistry(data)
	if err != nil {
		t.Fatalf("decodeRegistry: %v", err)
	}

	if len(out.Profiles) != len(in.Profiles) {
		t.Fatalf("len mismatch: got %d, want %d", len(out.Profiles), len(in.Profiles))
	}
	for i := range in.Profiles {
		if !out.Profiles[i].CreatedAt.Equal(in.Profiles[i].CreatedAt) {
			t.Errorf("profile[%d] CreatedAt: got %v, want %v", i, out.Profiles[i].CreatedAt, in.Profiles[i].CreatedAt)
		}
		if !out.Profiles[i].LastUsedAt.Equal(in.Profiles[i].LastUsedAt) {
			t.Errorf("profile[%d] LastUsedAt: got %v, want %v", i, out.Profiles[i].LastUsedAt, in.Profiles[i].LastUsedAt)
		}
		// Compare the time-independent fields.
		a, b := in.Profiles[i], out.Profiles[i]
		a.CreatedAt, a.LastUsedAt = time.Time{}, time.Time{}
		b.CreatedAt, b.LastUsedAt = time.Time{}, time.Time{}
		if a != b {
			t.Errorf("profile[%d] mismatch:\n got  %+v\n want %+v", i, b, a)
		}
	}
}

func TestDecodeRegistryEmptyBytes(t *testing.T) {
	out, err := decodeRegistry(nil)
	if err != nil {
		t.Fatalf("decodeRegistry(nil): %v", err)
	}
	if len(out.Profiles) != 0 {
		t.Errorf("empty input should yield 0 profiles, got %d", len(out.Profiles))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `registry`, `encodeRegistry`, `decodeRegistry` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/profile/registry.go`:

```go
package profile

import (
	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/pelletier/go-toml/v2"
)

// registry is the on-disk shape of profiles.toml. The TOML form is a single
// table-array under the key "profile":
//
//	[[profile]]
//	name        = "work"
//	config_dir  = "/home/u/.claude-profiles/work"
//	...
type registry struct {
	Profiles []contracts.Profile `toml:"profile"`
}

// encodeRegistry serializes r to TOML bytes.
func encodeRegistry(r registry) ([]byte, error) {
	return toml.Marshal(r)
}

// decodeRegistry parses TOML bytes into a registry. An empty or nil input
// produces an empty registry without error so that a freshly created file
// (or no file at all) round-trips cleanly.
func decodeRegistry(data []byte) (registry, error) {
	var r registry
	if len(data) == 0 {
		return r, nil
	}
	if err := toml.Unmarshal(data, &r); err != nil {
		return registry{}, err
	}
	return r, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/registry.go internal/profile/registry_test.go
git commit -m "feat(profile): add TOML registry encode/decode"
```

---

## Task 4: Atomic write + load helpers (TDD)

**Files:**
- Create: `internal/profile/io.go`
- Create: `internal/profile/io_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/profile/io_test.go`:

```go
package profile

import (
	"os"
	"path/filepath"
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
	if info.Mode().Perm() != 0o600 {
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `loadRegistry`, `atomicWriteRegistry` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/profile/io.go`:

```go
package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// loadRegistry reads and parses the registry file at path. A missing file is
// treated as an empty registry (not an error) so that the first run of ccx
// works without `ccx profile add` having been called yet.
func loadRegistry(path string) (registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return registry{}, nil
		}
		return registry{}, fmt.Errorf("reading registry %q: %w", path, err)
	}
	r, err := decodeRegistry(data)
	if err != nil {
		return registry{}, fmt.Errorf("parsing registry %q: %w", path, err)
	}
	return r, nil
}

// atomicWriteRegistry serializes r to TOML and writes it to path via a
// rename-from-tmp dance. The parent directory is created with 0700 if it
// does not exist. The final file mode is 0600.
//
// On error the .tmp file is removed if possible; the original path is left
// untouched.
func atomicWriteRegistry(path string, r registry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating registry dir: %w", err)
	}

	data, err := encodeRegistry(r)
	if err != nil {
		return fmt.Errorf("encoding registry: %w", err)
	}

	tmp := path + tmpSuffix
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing tmp registry: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming tmp registry: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/io.go internal/profile/io_test.go
git commit -m "feat(profile): add atomic registry load and write helpers"
```

---

## Task 5: Validation rules (TDD)

**Files:**
- Create: `internal/profile/validate.go`
- Create: `internal/profile/validate_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/profile/validate_test.go`:

```go
package profile_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
)

func TestValidateProfileRejectsEmptyName(t *testing.T) {
	p := contracts.Profile{
		Name:      "",
		ConfigDir: "/abs/path",
	}
	err := profile.ValidateProfile(p)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateProfileRejectsRelativeConfigDir(t *testing.T) {
	p := contracts.Profile{Name: "work", ConfigDir: "relative/path"}
	err := profile.ValidateProfile(p)
	if !errors.Is(err, contracts.ErrInvalidConfigDir) {
		t.Fatalf("expected ErrInvalidConfigDir, got %v", err)
	}
}

func TestValidateProfileRejectsEmptyConfigDir(t *testing.T) {
	p := contracts.Profile{Name: "work", ConfigDir: ""}
	err := profile.ValidateProfile(p)
	if !errors.Is(err, contracts.ErrInvalidConfigDir) {
		t.Fatalf("expected ErrInvalidConfigDir, got %v", err)
	}
}

func TestValidateProfileAcceptsAbsolutePath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "claude")
	p := contracts.Profile{
		Name:       "work",
		ConfigDir:  abs,
		CreatedAt:  time.Now().UTC(),
		LastUsedAt: time.Now().UTC(),
	}
	if err := profile.ValidateProfile(p); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateProfileRejectsNamesWithSlashOrSpace(t *testing.T) {
	for _, name := range []string{"foo/bar", "foo bar", "foo\tbar", "."} {
		p := contracts.Profile{Name: name, ConfigDir: "/abs/x"}
		if err := profile.ValidateProfile(p); err == nil {
			t.Errorf("expected error for name %q, got nil", name)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `profile.ValidateProfile` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/profile/validate.go`:

```go
package profile

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// ValidateProfile checks that p is well-formed enough to be stored in the
// registry. It does NOT touch the filesystem; existence checks are done by
// the Manager so that pure validation is cheap and testable.
//
// Rules:
//   - Name is non-empty
//   - Name contains no path separators, whitespace, or "." / ".."
//   - ConfigDir is non-empty and absolute (filepath.IsAbs)
func ValidateProfile(p contracts.Profile) error {
	if err := validateName(p.Name); err != nil {
		return err
	}
	if p.ConfigDir == "" {
		return fmt.Errorf("config_dir is empty: %w", contracts.ErrInvalidConfigDir)
	}
	if !filepath.IsAbs(p.ConfigDir) {
		return fmt.Errorf("config_dir %q is not absolute: %w", p.ConfigDir, contracts.ErrInvalidConfigDir)
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("profile: name is empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("profile: name %q is reserved", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("profile: name %q contains a path separator", name)
	}
	for _, r := range name {
		switch r {
		case ' ', '\t', '\n', '\r':
			return fmt.Errorf("profile: name %q contains whitespace", name)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/validate.go internal/profile/validate_test.go
git commit -m "feat(profile): add ValidateProfile with name and path rules"
```

---

## Task 6: `Add` method with absolute-path and creatable-dir checks (TDD)

**Files:**
- Modify: `internal/profile/manager.go`
- Modify: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/profile/manager_test.go`:

```go
import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)
```

(Adjust the existing import block so it reads:)

```go
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
```

Then append the new tests:

```go
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
	// ConfigDir does not exist yet — Add should create it.
	cfg := filepath.Join(t.TempDir(), "to-be-created", "work")

	if err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: cfg}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := os.Stat(cfg); err != nil {
		t.Errorf("expected ConfigDir to be created, stat err: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `mgr.Add` undefined.

- [ ] **Step 3: Implement `Add` on Manager**

Append to `internal/profile/manager.go`:

```go
import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)
```

(Adjust the existing import block so the final imports are:)

```go
import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)
```

Then append the `Add` method (and a small helper):

```go
// Add registers a new profile. Behavior:
//   - Validates the profile shape (name, absolute ConfigDir).
//   - Rejects with contracts.ErrProfileAlreadyExists if another profile has
//     the same name.
//   - Ensures ConfigDir exists (creating it with mode 0700 if missing).
//   - Sets CreatedAt/LastUsedAt to time.Now().UTC() when the caller leaves
//     them zero, so callers can pass a bare Profile{Name, ConfigDir}.
//   - Writes the full registry atomically.
func (m *Manager) Add(ctx context.Context, p contracts.Profile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateProfile(p); err != nil {
		return err
	}

	if err := ensureConfigDir(p.ConfigDir); err != nil {
		return fmt.Errorf("preparing config dir %q: %w", p.ConfigDir, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reg, err := loadRegistry(m.Path())
	if err != nil {
		return err
	}

	for _, existing := range reg.Profiles {
		if existing.Name == p.Name {
			return fmt.Errorf("profile %q: %w", p.Name, contracts.ErrProfileAlreadyExists)
		}
	}

	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.LastUsedAt.IsZero() {
		p.LastUsedAt = now
	}

	reg.Profiles = append(reg.Profiles, p)
	if err := atomicWriteRegistry(m.Path(), reg); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}
	return nil
}

// ensureConfigDir guarantees that path exists and is a directory. If path
// does not exist it is created with mode 0700. If path exists but is not a
// directory the call returns contracts.ErrInvalidConfigDir.
func ensureConfigDir(path string) error {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("path %q is not a directory: %w", path, contracts.ErrInvalidConfigDir)
		}
		return nil
	case errors.Is(err, os.ErrNotExist):
		if mkErr := os.MkdirAll(path, 0o700); mkErr != nil {
			return fmt.Errorf("creating %q: %w", path, contracts.ErrInvalidConfigDir)
		}
		return nil
	default:
		return fmt.Errorf("stat %q: %w", path, err)
	}
}
```

Reorder/clean the file so all imports are at the top and only declared once. The final `manager.go` should now contain: imports, constants (`fileName`, `tmpSuffix`), the `Manager` type, `NewManager`, `Root`, `Path`, `Add`, and `ensureConfigDir`. Remove `filepath` from the import list only if no other code uses it — it is still used by `Path()`, so keep it.

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test -race -count=1 ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/manager.go internal/profile/manager_test.go
git commit -m "feat(profile): implement Manager.Add with validation and dir creation"
```

---

## Task 7: Reject duplicate ConfigDir across profiles (TDD)

**Files:**
- Modify: `internal/profile/manager.go`
- Modify: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/profile/manager_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/profile/... -run TestAddRejectsDuplicateConfigDir
```
Expected: FAIL — duplicate ConfigDir is currently accepted.

- [ ] **Step 3: Add the conflict check in `Add`**

In `internal/profile/manager.go`, inside the `Add` method, locate the loop that checks for duplicate names:

```go
	for _, existing := range reg.Profiles {
		if existing.Name == p.Name {
			return fmt.Errorf("profile %q: %w", p.Name, contracts.ErrProfileAlreadyExists)
		}
	}
```

Replace it with:

```go
	for _, existing := range reg.Profiles {
		if existing.Name == p.Name {
			return fmt.Errorf("profile %q: %w", p.Name, contracts.ErrProfileAlreadyExists)
		}
		if existing.ConfigDir == p.ConfigDir {
			return fmt.Errorf("config_dir %q already used by profile %q: %w", p.ConfigDir, existing.Name, contracts.ErrConfigDirConflict)
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test -race -count=1 ./internal/profile/...
```
Expected: PASS for all profile tests.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/manager.go internal/profile/manager_test.go
git commit -m "feat(profile): reject duplicate config_dir on Add"
```

---

## Task 8: `Get` returns wrapped `ErrProfileNotFound` (TDD)

**Files:**
- Modify: `internal/profile/manager.go`
- Modify: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/profile/manager_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `mgr.Get` undefined.

- [ ] **Step 3: Implement `Get`**

Append to `internal/profile/manager.go`:

```go
// Get returns the profile with the given name. If no such profile exists,
// the returned error wraps contracts.ErrProfileNotFound.
func (m *Manager) Get(ctx context.Context, name string) (contracts.Profile, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Profile{}, err
	}
	if name == "" {
		return contracts.Profile{}, fmt.Errorf("profile: name is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reg, err := loadRegistry(m.Path())
	if err != nil {
		return contracts.Profile{}, err
	}
	for _, p := range reg.Profiles {
		if p.Name == name {
			return p, nil
		}
	}
	return contracts.Profile{}, fmt.Errorf("profile %q: %w", name, contracts.ErrProfileNotFound)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test -race -count=1 ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/manager.go internal/profile/manager_test.go
git commit -m "feat(profile): implement Manager.Get with ErrProfileNotFound"
```

---

## Task 9: `List` returns sorted profiles (TDD)

**Files:**
- Modify: `internal/profile/manager.go`
- Modify: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/profile/manager_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `mgr.List` undefined.

- [ ] **Step 3: Implement `List`**

Add `"sort"` to the import list in `internal/profile/manager.go`, then append:

```go
// List returns all profiles, sorted by Name. The returned slice is a fresh
// copy; mutating it does not affect the on-disk registry.
func (m *Manager) List(ctx context.Context) ([]contracts.Profile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reg, err := loadRegistry(m.Path())
	if err != nil {
		return nil, err
	}
	out := make([]contracts.Profile, len(reg.Profiles))
	copy(out, reg.Profiles)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test -race -count=1 ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/manager.go internal/profile/manager_test.go
git commit -m "feat(profile): implement Manager.List sorted by name"
```

---

## Task 10: `Remove` deletes by name (TDD)

**Files:**
- Modify: `internal/profile/manager.go`
- Modify: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/profile/manager_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `mgr.Remove` undefined.

- [ ] **Step 3: Implement `Remove`**

Append to `internal/profile/manager.go`:

```go
// Remove deletes the profile with the given name. If no such profile exists,
// the returned error wraps contracts.ErrProfileNotFound. The file is rewritten
// atomically only if the registry actually changed.
func (m *Manager) Remove(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("profile: name is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reg, err := loadRegistry(m.Path())
	if err != nil {
		return err
	}

	idx := -1
	for i, p := range reg.Profiles {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("profile %q: %w", name, contracts.ErrProfileNotFound)
	}

	reg.Profiles = append(reg.Profiles[:idx], reg.Profiles[idx+1:]...)
	if err := atomicWriteRegistry(m.Path(), reg); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test -race -count=1 ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/manager.go internal/profile/manager_test.go
git commit -m "feat(profile): implement Manager.Remove"
```

---

## Task 11: `MarkUsed` updates LastUsedAt (TDD)

**Files:**
- Modify: `internal/profile/manager.go`
- Modify: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/profile/manager_test.go`:

```go
func TestMarkUsedUpdatesTimestamp(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg := makeAbsDir(t, "work")

	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := contracts.Profile{
		Name:       "work",
		ConfigDir:  cfg,
		CreatedAt:  old,
		LastUsedAt: old,
	}
	if err := mgr.Add(ctx, in); err != nil {
		t.Fatalf("Add: %v", err)
	}

	before := time.Now().UTC().Add(-1 * time.Second)
	if err := mgr.MarkUsed(ctx, "work"); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	after := time.Now().UTC().Add(1 * time.Second)

	got, err := mgr.Get(ctx, "work")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastUsedAt.Before(before) || got.LastUsedAt.After(after) {
		t.Errorf("LastUsedAt %v not in [%v, %v]", got.LastUsedAt, before, after)
	}
	if !got.CreatedAt.Equal(old) {
		t.Errorf("CreatedAt should be untouched, got %v want %v", got.CreatedAt, old)
	}
}

func TestMarkUsedMissingProfileReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	err := mgr.MarkUsed(ctx, "ghost")
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `mgr.MarkUsed` undefined.

- [ ] **Step 3: Implement `MarkUsed`**

Append to `internal/profile/manager.go`:

```go
// MarkUsed updates the LastUsedAt field of the named profile to time.Now()
// in UTC. It is intended to be called by the cli layer after `ccx use`
// successfully emits an activation script. If no such profile exists, the
// returned error wraps contracts.ErrProfileNotFound.
func (m *Manager) MarkUsed(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("profile: name is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reg, err := loadRegistry(m.Path())
	if err != nil {
		return err
	}

	idx := -1
	for i, p := range reg.Profiles {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("profile %q: %w", name, contracts.ErrProfileNotFound)
	}

	reg.Profiles[idx].LastUsedAt = time.Now().UTC()
	if err := atomicWriteRegistry(m.Path(), reg); err != nil {
		return fmt.Errorf("saving registry: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test -race -count=1 ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/manager.go internal/profile/manager_test.go
git commit -m "feat(profile): implement Manager.MarkUsed"
```

---

## Task 12: `Active` detection via env (TDD)

**Files:**
- Modify: `internal/profile/manager.go`
- Modify: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/profile/manager_test.go`:

```go
func TestActiveNoEnvReturnsNoActiveProfile(t *testing.T) {
	t.Setenv("CCX_ACTIVE_PROFILE", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	ctx := context.Background()
	mgr := newTestManager(t)

	_, ok, err := mgr.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false when no env vars set")
	}
}

func TestActiveByCCXActiveProfileEnv(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg := makeAbsDir(t, "work")
	if err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: cfg}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	t.Setenv("CCX_ACTIVE_PROFILE", "work")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	got, ok, err := mgr.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.Name != "work" || got.ConfigDir != cfg {
		t.Errorf("got %+v, want name=work config=%q", got, cfg)
	}
}

func TestActiveCCXActiveProfileNotInRegistryIsError(t *testing.T) {
	t.Setenv("CCX_ACTIVE_PROFILE", "ghost")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	ctx := context.Background()
	mgr := newTestManager(t)

	_, ok, err := mgr.Active(ctx)
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v (ok=%v)", err, ok)
	}
}

func TestActiveByConfigDirEnv(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg := makeAbsDir(t, "personal")
	if err := mgr.Add(ctx, contracts.Profile{Name: "personal", ConfigDir: cfg}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	t.Setenv("CCX_ACTIVE_PROFILE", "")
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)

	got, ok, err := mgr.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.Name != "personal" {
		t.Errorf("got name=%q, want personal", got.Name)
	}
}

func TestActiveConfigDirNotInRegistryReturnsUnmanaged(t *testing.T) {
	// CLAUDE_CONFIG_DIR is set but does not match any registered profile.
	// Per spec section 6 ("Active-profile detection") this is reported as
	// "unmanaged config" rather than an error: ok=false, err=ErrNoActiveProfile.
	t.Setenv("CCX_ACTIVE_PROFILE", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "/nowhere/in/registry")

	ctx := context.Background()
	mgr := newTestManager(t)

	_, ok, err := mgr.Active(ctx)
	if ok {
		t.Fatal("expected ok=false for unmanaged config dir")
	}
	if !errors.Is(err, contracts.ErrNoActiveProfile) {
		t.Fatalf("expected ErrNoActiveProfile, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/profile/...
```
Expected: FAIL — `mgr.Active` undefined.

- [ ] **Step 3: Implement `Active`**

Append to `internal/profile/manager.go`:

```go
// Environment variable names that control active-profile detection. Defined
// here so tests and callers reference a single source of truth.
const (
	EnvActiveProfile = "CCX_ACTIVE_PROFILE"
	EnvConfigDir     = "CLAUDE_CONFIG_DIR"
)

// Active returns the active profile, if any, plus a boolean indicating
// whether one was found.
//
// Resolution order, per spec section 6:
//  1. If CCX_ACTIVE_PROFILE is set, look it up in the registry.
//     Found → return (p, true, nil). Not found → (zero, false, ErrProfileNotFound wrapped).
//  2. Else, if CLAUDE_CONFIG_DIR is set, search the registry by ConfigDir.
//     Found → return (p, true, nil). Not found → (zero, false, ErrNoActiveProfile wrapped)
//     to indicate an "unmanaged" config dir.
//  3. Else, return (zero, false, nil) — no active profile and no error.
func (m *Manager) Active(ctx context.Context) (contracts.Profile, bool, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Profile{}, false, err
	}

	if name := os.Getenv(EnvActiveProfile); name != "" {
		p, err := m.Get(ctx, name)
		if err != nil {
			return contracts.Profile{}, false, err
		}
		return p, true, nil
	}

	if cfg := os.Getenv(EnvConfigDir); cfg != "" {
		m.mu.Lock()
		reg, err := loadRegistry(m.Path())
		m.mu.Unlock()
		if err != nil {
			return contracts.Profile{}, false, err
		}
		for _, p := range reg.Profiles {
			if p.ConfigDir == cfg {
				return p, true, nil
			}
		}
		return contracts.Profile{}, false, fmt.Errorf("CLAUDE_CONFIG_DIR=%q not in registry: %w", cfg, contracts.ErrNoActiveProfile)
	}

	return contracts.Profile{}, false, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test -race -count=1 ./internal/profile/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/manager.go internal/profile/manager_test.go
git commit -m "feat(profile): implement Manager.Active via env vars"
```

---

## Task 13: Stale `.tmp` survives a simulated crash (TDD)

This task adds a Manager-level test on top of the lower-level io test, exercising the full read path.

**Files:**
- Modify: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the failing/passing test**

Append to `internal/profile/manager_test.go`:

```go
func TestManagerSurvivesLeftoverTmpFile(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	cfg := makeAbsDir(t, "work")

	if err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: cfg}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Simulate a crash during a future write: a stale .tmp containing garbage.
	if err := os.WriteFile(mgr.Path()+".tmp", []byte("garbage =="), 0o600); err != nil {
		t.Fatalf("seed .tmp: %v", err)
	}

	got, err := mgr.Get(ctx, "work")
	if err != nil {
		t.Fatalf("Get after stale .tmp: %v", err)
	}
	if got.Name != "work" {
		t.Errorf("got %q, want work", got.Name)
	}

	// Subsequent writes must still succeed and replace the .tmp cleanly.
	cfg2 := makeAbsDir(t, "side")
	if err := mgr.Add(ctx, contracts.Profile{Name: "side", ConfigDir: cfg2}); err != nil {
		t.Fatalf("Add after stale .tmp: %v", err)
	}
	if _, err := os.Stat(mgr.Path() + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("stale .tmp should be gone after successful Add, got err=%v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run:
```bash
go test -race -count=1 ./internal/profile/... -run TestManagerSurvivesLeftoverTmpFile
```
Expected: PASS (the implementation from Task 4 already handles this; this test pins the behavior at the Manager level).

- [ ] **Step 3: Commit**

```bash
git add internal/profile/manager_test.go
git commit -m "test(profile): pin behavior on stale .tmp file"
```

---

## Task 14: Concurrent reads are safe (TDD)

**Files:**
- Modify: `internal/profile/manager_test.go`

- [ ] **Step 1: Write the parallel test**

Append to `internal/profile/manager_test.go`:

```go
import (
	"sync"
)
```

(Adjust the existing import block of `manager_test.go` so `sync` is included alongside the other imports.)

Append the test:

```go
func TestConcurrentReadsAreSafe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mgr := newTestManager(t)
	for _, name := range []string{"a", "b", "c", "d"} {
		cfg := makeAbsDir(t, name)
		if err := mgr.Add(ctx, contracts.Profile{Name: name, ConfigDir: cfg}); err != nil {
			t.Fatalf("Add %s: %v", name, err)
		}
	}

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if _, err := mgr.List(ctx); err != nil {
					errCh <- err
					return
				}
				if _, err := mgr.Get(ctx, "b"); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent read error: %v", err)
	}
}

func TestConcurrentManagersOnDistinctRootsAreSafe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			mgr := newTestManager(t)
			cfg := makeAbsDir(t, "w")
			if err := mgr.Add(ctx, contracts.Profile{Name: "w", ConfigDir: cfg}); err != nil {
				errCh <- err
				return
			}
			if _, err := mgr.Get(ctx, "w"); err != nil {
				errCh <- err
				return
			}
			if err := mgr.Remove(ctx, "w"); err != nil {
				errCh <- err
				return
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("isolated-manager goroutine error: %v", err)
	}
}
```

- [ ] **Step 2: Run with race detector**

Run:
```bash
go test -race -count=1 ./internal/profile/...
```
Expected: PASS, no race detected.

- [ ] **Step 3: Commit**

```bash
git add internal/profile/manager_test.go
git commit -m "test(profile): cover concurrent reads under -race"
```

---

## Task 15: Local CI gate and PR

- [ ] **Step 1: Format check**

```bash
gofumpt -l ./internal/profile
```
Expected: no output. If any files are listed, run `gofumpt -w ./internal/profile`, inspect the diff with `git diff`, and commit any real changes:
```bash
git add -u internal/profile
git commit -m "style(profile): gofumpt"
```

- [ ] **Step 2: Lint**

```bash
golangci-lint run ./internal/profile/...
```
Expected: exit 0, no issues.

- [ ] **Step 3: Vet and build**

```bash
go vet ./internal/profile/...
go build ./internal/profile/...
```
Expected: no output.

- [ ] **Step 4: Full test pass with race detector**

```bash
go test -race -count=1 ./internal/profile/...
```
Expected: all tests PASS.

- [ ] **Step 5: Push the branch and open a PR**

```bash
git push -u origin feat/profile
gh pr create \
  --base main \
  --head feat/profile \
  --title "feat(profile): TOML registry with CRUD, validation, atomic writes" \
  --body "Implements internal/profile per plan A1.

- ProfileManager backed by ~/.ccx/profiles.toml
- Methods: Add, Get, List, Remove, MarkUsed, Active (all context.Context-aware)
- Validation: absolute ConfigDir, no duplicate name or ConfigDir, well-formed name
- Atomic writes via os.Rename from profiles.toml.tmp
- Active-profile detection via CCX_ACTIVE_PROFILE / CLAUDE_CONFIG_DIR
- Tests: TOML roundtrip, validation table, atomic write, stale .tmp survival, concurrent reads under -race, env-driven Active

Contract impact: none."
```

- [ ] **Step 6: Watch CI**

```bash
gh pr checks --watch
```

Expected: all jobs green. If a job fails, fix on this branch and push again — do not merge until green.

---

## Done definition

All of the following are true:

- [ ] `go build ./internal/profile/...` succeeds
- [ ] `go test -race -count=1 ./internal/profile/...` passes
- [ ] `golangci-lint run ./internal/profile/...` reports zero issues
- [ ] `gofumpt -l ./internal/profile` produces no output
- [ ] `Manager` exposes: `Add`, `Get`, `List`, `Remove`, `MarkUsed`, `Active`, `Root`, `Path`
- [ ] `Add` returns wrapped `contracts.ErrInvalidConfigDir`, `contracts.ErrProfileAlreadyExists`, `contracts.ErrConfigDirConflict` for their respective failure modes
- [ ] `Get`, `Remove`, `MarkUsed` return wrapped `contracts.ErrProfileNotFound` for missing profiles
- [ ] `Active` returns wrapped `contracts.ErrNoActiveProfile` for an unmanaged CLAUDE_CONFIG_DIR
- [ ] All sentinel errors detectable via `errors.Is`
- [ ] `go.mod` declares only `github.com/pelletier/go-toml/v2` as a new direct dependency for this package
- [ ] PR opened against `main`, CI green, ready for review

After merge, the worktree may be removed:
```bash
cd /Users/arafa/Developer/ccx
git worktree remove ../ccx-profile
git branch -d feat/profile
```
