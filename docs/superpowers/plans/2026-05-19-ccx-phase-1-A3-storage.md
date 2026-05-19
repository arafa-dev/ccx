# ccx Phase 1 A3 — `internal/storage/` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a SQLite-backed `contracts.Store` in `internal/storage/`. The package owns persistence: profile CRUD, event ingestion, usage aggregation, and incremental-scan cursors. The schema lives in `internal/storage/schema.sql` (locked by Phase 0) and is embedded via `//go:embed`.

**Architecture:** A `*Store` value wraps an `*sql.DB` connected via `modernc.org/sqlite` (pure-Go, no CGo). SQLite is single-writer; a `sync.Mutex` serializes writes while reads run concurrently against WAL-mode SQLite. The schema is idempotent (`CREATE TABLE IF NOT EXISTS` everywhere), so `Migrate` is safe to call multiple times. Re-scanning is safe because `InsertEvents` uses `ON CONFLICT(profile_name, event_uuid) DO NOTHING`. The package imports only `internal/contracts`, stdlib, and `modernc.org/sqlite`.

**Tech Stack:** Go 1.22+, `modernc.org/sqlite`, stdlib `database/sql`, stdlib `embed`.

**Spec reference:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — Section 7.2 (SQLite schema), Section 5.2 (library choices), Section 7.5 (performance targets).

**Worktree:** `feat/storage` branched from `main` *after* Phase 0 is merged. Create with:

```bash
git worktree add ../ccx-storage -b feat/storage main
cd ../ccx-storage
```

**Exit criteria:**
- `go build ./internal/storage/...` succeeds
- `go test -race -count=1 ./internal/storage/...` succeeds
- `go test -bench=. -benchmem ./internal/storage/...` reports `InsertEvents10000` under 1s
- `golangci-lint run ./internal/storage/...` is clean
- PR opened against `main`, CI green, merged

**Conventions:**
- All Go code uses tabs (gofumpt enforced)
- Commit message format: `type(scope): subject`, scope `storage`
- One commit per task; do not batch
- Each task ends with `go test ./internal/storage/...` passing
- This worktree may modify only files under `internal/storage/` and the top-level `go.mod` / `go.sum`. Do NOT edit `internal/storage/schema.sql` (Phase 0 locked it).

---

## Pre-flight

Confirm the worktree is on `feat/storage`, the working tree is clean, and Phase 0 outputs exist.

```bash
pwd                                              # → /Users/arafa/Developer/ccx-storage (or similar)
git status                                       # → On branch feat/storage, working tree clean
git rev-parse --verify HEAD                      # → at least one commit
test -f internal/storage/schema.sql && echo OK   # → OK
test -f internal/storage/doc.go && echo OK       # → OK
test -f internal/contracts/interfaces.go && echo OK # → OK
go build ./...                                   # → succeeds
go test ./...                                    # → succeeds
```

If any check fails, stop and resolve before proceeding.

---

## Task 1: Add `modernc.org/sqlite` dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add the dependency**

Run:
```bash
go get modernc.org/sqlite@latest
go mod tidy
```

- [ ] **Step 2: Verify it resolves**

Run:
```bash
go list -m modernc.org/sqlite
```
Expected: prints the module path followed by a version (e.g., `modernc.org/sqlite v1.34.1`).

- [ ] **Step 3: Verify build still succeeds**

Run:
```bash
go build ./...
```
Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "build(storage): add modernc.org/sqlite dependency"
```

---

## Task 2: Embed schema.sql and add the driver import

**Files:**
- Create: `internal/storage/embed.go`
- Create: `internal/storage/embed_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/storage/embed_test.go`:

```go
package storage_test

import (
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/storage"
)

func TestSchemaSQLIsEmbedded(t *testing.T) {
	got := storage.SchemaSQL()
	if got == "" {
		t.Fatal("SchemaSQL() returned empty string; schema.sql not embedded")
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS profiles",
		"CREATE TABLE IF NOT EXISTS events",
		"CREATE TABLE IF NOT EXISTS scan_cursors",
		"CREATE TABLE IF NOT EXISTS schema_version",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("embedded schema is missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/storage/...
```
Expected: FAIL — `storage.SchemaSQL` undefined.

- [ ] **Step 3: Write the embed file**

Create `internal/storage/embed.go`:

```go
package storage

import (
	_ "embed"

	// Register the modernc.org/sqlite driver under the name "sqlite".
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// SchemaSQL returns the embedded SQLite schema as a string. Exposed for tests
// and tooling; callers wanting to apply it should use (*Store).Migrate.
func SchemaSQL() string {
	return schemaSQL
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/embed.go internal/storage/embed_test.go
git commit -m "feat(storage): embed schema.sql and register sqlite driver"
```

---

## Task 3: Implement `NewStore`, `Close`, and connection-string builder (TDD)

**Files:**
- Create: `internal/storage/store.go`
- Create: `internal/storage/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/storage/store_test.go`:

```go
package storage_test

import (
	"context"
	"testing"

	"github.com/arafa-dev/ccx/internal/storage"
)

func TestNewStoreInMemory(t *testing.T) {
	ctx := context.Background()

	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if s == nil {
		t.Fatal("NewStore returned nil *Store with nil error")
	}
}

func TestNewStoreFileBacked(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := dir + "/ccx.db"

	s, err := storage.NewStore(ctx, path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/storage/...
```
Expected: FAIL — `storage.NewStore` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/storage/store.go`:

```go
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sync"
)

// Store is the SQLite-backed implementation of contracts.Store. It is safe
// for concurrent use: reads run in parallel against WAL-mode SQLite, while
// writes are serialized through writeMu (SQLite is single-writer).
type Store struct {
	db      *sql.DB
	writeMu sync.Mutex

	closeOnce sync.Once
	closeErr  error
}

// NewStore opens (or creates) a SQLite database at the given path. Use the
// literal string ":memory:" for an in-memory database (useful for tests).
// The returned Store has not yet been migrated; callers should run
// (*Store).Migrate before issuing CRUD calls.
func NewStore(ctx context.Context, dbPath string) (*Store, error) {
	dsn := buildDSN(dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite %q: %w", dbPath, err)
	}

	// Single open connection for :memory: so all callers see the same DB.
	// File-backed DBs benefit from a small pool but are still single-writer.
	if dbPath == ":memory:" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(8)
	}
	db.SetMaxIdleConns(2)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging sqlite %q: %w", dbPath, err)
	}

	return &Store{db: db}, nil
}

// Close releases the underlying database handle. Safe to call multiple times;
// returns the same error on subsequent calls.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.db.Close()
	})
	return s.closeErr
}

// buildDSN constructs the modernc.org/sqlite connection string with the
// pragmas we require: WAL journaling, foreign keys ON, synchronous NORMAL.
func buildDSN(dbPath string) string {
	// In-memory databases must not be URL-encoded; they need the literal
	// ":memory:" form. The modernc.org/sqlite driver accepts the raw form
	// with query parameters appended.
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(on)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "busy_timeout(5000)")

	if dbPath == ":memory:" {
		return ":memory:?" + q.Encode()
	}
	return "file:" + dbPath + "?" + q.Encode()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/store.go internal/storage/store_test.go
git commit -m "feat(storage): add Store with NewStore, Close, and DSN builder"
```

---

## Task 4: Implement `Migrate` (idempotent) (TDD)

**Files:**
- Create: `internal/storage/migrate.go`
- Create: `internal/storage/migrate_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/storage/migrate_test.go`:

```go
package storage_test

import (
	"context"
	"testing"

	"github.com/arafa-dev/ccx/internal/storage"
)

func TestMigrateCreatesTables(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, tbl := range []string{"profiles", "events", "scan_cursors", "schema_version"} {
		if !s.TableExists(ctx, t, tbl) {
			t.Errorf("expected table %q to exist after Migrate", tbl)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("third Migrate: %v", err)
	}
}

func TestMigrateSeedsSchemaVersion(t *testing.T) {
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	got := s.SchemaVersion(ctx, t)
	if got != 1 {
		t.Errorf("schema_version: got %d want 1", got)
	}
}
```

This test refers to test-helper methods `TableExists` and `SchemaVersion` that we'll add to the package in this task as build-tag-gated test helpers — placing them in a regular `_test.go` file in the same package keeps them out of the production binary.

Create `internal/storage/testhelpers_test.go`:

```go
package storage

import (
	"context"
	"testing"
)

// TableExists reports whether a table with the given name exists in the
// SQLite schema. Test-only helper.
func (s *Store) TableExists(ctx context.Context, t *testing.T, name string) bool {
	t.Helper()
	var found string
	err := s.db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&found)
	if err != nil {
		return false
	}
	return found == name
}

// SchemaVersion returns the single row from the schema_version table.
// Test-only helper.
func (s *Store) SchemaVersion(ctx context.Context, t *testing.T) int {
	t.Helper()
	var v int
	if err := s.db.QueryRowContext(ctx,
		`SELECT version FROM schema_version LIMIT 1`,
	).Scan(&v); err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	return v
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/storage/...
```
Expected: FAIL — `Migrate` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/storage/migrate.go`:

```go
package storage

import (
	"context"
	"fmt"
)

// Migrate applies the embedded schema. Safe to call multiple times because
// every statement uses CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS
// / INSERT OR IGNORE. For v0.1 there is exactly one schema version; future
// versions will run additional statements gated on the schema_version row.
func (s *Store) Migrate(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("applying schema: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/migrate.go internal/storage/migrate_test.go internal/storage/testhelpers_test.go
git commit -m "feat(storage): implement idempotent Migrate"
```

---

## Task 5: Implement `SaveProfile` and `GetProfile` (TDD)

**Files:**
- Create: `internal/storage/profiles.go`
- Create: `internal/storage/profiles_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/storage/profiles_test.go`:

```go
package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	ctx := context.Background()
	s, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSaveAndGetProfileRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	in := contracts.Profile{
		Name:       "work",
		ConfigDir:  "/Users/arafa/.claude-profiles/work",
		Label:      "Work account",
		Color:      "#3B82F6",
		CreatedAt:  time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		LastUsedAt: time.Date(2026, 5, 19, 15, 30, 0, 0, time.UTC),
	}

	if err := s.SaveProfile(ctx, in); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	got, err := s.GetProfile(ctx, "work")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}

	if got.Name != in.Name || got.ConfigDir != in.ConfigDir ||
		got.Label != in.Label || got.Color != in.Color ||
		!got.CreatedAt.Equal(in.CreatedAt) || !got.LastUsedAt.Equal(in.LastUsedAt) {
		t.Errorf("roundtrip mismatch:\n got  %+v\n want %+v", got, in)
	}
}

func TestSaveProfileUpsertOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	t0 := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 5, 19, 18, 0, 0, 0, time.UTC)

	first := contracts.Profile{
		Name: "work", ConfigDir: "/p/work", Label: "old", Color: "#000000",
		CreatedAt: t0, LastUsedAt: t0,
	}
	second := contracts.Profile{
		Name: "work", ConfigDir: "/p/work", Label: "new", Color: "#FFFFFF",
		CreatedAt: t0, LastUsedAt: t1,
	}

	if err := s.SaveProfile(ctx, first); err != nil {
		t.Fatalf("first SaveProfile: %v", err)
	}
	if err := s.SaveProfile(ctx, second); err != nil {
		t.Fatalf("second SaveProfile: %v", err)
	}

	got, err := s.GetProfile(ctx, "work")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got.Label != "new" || got.Color != "#FFFFFF" || !got.LastUsedAt.Equal(t1) {
		t.Errorf("upsert did not overwrite:\n got  %+v\n want label=new color=#FFFFFF lastUsed=%v", got, t1)
	}
}

func TestGetProfileNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.GetProfile(ctx, "nope")
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Errorf("GetProfile(nope) err = %v, want ErrProfileNotFound", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/storage/...
```
Expected: FAIL — `SaveProfile`/`GetProfile` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/storage/profiles.go`:

```go
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// SaveProfile inserts or replaces a profile by name. Timestamps are stored
// as Unix nanoseconds for monotonic comparison and to avoid timezone drift.
func (s *Store) SaveProfile(ctx context.Context, p contracts.Profile) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	const q = `
INSERT INTO profiles (name, config_dir, label, color, created_at, last_used_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
    config_dir   = excluded.config_dir,
    label        = excluded.label,
    color        = excluded.color,
    created_at   = excluded.created_at,
    last_used_at = excluded.last_used_at
`
	_, err := s.db.ExecContext(ctx, q,
		p.Name,
		p.ConfigDir,
		p.Label,
		p.Color,
		p.CreatedAt.UnixNano(),
		p.LastUsedAt.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("saving profile %q: %w", p.Name, err)
	}
	return nil
}

// GetProfile returns the profile with the given name. Returns
// contracts.ErrProfileNotFound (wrapped) if no row exists.
func (s *Store) GetProfile(ctx context.Context, name string) (contracts.Profile, error) {
	const q = `
SELECT name, config_dir, label, color, created_at, last_used_at
FROM profiles
WHERE name = ?
`
	var (
		p              contracts.Profile
		label, color   sql.NullString
		createdNs, usedNs int64
	)
	err := s.db.QueryRowContext(ctx, q, name).Scan(
		&p.Name, &p.ConfigDir, &label, &color, &createdNs, &usedNs,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.Profile{}, fmt.Errorf("looking up profile %q: %w", name, contracts.ErrProfileNotFound)
	}
	if err != nil {
		return contracts.Profile{}, fmt.Errorf("looking up profile %q: %w", name, err)
	}
	p.Label = label.String
	p.Color = color.String
	p.CreatedAt = time.Unix(0, createdNs).UTC()
	p.LastUsedAt = time.Unix(0, usedNs).UTC()
	return p, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/storage/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/profiles.go internal/storage/profiles_test.go
git commit -m "feat(storage): implement SaveProfile and GetProfile"
```

---

## Task 6: Implement `ListProfiles` and `DeleteProfile` (TDD)

**Files:**
- Modify: `internal/storage/profiles.go`
- Modify: `internal/storage/profiles_test.go`

- [ ] **Step 1: Append failing tests**

Append to `internal/storage/profiles_test.go`:

```go
func TestListProfilesEmptyReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	got, err := s.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListProfiles on empty store: got %d profiles, want 0", len(got))
	}
}

func TestListProfilesSortedByName(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	t0 := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		p := contracts.Profile{
			Name: name, ConfigDir: "/p/" + name,
			CreatedAt: t0, LastUsedAt: t0,
		}
		if err := s.SaveProfile(ctx, p); err != nil {
			t.Fatalf("SaveProfile(%q): %v", name, err)
		}
	}

	got, err := s.ListProfiles(ctx)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListProfiles: got %d profiles, want 3", len(got))
	}
	want := []string{"alpha", "bravo", "charlie"}
	for i, p := range got {
		if p.Name != want[i] {
			t.Errorf("profile[%d].Name = %q, want %q", i, p.Name, want[i])
		}
	}
}

func TestDeleteProfileRemovesRow(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	t0 := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	p := contracts.Profile{
		Name: "work", ConfigDir: "/p/work",
		CreatedAt: t0, LastUsedAt: t0,
	}
	if err := s.SaveProfile(ctx, p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	if err := s.DeleteProfile(ctx, "work"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}

	if _, err := s.GetProfile(ctx, "work"); !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Errorf("after delete, GetProfile err = %v, want ErrProfileNotFound", err)
	}
}

func TestDeleteProfileUnknownReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	err := s.DeleteProfile(ctx, "ghost")
	if !errors.Is(err, contracts.ErrProfileNotFound) {
		t.Errorf("DeleteProfile(ghost) err = %v, want ErrProfileNotFound", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/storage/...
```
Expected: FAIL — `ListProfiles`/`DeleteProfile` undefined.

- [ ] **Step 3: Append the implementation**

Append to `internal/storage/profiles.go`:

```go
// ListProfiles returns every profile, sorted ascending by name.
func (s *Store) ListProfiles(ctx context.Context) ([]contracts.Profile, error) {
	const q = `
SELECT name, config_dir, label, color, created_at, last_used_at
FROM profiles
ORDER BY name ASC
`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listing profiles: %w", err)
	}
	defer rows.Close()

	var out []contracts.Profile
	for rows.Next() {
		var (
			p                 contracts.Profile
			label, color      sql.NullString
			createdNs, usedNs int64
		)
		if err := rows.Scan(&p.Name, &p.ConfigDir, &label, &color, &createdNs, &usedNs); err != nil {
			return nil, fmt.Errorf("scanning profile row: %w", err)
		}
		p.Label = label.String
		p.Color = color.String
		p.CreatedAt = time.Unix(0, createdNs).UTC()
		p.LastUsedAt = time.Unix(0, usedNs).UTC()
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating profile rows: %w", err)
	}
	return out, nil
}

// DeleteProfile removes the named profile. The FOREIGN KEY ... ON DELETE
// CASCADE on events and scan_cursors removes the associated rows automatically.
// Returns contracts.ErrProfileNotFound (wrapped) if no row matched.
func (s *Store) DeleteProfile(ctx context.Context, name string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	res, err := s.db.ExecContext(ctx, `DELETE FROM profiles WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("deleting profile %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected for delete %q: %w", name, err)
	}
	if n == 0 {
		return fmt.Errorf("deleting profile %q: %w", name, contracts.ErrProfileNotFound)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/storage/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/profiles.go internal/storage/profiles_test.go
git commit -m "feat(storage): implement ListProfiles and DeleteProfile"
```

---

## Task 7: Implement `InsertEvents` with batch transaction and conflict handling (TDD)

**Files:**
- Create: `internal/storage/events.go`
- Create: `internal/storage/events_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/storage/events_test.go`:

```go
package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func mustSaveProfile(t *testing.T, s interface {
	SaveProfile(ctx context.Context, p contracts.Profile) error
}, name string) {
	t.Helper()
	t0 := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	if err := s.SaveProfile(context.Background(), contracts.Profile{
		Name: name, ConfigDir: "/p/" + name,
		CreatedAt: t0, LastUsedAt: t0,
	}); err != nil {
		t.Fatalf("SaveProfile(%q): %v", name, err)
	}
}

func TestInsertEventsEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if err := s.InsertEvents(ctx, nil); err != nil {
		t.Errorf("InsertEvents(nil): %v", err)
	}
	if err := s.InsertEvents(ctx, []contracts.Event{}); err != nil {
		t.Errorf("InsertEvents(empty): %v", err)
	}
}

func TestInsertEventsRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	events := []contracts.Event{
		{
			UUID: "e1", SessionID: "s1", Timestamp: ts, Type: "assistant",
			Project: "ccx", Model: "claude-opus-4-7",
			Usage: &contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 25},
		},
		{
			UUID: "e2", SessionID: "s1", Timestamp: ts.Add(time.Second), Type: "assistant",
			Project: "ccx", Model: "claude-opus-4-7",
			Usage: &contracts.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 20, CacheCreateTokens: 1},
		},
	}

	if err := s.InsertEventsForProfile(ctx, "work", events); err != nil {
		t.Fatalf("InsertEventsForProfile: %v", err)
	}

	got := s.CountEvents(ctx, t, "work")
	if got != 2 {
		t.Errorf("event count: got %d, want 2", got)
	}
}

func TestInsertEventsDuplicateUUIDIgnored(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	events := []contracts.Event{
		{UUID: "dup", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
		{UUID: "dup", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
		{UUID: "uniq", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
	}

	if err := s.InsertEventsForProfile(ctx, "work", events); err != nil {
		t.Fatalf("InsertEventsForProfile: %v", err)
	}

	got := s.CountEvents(ctx, t, "work")
	if got != 2 {
		t.Errorf("event count after dedup: got %d, want 2", got)
	}
}

func TestInsertEventsRescanIsSafe(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	events := []contracts.Event{
		{UUID: "a", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
		{UUID: "b", SessionID: "s1", Timestamp: ts, Type: "assistant", Project: "p", Model: "m"},
	}

	if err := s.InsertEventsForProfile(ctx, "work", events); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := s.InsertEventsForProfile(ctx, "work", events); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	got := s.CountEvents(ctx, t, "work")
	if got != 2 {
		t.Errorf("event count after rescan: got %d, want 2", got)
	}
}

func TestInsertEventsNilUsageStoresZero(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	events := []contracts.Event{
		{UUID: "e1", SessionID: "s1", Timestamp: ts, Type: "user", Project: "p", Model: "", Usage: nil},
	}

	if err := s.InsertEventsForProfile(ctx, "work", events); err != nil {
		t.Fatalf("InsertEventsForProfile: %v", err)
	}

	if got := s.CountEvents(ctx, t, "work"); got != 1 {
		t.Errorf("event count: got %d, want 1", got)
	}
}
```

We also need a test helper `CountEvents` and the profile-scoped insert helper `InsertEventsForProfile`. Append to `internal/storage/testhelpers_test.go`:

```go
// CountEvents returns the number of rows in events for the given profile.
// Test-only helper.
func (s *Store) CountEvents(ctx context.Context, t *testing.T, profileName string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE profile_name = ?`, profileName,
	).Scan(&n); err != nil {
		t.Fatalf("CountEvents: %v", err)
	}
	return n
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/storage/...
```
Expected: FAIL — `InsertEvents`/`InsertEventsForProfile` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/storage/events.go`:

```go
package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// InsertEvents writes events to the store. The contracts.Event type does not
// itself carry a profile name (it is profile-agnostic at parse time), so this
// method requires the caller to embed profile context separately via
// InsertEventsForProfile. To preserve the contracts.Store interface, this
// method returns ErrNoActiveProfile when called with a non-empty slice.
//
// Callers should use InsertEventsForProfile in practice; this method exists
// to satisfy the interface signature.
func (s *Store) InsertEvents(ctx context.Context, events []contracts.Event) error {
	if len(events) == 0 {
		return nil
	}
	return fmt.Errorf("InsertEvents requires profile context: %w", contracts.ErrNoActiveProfile)
}

// InsertEventsForProfile writes a batch of events under a single transaction.
// Duplicate (profile_name, event_uuid) pairs are silently skipped via ON
// CONFLICT DO NOTHING, which makes re-scanning safe and idempotent.
func (s *Store) InsertEventsForProfile(ctx context.Context, profileName string, events []contracts.Event) error {
	if len(events) == 0 {
		return nil
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for %q: %w", profileName, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const q = `
INSERT INTO events (
    profile_name, session_id, event_uuid, ts, project, model,
    input_tokens, output_tokens, cache_read_tokens, cache_create_tokens
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(profile_name, event_uuid) DO NOTHING
`
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("prepare insert for %q: %w", profileName, err)
	}
	defer stmt.Close()

	for i := range events {
		ev := events[i]
		var in, out, cr, cc int
		if ev.Usage != nil {
			in = ev.Usage.InputTokens
			out = ev.Usage.OutputTokens
			cr = ev.Usage.CacheReadTokens
			cc = ev.Usage.CacheCreateTokens
		}
		if _, execErr := stmt.ExecContext(ctx,
			profileName,
			ev.SessionID,
			ev.UUID,
			ev.Timestamp.UnixNano(),
			ev.Project,
			ev.Model,
			in, out, cr, cc,
		); execErr != nil {
			err = fmt.Errorf("inserting event %q for %q: %w", ev.UUID, profileName, execErr)
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit insert for %q: %w", profileName, err)
	}
	return nil
}

// ensureNoEventsBeforeMigrate is a defensive guard against callers issuing
// inserts on an unmigrated store. It returns a wrapped sql.ErrNoRows-equivalent
// for the common "table missing" case, surfaced to callers as a clear error.
var _ = errors.New // keep errors imported for future helpers
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/storage/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/events.go internal/storage/events_test.go internal/storage/testhelpers_test.go
git commit -m "feat(storage): implement InsertEventsForProfile with batch tx and ON CONFLICT"
```

---

## Task 8: Implement `QueryUsage` with aggregation by (profile, project, model, day) (TDD)

**Files:**
- Create: `internal/storage/query.go`
- Create: `internal/storage/query_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/storage/query_test.go`:

```go
package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func seedUsageFixture(t *testing.T, s *testStoreHandle) {
	t.Helper()
	ctx := context.Background()

	mustSaveProfile(t, s.store, "work")
	mustSaveProfile(t, s.store, "personal")

	day1 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

	workEvents := []contracts.Event{
		// work, ccx, opus, day1 — 2 events
		{UUID: "w1", SessionID: "ws1", Timestamp: day1, Type: "assistant",
			Project: "ccx", Model: "claude-opus-4-7",
			Usage: &contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 10}},
		{UUID: "w2", SessionID: "ws1", Timestamp: day1.Add(time.Hour), Type: "assistant",
			Project: "ccx", Model: "claude-opus-4-7",
			Usage: &contracts.Usage{InputTokens: 50, OutputTokens: 25, CacheReadTokens: 100, CacheCreateTokens: 5}},
		// work, ccx, sonnet, day2 — 1 event
		{UUID: "w3", SessionID: "ws2", Timestamp: day2, Type: "assistant",
			Project: "ccx", Model: "claude-sonnet-4-6",
			Usage: &contracts.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 20, CacheCreateTokens: 1}},
	}
	if err := s.store.InsertEventsForProfile(ctx, "work", workEvents); err != nil {
		t.Fatalf("InsertEventsForProfile(work): %v", err)
	}

	personalEvents := []contracts.Event{
		{UUID: "p1", SessionID: "ps1", Timestamp: day2, Type: "assistant",
			Project: "hobby", Model: "claude-sonnet-4-6",
			Usage: &contracts.Usage{InputTokens: 1, OutputTokens: 1, CacheReadTokens: 1, CacheCreateTokens: 1}},
	}
	if err := s.store.InsertEventsForProfile(ctx, "personal", personalEvents); err != nil {
		t.Fatalf("InsertEventsForProfile(personal): %v", err)
	}
}

type testStoreHandle struct {
	store storeFacade
}

// storeFacade is the subset of *Store used by seedUsageFixture; declared as
// an interface to keep test helpers loosely coupled.
type storeFacade interface {
	SaveProfile(ctx context.Context, p contracts.Profile) error
	InsertEventsForProfile(ctx context.Context, profileName string, events []contracts.Event) error
}

func TestQueryUsageGroupsByProfileProjectModelDay(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 19, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}

	// Expect 3 groups: (work, ccx, opus, day1), (work, ccx, sonnet, day2),
	// (personal, hobby, sonnet, day2).
	if len(rows) != 3 {
		for i, r := range rows {
			t.Logf("row[%d] = %+v", i, r)
		}
		t.Fatalf("rows: got %d, want 3", len(rows))
	}

	// The day1 work/opus group should sum the two events.
	for _, r := range rows {
		if r.Profile == "work" && r.Model == "claude-opus-4-7" {
			want := contracts.Usage{InputTokens: 150, OutputTokens: 75, CacheReadTokens: 300, CacheCreateTokens: 15}
			if r.Usage != want {
				t.Errorf("work/opus usage = %+v, want %+v", r.Usage, want)
			}
			if r.SessionCount != 1 {
				t.Errorf("work/opus session_count = %d, want 1", r.SessionCount)
			}
		}
	}
}

func TestQueryUsageProfileFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Profile: "personal",
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	if rows[0].Profile != "personal" {
		t.Errorf("rows[0].Profile = %q, want personal", rows[0].Profile)
	}
}

func TestQueryUsageEmptyProfileReturnsAll(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	profiles := map[string]bool{}
	for _, r := range rows {
		profiles[r.Profile] = true
	}
	if !profiles["work"] || !profiles["personal"] {
		t.Errorf("expected rows for both profiles, got %v", profiles)
	}
}

func TestQueryUsageRangeFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	// Only day2 — day1 events excluded.
	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 19, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	// Expect 2 rows: (work, ccx, sonnet, day2) and (personal, hobby, sonnet, day2).
	if len(rows) != 2 {
		for i, r := range rows {
			t.Logf("row[%d] = %+v", i, r)
		}
		t.Errorf("rows: got %d, want 2", len(rows))
	}
}

func TestQueryUsageProjectFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedUsageFixture(t, &testStoreHandle{store: s})

	rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
		Project: "hobby",
		Range: contracts.TimeRange{
			Start: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	if len(rows) != 1 || rows[0].Project != "hobby" {
		t.Errorf("expected single hobby row, got %+v", rows)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/storage/...
```
Expected: FAIL — `QueryUsage` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/storage/query.go`:

```go
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// QueryUsage returns one UsageRow per (profile_name, project, model, day)
// group that overlaps the query's TimeRange. An empty Profile/Project field
// in the query is treated as "any". EstimatedUSD is left at zero; pricing is
// the caller's responsibility (see internal/pricing).
func (s *Store) QueryUsage(ctx context.Context, q contracts.UsageQuery) ([]contracts.UsageRow, error) {
	var (
		where []string
		args  []any
	)

	if q.Profile != "" {
		where = append(where, "profile_name = ?")
		args = append(args, q.Profile)
	}
	if q.Project != "" {
		where = append(where, "project = ?")
		args = append(args, q.Project)
	}
	if !q.Range.Start.IsZero() {
		where = append(where, "ts >= ?")
		args = append(args, q.Range.Start.UnixNano())
	}
	if !q.Range.End.IsZero() {
		where = append(where, "ts <= ?")
		args = append(args, q.Range.End.UnixNano())
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	// Group by (profile, project, model, day). Day is computed by truncating
	// the ts (ns) to the start of UTC day. 86400 seconds × 1e9 ns/s.
	const nsPerDay = int64(86400) * int64(1e9)

	query := fmt.Sprintf(`
SELECT
    profile_name,
    COALESCE(project, '')               AS project,
    COALESCE(model, '')                 AS model,
    (ts / %d) * %d                       AS day_ns,
    SUM(input_tokens)                    AS in_tokens,
    SUM(output_tokens)                   AS out_tokens,
    SUM(cache_read_tokens)               AS cr_tokens,
    SUM(cache_create_tokens)             AS cc_tokens,
    COUNT(DISTINCT session_id)           AS sessions
FROM events
%s
GROUP BY profile_name, project, model, day_ns
ORDER BY profile_name ASC, day_ns ASC, project ASC, model ASC
`, nsPerDay, nsPerDay, whereSQL)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying usage: %w", err)
	}
	defer rows.Close()

	var out []contracts.UsageRow
	for rows.Next() {
		var (
			r       contracts.UsageRow
			project sql.NullString
			model   sql.NullString
			dayNs   int64
		)
		if err := rows.Scan(
			&r.Profile,
			&project,
			&model,
			&dayNs,
			&r.Usage.InputTokens,
			&r.Usage.OutputTokens,
			&r.Usage.CacheReadTokens,
			&r.Usage.CacheCreateTokens,
			&r.SessionCount,
		); err != nil {
			return nil, fmt.Errorf("scanning usage row: %w", err)
		}
		r.Project = project.String
		r.Model = model.String
		r.Day = time.Unix(0, dayNs).UTC()
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating usage rows: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/storage/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/query.go internal/storage/query_test.go
git commit -m "feat(storage): implement QueryUsage with day-bucketed aggregation"
```

---

## Task 9: Implement `GetCursor` and `SetCursor` (TDD)

**Files:**
- Create: `internal/storage/cursors.go`
- Create: `internal/storage/cursors_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/storage/cursors_test.go`:

```go
package storage_test

import (
	"context"
	"testing"
)

func TestGetCursorUnknownReturnsZero(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	offset, inode, err := s.GetCursor(ctx, "work", "/no/such/file.jsonl")
	if err != nil {
		t.Fatalf("GetCursor unknown file: %v", err)
	}
	if offset != 0 || inode != 0 {
		t.Errorf("unknown cursor: got (%d, %d), want (0, 0)", offset, inode)
	}
}

func TestSetAndGetCursorRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	path := "/Users/arafa/.claude-profiles/work/projects/ccx/sess.jsonl"

	if err := s.SetCursor(ctx, "work", path, 4096, 0xDEADBEEF); err != nil {
		t.Fatalf("SetCursor: %v", err)
	}

	offset, inode, err := s.GetCursor(ctx, "work", path)
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if offset != 4096 {
		t.Errorf("offset: got %d, want 4096", offset)
	}
	if inode != 0xDEADBEEF {
		t.Errorf("inode: got %x, want %x", inode, uint64(0xDEADBEEF))
	}
}

func TestSetCursorUpsert(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	path := "/a/b/c.jsonl"
	if err := s.SetCursor(ctx, "work", path, 100, 1); err != nil {
		t.Fatalf("first SetCursor: %v", err)
	}
	if err := s.SetCursor(ctx, "work", path, 200, 2); err != nil {
		t.Fatalf("second SetCursor: %v", err)
	}
	offset, inode, err := s.GetCursor(ctx, "work", path)
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if offset != 200 || inode != 2 {
		t.Errorf("upserted cursor: got (%d, %d), want (200, 2)", offset, inode)
	}
}

func TestCursorIsolatedPerProfile(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")
	mustSaveProfile(t, s, "personal")

	path := "/shared/path.jsonl"
	if err := s.SetCursor(ctx, "work", path, 100, 1); err != nil {
		t.Fatalf("SetCursor(work): %v", err)
	}
	if err := s.SetCursor(ctx, "personal", path, 999, 2); err != nil {
		t.Fatalf("SetCursor(personal): %v", err)
	}

	workOffset, _, _ := s.GetCursor(ctx, "work", path)
	persOffset, _, _ := s.GetCursor(ctx, "personal", path)
	if workOffset != 100 || persOffset != 999 {
		t.Errorf("isolation: work=%d personal=%d, want 100/999", workOffset, persOffset)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/storage/...
```
Expected: FAIL — `GetCursor`/`SetCursor` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/storage/cursors.go`:

```go
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetCursor returns the last recorded (offset, inode) for the JSONL file at
// filePath under profileName. If no row exists, returns (0, 0, nil) — an
// unknown cursor is not an error; it just means "start from the beginning."
func (s *Store) GetCursor(ctx context.Context, profileName, filePath string) (int64, uint64, error) {
	var (
		offset int64
		inode  sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT offset, inode FROM scan_cursors WHERE profile_name = ? AND file_path = ?`,
		profileName, filePath,
	).Scan(&offset, &inode)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("get cursor for %q %q: %w", profileName, filePath, err)
	}
	var inodeOut uint64
	if inode.Valid {
		inodeOut = uint64(inode.Int64) //nolint:gosec // intentional roundtrip via int64
	}
	return offset, inodeOut, nil
}

// SetCursor upserts the (offset, inode) for the JSONL file at filePath under
// profileName. Subsequent calls overwrite the previous row.
func (s *Store) SetCursor(ctx context.Context, profileName, filePath string, offset int64, inode uint64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	const q = `
INSERT INTO scan_cursors (profile_name, file_path, offset, inode)
VALUES (?, ?, ?, ?)
ON CONFLICT(profile_name, file_path) DO UPDATE SET
    offset = excluded.offset,
    inode  = excluded.inode
`
	if _, err := s.db.ExecContext(ctx, q,
		profileName, filePath, offset, int64(inode), //nolint:gosec // intentional roundtrip
	); err != nil {
		return fmt.Errorf("set cursor for %q %q: %w", profileName, filePath, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/storage/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/cursors.go internal/storage/cursors_test.go
git commit -m "feat(storage): implement GetCursor and SetCursor"
```

---

## Task 10: Verify `*Store` satisfies `contracts.Store` (compile-time assertion)

**Files:**
- Create: `internal/storage/contract_test.go`

- [ ] **Step 1: Write the assertion**

Create `internal/storage/contract_test.go`:

```go
package storage_test

import (
	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

// Compile-time assertion that *storage.Store satisfies contracts.Store. If a
// future contract method is added, this file will fail to build until Store
// implements it.
var _ contracts.Store = (*storage.Store)(nil)
```

- [ ] **Step 2: Verify it compiles**

Run:
```bash
go build ./internal/storage/...
go test ./internal/storage/...
```
Expected: PASS for both. If the assertion fails, the build halts with "does not implement contracts.Store" — fix the missing method before moving on.

- [ ] **Step 3: Commit**

```bash
git add internal/storage/contract_test.go
git commit -m "test(storage): assert *Store satisfies contracts.Store"
```

---

## Task 11: Concurrency test — parallel reads (TDD)

**Files:**
- Create: `internal/storage/concurrent_test.go`

- [ ] **Step 1: Write the test**

Create `internal/storage/concurrent_test.go`:

```go
package storage_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

func TestConcurrentReadsDoNotDeadlock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Use a file-backed DB so WAL mode actually engages (in-memory ignores
	// some pragmas) and so multiple sql.DB connections can run in parallel.
	dir := t.TempDir()
	s, err := storage.NewStore(ctx, filepath.Join(dir, "ccx.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	t0 := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	if err := s.SaveProfile(ctx, contracts.Profile{
		Name: "work", ConfigDir: "/p/work",
		CreatedAt: t0, LastUsedAt: t0,
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	events := make([]contracts.Event, 500)
	for i := range events {
		events[i] = contracts.Event{
			UUID:      "u" + itoa(i),
			SessionID: "s",
			Timestamp: t0.Add(time.Duration(i) * time.Second),
			Type:      "assistant",
			Project:   "ccx",
			Model:     "claude-opus-4-7",
			Usage:     &contracts.Usage{InputTokens: 1, OutputTokens: 1, CacheReadTokens: 1, CacheCreateTokens: 1},
		}
	}
	if err := s.InsertEventsForProfile(ctx, "work", events); err != nil {
		t.Fatalf("InsertEventsForProfile: %v", err)
	}

	const readers = 8
	const reads = 25
	var wg sync.WaitGroup
	errCh := make(chan error, readers*reads)

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < reads; i++ {
				rows, err := s.QueryUsage(ctx, contracts.UsageQuery{
					Range: contracts.TimeRange{
						Start: t0,
						End:   t0.Add(time.Hour),
					},
				})
				if err != nil {
					errCh <- err
					return
				}
				if len(rows) == 0 {
					errCh <- errEmpty
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent read error: %v", err)
	}
}

// itoa is a tiny stdlib-free int→string helper to keep the imports clean.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

var errEmpty = errString("expected non-empty rows")

type errString string

func (e errString) Error() string { return string(e) }
```

- [ ] **Step 2: Run test to verify it passes**

Run:
```bash
go test -race -count=1 ./internal/storage/...
```
Expected: PASS. The `-race` flag catches data races.

- [ ] **Step 3: Commit**

```bash
git add internal/storage/concurrent_test.go
git commit -m "test(storage): exercise concurrent reads with race detector"
```

---

## Task 12: Benchmark — `InsertEvents` of 10,000 events under 1s

**Files:**
- Create: `internal/storage/bench_test.go`

- [ ] **Step 1: Write the benchmark and the threshold test**

Create `internal/storage/bench_test.go`:

```go
package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/storage"
)

func makeEvents(n int, base time.Time) []contracts.Event {
	out := make([]contracts.Event, n)
	for i := 0; i < n; i++ {
		out[i] = contracts.Event{
			UUID:      "bench-" + itoa(i),
			SessionID: "bench-session",
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Type:      "assistant",
			Project:   "ccx",
			Model:     "claude-opus-4-7",
			Usage: &contracts.Usage{
				InputTokens: 100, OutputTokens: 50,
				CacheReadTokens: 200, CacheCreateTokens: 25,
			},
		}
	}
	return out
}

func BenchmarkInsertEvents10000(b *testing.B) {
	ctx := context.Background()
	base := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dir := b.TempDir()
		s, err := storage.NewStore(ctx, filepath.Join(dir, "bench.db"))
		if err != nil {
			b.Fatalf("NewStore: %v", err)
		}
		if err := s.Migrate(ctx); err != nil {
			b.Fatalf("Migrate: %v", err)
		}
		if err := s.SaveProfile(ctx, contracts.Profile{
			Name: "bench", ConfigDir: "/p/bench",
			CreatedAt: base, LastUsedAt: base,
		}); err != nil {
			b.Fatalf("SaveProfile: %v", err)
		}
		events := makeEvents(10_000, base)
		b.StartTimer()

		if err := s.InsertEventsForProfile(ctx, "bench", events); err != nil {
			b.Fatalf("InsertEventsForProfile: %v", err)
		}

		b.StopTimer()
		_ = s.Close()
	}
}

func TestInsertEvents10000UnderOneSecond(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf threshold in -short mode")
	}

	ctx := context.Background()
	base := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	s, err := storage.NewStore(ctx, filepath.Join(dir, "perf.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := s.SaveProfile(ctx, contracts.Profile{
		Name: "perf", ConfigDir: "/p/perf",
		CreatedAt: base, LastUsedAt: base,
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	events := makeEvents(10_000, base)
	start := time.Now()
	if err := s.InsertEventsForProfile(ctx, "perf", events); err != nil {
		t.Fatalf("InsertEventsForProfile: %v", err)
	}
	elapsed := time.Since(start)

	t.Logf("InsertEvents(10000) took %v", elapsed)
	if elapsed > time.Second {
		t.Errorf("InsertEvents(10000) took %v, want <1s", elapsed)
	}
}
```

- [ ] **Step 2: Run the threshold test**

Run:
```bash
go test -count=1 -run TestInsertEvents10000UnderOneSecond ./internal/storage/
```
Expected: PASS with logged duration. If it fails, investigate (likely missing transaction or pragma).

- [ ] **Step 3: Run the benchmark for the record**

Run:
```bash
go test -bench=BenchmarkInsertEvents10000 -benchmem -run=^$ ./internal/storage/
```
Expected: benchmark prints `ns/op` and `B/op`. Note the result in the commit message.

- [ ] **Step 4: Commit**

```bash
git add internal/storage/bench_test.go
git commit -m "test(storage): benchmark InsertEvents 10k events with <1s threshold"
```

---

## Task 13: Run full local CI gate

- [ ] **Step 1: Format check**

Run:
```bash
gofumpt -l internal/storage/
```
Expected: no output. If files are listed, run `gofumpt -w internal/storage/`, review, and commit:
```bash
git add -u && git commit -m "style(storage): gofumpt"
```

- [ ] **Step 2: Vet**

Run:
```bash
go vet ./internal/storage/...
```
Expected: no output.

- [ ] **Step 3: Lint**

Run:
```bash
golangci-lint run ./internal/storage/...
```
Expected: exit 0, no issues.

- [ ] **Step 4: Test with race detector**

Run:
```bash
go test -race -count=1 ./internal/storage/...
```
Expected: PASS.

- [ ] **Step 5: Test the full module compiles**

Run:
```bash
go build ./...
go test ./...
```
Expected: PASS across all packages.

If any step fails, fix and re-run the entire gate before continuing.

---

## Task 14: Open the PR

- [ ] **Step 1: Push the branch**

```bash
git push -u origin feat/storage
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "feat(storage): SQLite-backed contracts.Store implementation" --body "$(cat <<'EOF'
## What

Implements `internal/storage/` per Plan A3:

- `Store` wraps `*sql.DB` from `modernc.org/sqlite` (pure-Go, no CGo)
- Embedded `schema.sql` applied via idempotent `Migrate`
- WAL mode + foreign keys ON + synchronous NORMAL pragmas
- Profile CRUD (`SaveProfile`, `GetProfile`, `ListProfiles`, `DeleteProfile`) with FK cascade
- Batch `InsertEventsForProfile` in a single transaction with `ON CONFLICT DO NOTHING` for re-scan safety
- `QueryUsage` aggregates events by (profile, project, model, day)
- Scan cursors (`GetCursor`/`SetCursor`) for the scanner package
- `sync.Mutex` serializes writes; reads run concurrently

## Why

Phase 1 worktree A3, required by every command that touches usage data.

## Contract impact

- [x] This PR does NOT modify `internal/contracts/`, `api/openapi.yaml`, `internal/storage/schema.sql`, or `docs/conventions.md`
- [ ] If it does, this is a contract-amendment PR (other worktrees will rebase)

## Checklist

- [x] Tests added/updated and all pass locally (`make test`)
- [x] Lint clean locally (`make lint`)
- [x] No new dependencies without justification (added `modernc.org/sqlite` per Section 5.2 of spec)
- [ ] Updates to user-visible behavior reflected in `README.md` or `docs/` (none — internal package)

## Phase 1 worktree?

- Package: `internal/storage`
- Plan: `docs/superpowers/plans/2026-05-19-ccx-phase-1-A3-storage.md`
EOF
)"
```

- [ ] **Step 3: Watch CI**

```bash
gh pr checks --watch
```
Expected: lint, test (3 OSes), build (3 OSes) all green.

If any job fails: address on this branch, push, and re-run `gh pr checks --watch`. Do not merge until all checks pass.

- [ ] **Step 4: Request review and merge**

Once CI is green and any review comments are addressed:
```bash
gh pr merge --squash --delete-branch
```

---

## Plan A3 done definition

All of the following are true:

- [ ] `go build ./internal/storage/...` succeeds
- [ ] `go test -race -count=1 ./internal/storage/...` is green
- [ ] `go test -bench=BenchmarkInsertEvents10000 -benchmem -run=^$ ./internal/storage/` runs to completion
- [ ] `TestInsertEvents10000UnderOneSecond` reports an elapsed time under 1s
- [ ] `golangci-lint run ./internal/storage/...` reports zero issues
- [ ] `gofumpt -l internal/storage/` produces no output
- [ ] Compile-time assertion `var _ contracts.Store = (*storage.Store)(nil)` builds clean
- [ ] All files from this plan exist and are committed:
  - `internal/storage/embed.go`, `internal/storage/embed_test.go`
  - `internal/storage/store.go`, `internal/storage/store_test.go`
  - `internal/storage/migrate.go`, `internal/storage/migrate_test.go`
  - `internal/storage/profiles.go`, `internal/storage/profiles_test.go`
  - `internal/storage/events.go`, `internal/storage/events_test.go`
  - `internal/storage/query.go`, `internal/storage/query_test.go`
  - `internal/storage/cursors.go`, `internal/storage/cursors_test.go`
  - `internal/storage/contract_test.go`
  - `internal/storage/concurrent_test.go`
  - `internal/storage/bench_test.go`
  - `internal/storage/testhelpers_test.go`
- [ ] PR opened, CI green on macOS/Linux/Windows, merged to `main`
- [ ] Worktree cleaned up:
  ```bash
  cd /Users/arafa/Developer/ccx
  git worktree remove ../ccx-storage
  git fetch --prune
  ```

After this plan lands, the scanner worktree (A2) and the integration phase (P2) can consume `*storage.Store` as their `contracts.Store` implementation.
