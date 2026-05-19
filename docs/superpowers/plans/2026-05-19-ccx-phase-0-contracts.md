# ccx Phase 0 — Contracts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lock down all cross-package contracts (types, interfaces, schema, openapi, conventions, CI) so that nine Phase 1 worktrees can subsequently work in parallel without colliding.

**Architecture:** All shared types and interfaces live in `internal/contracts/`. Every Phase 1 package imports from `contracts/` only — never from sibling packages. The HTTP API contract lives in `api/openapi.yaml` (allows the frontend to mock the API independently of the backend). Conventions (error wrapping, exit codes, lint config) are documented in `docs/conventions.md`.

**Tech Stack:** Go 1.22+, golangci-lint, gofumpt, lefthook, GitHub Actions.

**Spec reference:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — Section 11.1 (Phase 0).

**Worktree:** This phase runs on `main`. No parallel work happens during Phase 0.

**Exit criteria:**
- `go build ./...` succeeds
- `go test ./...` succeeds (only contract tests at this point)
- `golangci-lint run` succeeds
- A CI run on a PR shows green
- All Phase 1 worktrees can be created with `git worktree add ../ccx-<name> -b feat/<name>` and start work immediately

---

## Pre-flight

Confirm the working directory is `/Users/arafa/Developer/ccx` and is a git repo initialized on branch `main` with at least one commit (the spec commit).

```bash
pwd                         # → /Users/arafa/Developer/ccx
git status                  # → On branch main, working tree clean
git log --oneline | head    # → at least one commit (the spec)
```

If `git` is not initialized: `git init -b main`. If no commits: that's fine, Task 1 makes the first.

**Conventions for this plan:**
- All Go code uses tabs for indentation (gofumpt enforced)
- All commit messages follow `type(scope): subject` — e.g., `feat(contracts): add Profile type`
- Every task ends with a commit; do not batch
- Run `go test ./...` and `golangci-lint run` before every commit

---

## Task 1: Initialize Go module

**Files:**
- Create: `go.mod`
- Create: `.gitignore`

- [ ] **Step 1: Create `.gitignore`**

```
# Binaries
/ccx
/cmd/ccx/ccx
/dist/

# Test artifacts
*.test
*.out
coverage.txt
coverage.html

# OS junk
.DS_Store
Thumbs.db

# IDE
.idea/
.vscode/
*.swp

# Web build output (committed via go:embed only when building releases)
web/node_modules/
web/.next/
web/out/

# Local state
.env
.env.local
*.local

# Worktrees parent dir if user keeps them under repo
ccx-*/
```

- [ ] **Step 2: Initialize Go module**

Run:
```bash
go mod init github.com/arafa-dev/ccx
go mod edit -go=1.22
```

Verify `go.mod` looks like:
```
module github.com/arafa-dev/ccx

go 1.22
```

- [ ] **Step 3: Commit**

```bash
git add go.mod .gitignore
git commit -m "chore: initialize go module"
```

---

## Task 2: Create Makefile skeleton

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Write `Makefile`**

```make
.PHONY: help build test lint fmt clean web dev release ci all

# Default goal
.DEFAULT_GOAL := help

# Variables
BINARY      := ccx
GO_PACKAGES := ./...
LDFLAGS     := -s -w -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build the ccx binary
	@mkdir -p dist
	go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY) ./cmd/ccx

test: ## Run all Go tests
	go test -race -count=1 $(GO_PACKAGES)

lint: ## Run linters
	golangci-lint run

fmt: ## Format all Go code
	gofumpt -w .

clean: ## Remove build artifacts
	rm -rf dist web/out web/.next

web: ## Build the Next.js dashboard (Phase 1 A7)
	@echo "web build not yet wired — see Phase 1 plan A7"

dev: ## Run dev mode (CLI + dashboard) — Phase 2
	@echo "dev mode not yet wired — see Phase 2 plan"

release: ## Run goreleaser locally (Phase 1 A8)
	@echo "release not yet wired — see Phase 1 plan A8"

ci: lint test ## Run the full CI gate locally

all: clean fmt lint test build ## Full local pipeline
```

- [ ] **Step 2: Verify Makefile is sane**

Run:
```bash
make help
```
Expected: prints the help listing with all targets.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile skeleton"
```

---

## Task 3: Define `Profile` type (TDD)

**Files:**
- Create: `internal/contracts/types.go`
- Test: `internal/contracts/types_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/contracts/types_test.go`:

```go
package contracts_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestProfileJSONRoundtrip(t *testing.T) {
	in := contracts.Profile{
		Name:       "work",
		ConfigDir:  "/Users/arafa/.claude-profiles/work",
		Label:      "Work account",
		Color:      "#3B82F6",
		CreatedAt:  time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		LastUsedAt: time.Date(2026, 5, 19, 15, 30, 0, 0, time.UTC),
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out contracts.Profile
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out != in {
		t.Errorf("roundtrip mismatch:\n got  %+v\n want %+v", out, in)
	}
}

func TestProfileZeroValueIsUsable(t *testing.T) {
	var p contracts.Profile
	if p.Name != "" {
		t.Errorf("zero Profile.Name should be empty, got %q", p.Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/contracts/...
```
Expected: FAIL because `internal/contracts` package does not exist yet.

- [ ] **Step 3: Write minimal implementation**

Create `internal/contracts/types.go`:

```go
// Package contracts defines the shared types and interfaces used across all
// ccx internal packages. Every other internal/* package imports from this
// package only — never from sibling packages. This isolation is what allows
// Phase 1 development to run in parallel git worktrees.
package contracts

import "time"

// Profile identifies a Claude Code account by its config directory. The
// ConfigDir field is the only thing that determines identity — setting
// CLAUDE_CONFIG_DIR to this value is what isolates the account.
type Profile struct {
	Name       string    `json:"name"        toml:"name"`
	ConfigDir  string    `json:"config_dir"  toml:"config_dir"`
	Label      string    `json:"label"       toml:"label,omitempty"`
	Color      string    `json:"color"       toml:"color,omitempty"`
	CreatedAt  time.Time `json:"created_at"  toml:"created_at"`
	LastUsedAt time.Time `json:"last_used_at" toml:"last_used_at"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/contracts/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/contracts/types.go internal/contracts/types_test.go
git commit -m "feat(contracts): add Profile type"
```

---

## Task 4: Define `Usage` and `Event` types (TDD)

**Files:**
- Modify: `internal/contracts/types.go`
- Modify: `internal/contracts/types_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/contracts/types_test.go`:

```go
func TestUsageAdd(t *testing.T) {
	a := contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 25}
	b := contracts.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 20, CacheCreateTokens: 1}

	got := a.Add(b)
	want := contracts.Usage{InputTokens: 110, OutputTokens: 55, CacheReadTokens: 220, CacheCreateTokens: 26}

	if got != want {
		t.Errorf("Add mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestUsageTotalTokens(t *testing.T) {
	u := contracts.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 200, CacheCreateTokens: 25}
	if got, want := u.TotalTokens(), 375; got != want {
		t.Errorf("TotalTokens: got %d want %d", got, want)
	}
}

func TestEventJSONRoundtrip(t *testing.T) {
	in := contracts.Event{
		UUID:      "01H7Z8...",
		SessionID: "sess-abc",
		Timestamp: time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Type:      "assistant",
		Project:   "ccx",
		Model:     "claude-opus-4-7",
		Usage: &contracts.Usage{
			InputTokens:       1000,
			OutputTokens:      200,
			CacheReadTokens:   5000,
			CacheCreateTokens: 100,
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out contracts.Event
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.UUID != in.UUID || out.Type != in.Type || out.Usage == nil || *out.Usage != *in.Usage {
		t.Errorf("roundtrip mismatch:\n got  %+v\n want %+v", out, in)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/contracts/...
```
Expected: FAIL — `Usage`, `Event`, `Add`, `TotalTokens` undefined.

- [ ] **Step 3: Add types to `types.go`**

Append to `internal/contracts/types.go`:

```go
// Usage holds the token counts for a single Claude Code event. All fields are
// non-negative. Token counts come from the upstream JSONL `message.usage` block.
type Usage struct {
	InputTokens       int `json:"input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	CacheReadTokens   int `json:"cache_read_tokens"`
	CacheCreateTokens int `json:"cache_create_tokens"`
}

// Add returns the element-wise sum of u and v.
func (u Usage) Add(v Usage) Usage {
	return Usage{
		InputTokens:       u.InputTokens + v.InputTokens,
		OutputTokens:      u.OutputTokens + v.OutputTokens,
		CacheReadTokens:   u.CacheReadTokens + v.CacheReadTokens,
		CacheCreateTokens: u.CacheCreateTokens + v.CacheCreateTokens,
	}
}

// TotalTokens returns the sum of all four token counts. Useful for one-number
// usage displays, but cost calculations should use the per-bucket fields
// because each bucket has a different rate.
func (u Usage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheCreateTokens
}

// Event is a single parsed JSONL line from a Claude Code session file.
// The Usage field is non-nil only for assistant events that carry token counts.
type Event struct {
	UUID      string    `json:"uuid"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Project   string    `json:"project"`
	Model     string    `json:"model,omitempty"`
	Usage     *Usage    `json:"usage,omitempty"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/contracts/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/contracts/types.go internal/contracts/types_test.go
git commit -m "feat(contracts): add Usage and Event types"
```

---

## Task 5: Define `TimeRange`, `UsageQuery`, `UsageRow` (TDD)

**Files:**
- Modify: `internal/contracts/types.go`
- Modify: `internal/contracts/types_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/contracts/types_test.go`:

```go
func TestTimeRangeContains(t *testing.T) {
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)
	tr := contracts.TimeRange{Start: start, End: end}

	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"before", time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC), false},
		{"at start", start, true},
		{"middle", time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC), true},
		{"at end", end, true},
		{"after", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tr.Contains(tc.t); got != tc.want {
				t.Errorf("Contains(%v) = %v want %v", tc.t, got, tc.want)
			}
		})
	}
}

func TestUsageQueryDefaults(t *testing.T) {
	q := contracts.UsageQuery{}
	if q.Profile != "" {
		t.Errorf("default Profile should be empty (means all), got %q", q.Profile)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/contracts/...
```
Expected: FAIL — types undefined.

- [ ] **Step 3: Add types**

Append to `internal/contracts/types.go`:

```go
// TimeRange is a closed interval [Start, End] used for usage queries.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// Contains reports whether t falls within the closed interval [Start, End].
func (r TimeRange) Contains(t time.Time) bool {
	return !t.Before(r.Start) && !t.After(r.End)
}

// UsageQuery filters and groups events for the Store.QueryUsage method.
// An empty Profile means "all profiles." An empty Project means "all projects."
type UsageQuery struct {
	Profile string
	Project string
	Range   TimeRange
}

// UsageRow is one aggregated row returned by Store.QueryUsage. Aggregation
// granularity (per-profile, per-day, per-project) is determined by the
// concrete Store implementation.
type UsageRow struct {
	Profile      string
	Project      string
	Model        string
	Day          time.Time // truncated to start of day in UTC
	Usage        Usage
	SessionCount int
	EstimatedUSD float64 // populated by the caller after pricing lookup
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/contracts/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/contracts/types.go internal/contracts/types_test.go
git commit -m "feat(contracts): add TimeRange, UsageQuery, UsageRow"
```

---

## Task 6: Define `Shell` enum and helpers (TDD)

**Files:**
- Modify: `internal/contracts/types.go`
- Modify: `internal/contracts/types_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/contracts/types_test.go`:

```go
func TestParseShell(t *testing.T) {
	tests := []struct {
		in   string
		want contracts.Shell
		ok   bool
	}{
		{"zsh", contracts.ShellZsh, true},
		{"bash", contracts.ShellBash, true},
		{"fish", contracts.ShellFish, true},
		{"pwsh", contracts.ShellPowerShell, true},
		{"powershell", contracts.ShellPowerShell, true},
		{"unknown", contracts.ShellUnknown, false},
		{"", contracts.ShellUnknown, false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := contracts.ParseShell(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Errorf("ParseShell(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestShellString(t *testing.T) {
	if got, want := contracts.ShellZsh.String(), "zsh"; got != want {
		t.Errorf("ShellZsh.String() = %q want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/contracts/...
```
Expected: FAIL.

- [ ] **Step 3: Add Shell enum**

Append to `internal/contracts/types.go`:

```go
// Shell identifies a shell flavor for the purpose of emitting init scripts
// and `ccx use` shell-eval output.
type Shell int

const (
	ShellUnknown Shell = iota
	ShellZsh
	ShellBash
	ShellFish
	ShellPowerShell
)

// String returns the canonical name of the shell.
func (s Shell) String() string {
	switch s {
	case ShellZsh:
		return "zsh"
	case ShellBash:
		return "bash"
	case ShellFish:
		return "fish"
	case ShellPowerShell:
		return "pwsh"
	default:
		return "unknown"
	}
}

// ParseShell parses a shell name. Accepts "zsh", "bash", "fish", "pwsh",
// "powershell". Returns (ShellUnknown, false) for unknown input.
func ParseShell(s string) (Shell, bool) {
	switch s {
	case "zsh":
		return ShellZsh, true
	case "bash":
		return ShellBash, true
	case "fish":
		return ShellFish, true
	case "pwsh", "powershell":
		return ShellPowerShell, true
	default:
		return ShellUnknown, false
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/contracts/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/contracts/types.go internal/contracts/types_test.go
git commit -m "feat(contracts): add Shell enum"
```

---

## Task 7: Define sentinel errors (TDD)

**Files:**
- Create: `internal/contracts/errors.go`
- Create: `internal/contracts/errors_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/contracts/errors_test.go`:

```go
package contracts_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestSentinelErrorsAreDistinguishable(t *testing.T) {
	wrapped := fmt.Errorf("looking up profile %q: %w", "work", contracts.ErrProfileNotFound)

	if !errors.Is(wrapped, contracts.ErrProfileNotFound) {
		t.Errorf("errors.Is should match wrapped ErrProfileNotFound")
	}
	if errors.Is(wrapped, contracts.ErrInvalidConfigDir) {
		t.Errorf("errors.Is should NOT match a different sentinel")
	}
}

func TestEveryDefinedSentinelHasMessage(t *testing.T) {
	for _, err := range []error{
		contracts.ErrProfileNotFound,
		contracts.ErrInvalidConfigDir,
		contracts.ErrProfileAlreadyExists,
		contracts.ErrConfigDirConflict,
		contracts.ErrUnknownShell,
		contracts.ErrNoActiveProfile,
	} {
		if err.Error() == "" {
			t.Errorf("sentinel %T has empty message", err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/contracts/...
```
Expected: FAIL — sentinels undefined.

- [ ] **Step 3: Add `errors.go`**

Create `internal/contracts/errors.go`:

```go
package contracts

import "errors"

// Sentinel errors. All ccx packages return one of these (wrapped with %w
// for context) for known, expected failure modes. Tests and callers use
// errors.Is to detect them.
var (
	// ErrProfileNotFound is returned when the requested profile name has no
	// entry in the registry.
	ErrProfileNotFound = errors.New("profile not found")

	// ErrProfileAlreadyExists is returned by profile-add when a profile with
	// the requested name is already registered.
	ErrProfileAlreadyExists = errors.New("profile already exists")

	// ErrInvalidConfigDir is returned when a path is not a valid Claude Code
	// config directory (e.g., not a directory, or unreadable).
	ErrInvalidConfigDir = errors.New("invalid config directory")

	// ErrConfigDirConflict is returned when two profiles would point at the
	// same config directory.
	ErrConfigDirConflict = errors.New("config directory already used by another profile")

	// ErrUnknownShell is returned when a shell name is not recognized by the
	// shell package (see ParseShell).
	ErrUnknownShell = errors.New("unknown shell")

	// ErrNoActiveProfile is returned when an operation requires an active
	// profile but neither CCX_ACTIVE_PROFILE nor CLAUDE_CONFIG_DIR is set.
	ErrNoActiveProfile = errors.New("no active profile")
)
```

- [ ] **Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/contracts/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/contracts/errors.go internal/contracts/errors_test.go
git commit -m "feat(contracts): add sentinel errors"
```

---

## Task 8: Define core interfaces

**Files:**
- Create: `internal/contracts/interfaces.go`

Interfaces have no behavior, so there are no behavior tests at this level. Phase 1 packages will test their implementations.

- [ ] **Step 1: Write the interfaces file**

Create `internal/contracts/interfaces.go`:

```go
package contracts

import (
	"context"
	"time"
)

// Scanner walks a profile's JSONL files and emits parsed Events. Implementations
// are expected to use the Store's scan-cursor API (via the host package wiring)
// to avoid re-reading already-consumed bytes.
type Scanner interface {
	// Scan walks all JSONL files under profile.ConfigDir/projects/ and emits
	// Events on the returned channel. The channel is closed when scanning
	// completes or when ctx is cancelled. Errors are logged but do not abort
	// the scan; truly fatal errors return early via the error channel.
	Scan(ctx context.Context, profile Profile) (<-chan Event, <-chan error)
}

// Store persists profiles and events. SQLite is the v0.1 implementation, but
// the interface is deliberately storage-agnostic.
type Store interface {
	// Profile CRUD
	SaveProfile(ctx context.Context, p Profile) error
	GetProfile(ctx context.Context, name string) (Profile, error)
	ListProfiles(ctx context.Context) ([]Profile, error)
	DeleteProfile(ctx context.Context, name string) error

	// Event ingestion
	InsertEvents(ctx context.Context, events []Event) error

	// Usage queries
	QueryUsage(ctx context.Context, q UsageQuery) ([]UsageRow, error)

	// Scan cursors (for incremental scanning)
	GetCursor(ctx context.Context, profileName, filePath string) (offset int64, inode uint64, err error)
	SetCursor(ctx context.Context, profileName, filePath string, offset int64, inode uint64) error

	// Lifecycle
	Migrate(ctx context.Context) error
	Close() error
}

// PricingTable returns estimated USD cost for a given model + usage at a given
// timestamp. Implementations consult an embedded pricing YAML; users may
// override via ~/.ccx/pricing.yaml.
type PricingTable interface {
	Cost(model string, ts time.Time, usage Usage) (float64, error)
	LastUpdated() time.Time
}

// ShellEmitter generates shell-specific snippets for `ccx use` and `ccx init`.
type ShellEmitter interface {
	// EmitUseScript returns the script that, when eval'd by the user's shell,
	// activates the given profile. The script sets CLAUDE_CONFIG_DIR and
	// CCX_ACTIVE_PROFILE.
	EmitUseScript(profile Profile, shell Shell) (string, error)

	// EmitInitScript returns the rc-file snippet the user pastes into their
	// shell config once. The snippet defines a wrapper function so `ccx use foo`
	// works without `eval`.
	EmitInitScript(shell Shell) (string, error)
}

// Doctor runs diagnostic checks and reports them as a structured slice.
type Doctor interface {
	Run(ctx context.Context) ([]DoctorCheck, error)
}

// DoctorCheck is one diagnostic finding. Status is "ok", "warn", or "fail".
type DoctorCheck struct {
	Name        string
	Status      string
	Detail      string
	Remediation string
}
```

- [ ] **Step 2: Verify it compiles**

Run:
```bash
go build ./...
go vet ./...
```
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/contracts/interfaces.go
git commit -m "feat(contracts): add Scanner, Store, PricingTable, ShellEmitter, Doctor interfaces"
```

---

## Task 9: Write SQLite schema

**Files:**
- Create: `internal/storage/schema.sql`
- Create: `internal/storage/doc.go`

- [ ] **Step 1: Write the schema**

Create `internal/storage/schema.sql`:

```sql
-- ccx v0.1 SQLite schema.
-- This file is the source of truth. Migrations in internal/storage are derived
-- from this file. Do not edit this file from inside Phase 1 worktrees — open
-- a contract-amendment PR against main instead.

CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS profiles (
    name           TEXT PRIMARY KEY,
    config_dir     TEXT UNIQUE NOT NULL,
    label          TEXT,
    color          TEXT,
    created_at     INTEGER NOT NULL,
    last_used_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    profile_name        TEXT NOT NULL,
    session_id          TEXT NOT NULL,
    event_uuid          TEXT NOT NULL,
    ts                  INTEGER NOT NULL,
    project             TEXT,
    model               TEXT,
    input_tokens        INTEGER NOT NULL DEFAULT 0,
    output_tokens       INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens   INTEGER NOT NULL DEFAULT 0,
    cache_create_tokens INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (profile_name, event_uuid),
    FOREIGN KEY (profile_name) REFERENCES profiles(name) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS events_profile_ts ON events(profile_name, ts);
CREATE INDEX IF NOT EXISTS events_project    ON events(project);

CREATE TABLE IF NOT EXISTS scan_cursors (
    profile_name TEXT NOT NULL,
    file_path    TEXT NOT NULL,
    offset       INTEGER NOT NULL DEFAULT 0,
    inode        INTEGER,
    PRIMARY KEY (profile_name, file_path),
    FOREIGN KEY (profile_name) REFERENCES profiles(name) ON DELETE CASCADE
);

-- Seed the schema version
INSERT OR IGNORE INTO schema_version (version) VALUES (1);
```

- [ ] **Step 2: Create the package doc.go**

Create `internal/storage/doc.go`:

```go
// Package storage provides a SQLite-backed implementation of contracts.Store.
//
// The schema is defined in schema.sql and embedded at build time. Migrations
// (when added in a later release) live alongside this file and are run by
// (*Store).Migrate.
package storage
```

- [ ] **Step 3: Verify build**

Run:
```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/storage/schema.sql internal/storage/doc.go
git commit -m "feat(storage): add SQLite schema and package stub"
```

---

## Task 10: Write OpenAPI contract

**Files:**
- Create: `api/openapi.yaml`

- [ ] **Step 1: Write the OpenAPI spec**

Create `api/openapi.yaml`:

```yaml
openapi: 3.1.0
info:
  title: ccx local API
  version: 0.1.0
  description: |
    Local HTTP API served by `ccx dashboard`. Always bound to 127.0.0.1.
    No authentication (localhost-only). All endpoints return JSON unless noted.

servers:
  - url: http://127.0.0.1:7777
    description: Default local server

paths:
  /api/health:
    get:
      summary: Health check
      operationId: getHealth
      responses:
        "200":
          description: Server is alive
          content:
            application/json:
              schema:
                type: object
                required: [ok, version]
                properties:
                  ok:      { type: boolean }
                  version: { type: string  }

  /api/profiles:
    get:
      summary: List all registered profiles with today's totals
      operationId: listProfiles
      responses:
        "200":
          description: A list of profiles
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/ProfileWithTotals"

  /api/usage:
    get:
      summary: Aggregated usage rows
      operationId: getUsage
      parameters:
        - in: query
          name: profile
          schema: { type: string }
          description: Filter to one profile. Omit for all.
        - in: query
          name: project
          schema: { type: string }
          description: Filter to one project. Omit for all.
        - in: query
          name: since
          schema: { type: string }
          description: Duration string like "24h", "7d", "30d". Default "24h".
      responses:
        "200":
          description: Aggregated usage rows
          content:
            application/json:
              schema:
                type: object
                required: [rows, total]
                properties:
                  rows:
                    type: array
                    items:
                      $ref: "#/components/schemas/UsageRow"
                  total:
                    $ref: "#/components/schemas/UsageTotal"

  /api/usage/live:
    get:
      summary: Server-Sent Events stream of usage updates
      operationId: getUsageLive
      description: |
        Emits a `usage` event whenever a watched JSONL file changes.
        The data payload is a JSON-encoded UsageRow array.
      responses:
        "200":
          description: SSE stream
          content:
            text/event-stream:
              schema:
                type: string

components:
  schemas:
    Profile:
      type: object
      required: [name, config_dir, created_at, last_used_at]
      properties:
        name:         { type: string }
        config_dir:   { type: string }
        label:        { type: string }
        color:        { type: string }
        created_at:   { type: string, format: date-time }
        last_used_at: { type: string, format: date-time }

    ProfileWithTotals:
      allOf:
        - $ref: "#/components/schemas/Profile"
        - type: object
          required: [today]
          properties:
            today:
              $ref: "#/components/schemas/UsageTotal"

    Usage:
      type: object
      required: [input_tokens, output_tokens, cache_read_tokens, cache_create_tokens]
      properties:
        input_tokens:        { type: integer }
        output_tokens:       { type: integer }
        cache_read_tokens:   { type: integer }
        cache_create_tokens: { type: integer }

    UsageRow:
      type: object
      required: [profile, day, usage, session_count, estimated_usd]
      properties:
        profile:        { type: string }
        project:        { type: string }
        model:          { type: string }
        day:            { type: string, format: date-time }
        usage:          { $ref: "#/components/schemas/Usage" }
        session_count:  { type: integer }
        estimated_usd:  { type: number, format: double }

    UsageTotal:
      type: object
      required: [usage, estimated_usd]
      properties:
        usage:          { $ref: "#/components/schemas/Usage" }
        estimated_usd:  { type: number, format: double }
```

- [ ] **Step 2: Validate the OpenAPI file (optional but recommended)**

If `redocly` or `swagger-cli` is available, run:
```bash
npx --yes @redocly/cli@latest lint api/openapi.yaml
```
If the tool reports errors, fix them. If neither tool is available, skip and trust the schema.

- [ ] **Step 3: Commit**

```bash
git add api/openapi.yaml
git commit -m "feat(api): add OpenAPI 3.1 contract for dashboard server"
```

---

## Task 11: Write `docs/conventions.md`

**Files:**
- Create: `docs/conventions.md`

- [ ] **Step 1: Write the conventions doc**

Create `docs/conventions.md`:

```markdown
# ccx Engineering Conventions

This document is the source of truth for cross-package conventions. Phase 1
worktrees follow these rules without negotiation. Changes require a PR against
main, not a feature-branch edit.

## 1. Go style

- Format with `gofumpt` (stricter than `gofmt`). Pre-commit hook enforces.
- Lint with `golangci-lint`. Config in `.golangci.yml`. CI gates on it.
- Tabs for indentation in Go files. Spaces in YAML, SQL, Markdown.
- All exported types and functions documented with a Go doc comment beginning
  with the name of the symbol.
- Files end with a trailing newline.

## 2. Error handling

- Always wrap with context: `fmt.Errorf("loading profile %q: %w", name, err)`.
- Define a sentinel in `internal/contracts/errors.go` for any error a *caller*
  might want to detect. Use `errors.Is` to detect.
- Do not return raw `errors.New(...)` from a public function for a known case —
  add a sentinel.
- Do not log AND return; one or the other (callers log at the boundary).

## 3. Logging

- Use `log/slog` (Go 1.21+ stdlib). No third-party loggers.
- Default level: INFO. `--verbose` raises to DEBUG.
- Default handler: text on stderr. `--log-format=json` switches to JSON.
- Standard field names: `profile`, `path`, `count`, `duration`, `err`.

## 4. CLI exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | User error (bad input, missing profile, etc.) |
| 2 | Internal error |
| 64 | EX_USAGE: command-line usage error |
| 70 | EX_SOFTWARE: internal software error |
| 73 | EX_CANTCREAT: can't create file |
| 74 | EX_IOERR: input/output error |

Use the higher (sysexits) codes only when they communicate something more
specific than the generic 1/2.

## 5. Context propagation

Every public function that does I/O or could block takes a `context.Context`
as its first parameter, named `ctx`. Callers must respect cancellation.

## 6. Package boundaries

- `internal/contracts/` is the only shared package. Every other `internal/*`
  package imports from `contracts` and from stdlib, plus its own declared
  dependencies. No cross-imports between sibling `internal/*` packages
  during Phase 1.
- Phase 2 wires packages together in `internal/cli/`, `internal/server/`,
  `internal/tui/`, `internal/doctor/`, and `cmd/ccx/`.

## 7. Testing

- Table-driven tests preferred for pure functions.
- One test file per source file: `foo.go` ↔ `foo_test.go`.
- Use `_test` package suffix (`package contracts_test`) for black-box tests.
- Use the same-package suffix (`package contracts`) only when testing
  unexported helpers.
- Use `testdata/` for fixtures. Test files inside `testdata/` are never
  compiled (the dir name is ignored by Go).
- Run with `-race -count=1` in CI (no cached results).

## 8. Commit messages

Conventional commits: `type(scope): subject`.

Common types: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`, `perf`,
`build`, `ci`. Scope is the package name (e.g., `feat(contracts):`,
`fix(storage):`).

One logical change per commit. Do not batch unrelated changes.

## 9. Branch and worktree naming

- `feat/<package>` for Phase 1 packages (e.g., `feat/profile`, `feat/scanner`)
- `chore/<topic>` for non-feature work
- Worktree directories: `../ccx-<package>` (sibling of repo)

## 10. PR requirements

- One package per PR
- All tests pass in the worktree before opening
- PR description includes: "Did this work require any contract change? If yes,
  link the amendment PR."
- At least one CI run green before merge

## 11. Security

- Never log credential contents or `.credentials.json` paths from non-default
  config dirs unless explicitly requested by the user with `--debug`.
- Dashboard HTTP server binds to 127.0.0.1 only. Never 0.0.0.0.
- No outbound network calls from the binary except: (a) update checks (opt-in,
  later release), (b) `go mod download` at build time. The runtime is
  offline-by-default.

## 12. Pricing data

- The embedded `pricing/models.yaml` is the v0.1 baseline.
- All currency displays are labeled "Estimated USD" — Anthropic rates can
  change without notice.
- User overrides via `~/.ccx/pricing.yaml` are respected if present and valid.
```

- [ ] **Step 2: Commit**

```bash
git add docs/conventions.md
git commit -m "docs: add engineering conventions"
```

---

## Task 12: Create golangci-lint config

**Files:**
- Create: `.golangci.yml`

- [ ] **Step 1: Write the lint config**

Create `.golangci.yml`:

```yaml
run:
  timeout: 5m
  tests: true
  go: "1.22"

linters:
  disable-all: true
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - bodyclose
    - errorlint
    - gocritic
    - gofumpt
    - goimports
    - gosec
    - misspell
    - nakedret
    - prealloc
    - revive
    - unconvert
    - unparam
    - whitespace

linters-settings:
  gocritic:
    enabled-tags:
      - diagnostic
      - performance
      - style
  errorlint:
    errorf: true
    asserts: true
    comparison: true
  revive:
    rules:
      - name: exported
        severity: warning
        disabled: false

issues:
  exclude-use-default: false
  exclude-rules:
    # Allow long lines in generated/embedded test fixtures
    - path: testdata/
      linters:
        - lll
    # Tests don't need exhaustive comments
    - path: _test\.go
      linters:
        - revive
        - gocritic
  max-issues-per-linter: 0
  max-same-issues: 0
```

- [ ] **Step 2: Verify it runs**

Install golangci-lint locally if not present:
```bash
which golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

Run:
```bash
golangci-lint run
```
Expected: no errors (a few warnings on the bare contracts package are acceptable, but the run should exit 0).

If any errors surface, address them before continuing.

- [ ] **Step 3: Commit**

```bash
git add .golangci.yml
git commit -m "ci: add golangci-lint config"
```

---

## Task 13: Set up lefthook for pre-commit

**Files:**
- Create: `lefthook.yml`

- [ ] **Step 1: Write the lefthook config**

Create `lefthook.yml`:

```yaml
pre-commit:
  parallel: true
  commands:
    gofumpt:
      glob: "*.go"
      run: |
        if ! command -v gofumpt >/dev/null 2>&1; then
          echo "gofumpt not installed; skip"
          exit 0
        fi
        gofumpt -l -w {staged_files}
        git add {staged_files}
    lint:
      glob: "*.go"
      run: |
        if ! command -v golangci-lint >/dev/null 2>&1; then
          echo "golangci-lint not installed; skip"
          exit 0
        fi
        golangci-lint run --new-from-rev=HEAD~1 ./...
```

- [ ] **Step 2: Install lefthook locally**

```bash
which lefthook || brew install lefthook 2>/dev/null || go install github.com/evilmartians/lefthook@latest
lefthook install
```

- [ ] **Step 3: Verify hooks are registered**

```bash
cat .git/hooks/pre-commit | head -3
```
Expected: a lefthook-generated script.

- [ ] **Step 4: Commit**

```bash
git add lefthook.yml
git commit -m "ci: add lefthook pre-commit hooks"
```

---

## Task 14: Create GitHub Actions CI workflow skeleton

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    name: Lint
    runs-on: ubuntu-22.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true
      - name: Install gofumpt
        run: go install mvdan.cc/gofumpt@latest
      - name: Check formatting
        run: |
          DIFF=$(gofumpt -l .)
          if [ -n "$DIFF" ]; then
            echo "::error::Files not formatted with gofumpt:"
            echo "$DIFF"
            exit 1
          fi
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest
          args: --timeout=5m

  test:
    name: Test (${{ matrix.os }})
    needs: lint
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-22.04, macos-14, windows-2022]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true
      - name: Test
        run: go test -race -count=1 ./...

  build:
    name: Build (${{ matrix.os }})
    needs: test
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-22.04, macos-14, windows-2022]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true
      - name: Build
        # cmd/ccx may not exist yet during Phase 0; tolerate missing main.
        run: |
          if [ -d cmd/ccx ]; then
            go build -trimpath -o /tmp/ccx ./cmd/ccx
          else
            echo "cmd/ccx not present yet; skipping binary build"
          fi
        shell: bash
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add GitHub Actions workflow (lint, test, build)"
```

---

## Task 15: Create empty stub packages for Phase 1

These stubs let `go build ./...` succeed before any Phase 1 work begins. Each stub is a `doc.go` file with a package comment.

**Files (all created in this task):**
- Create: `internal/profile/doc.go`
- Create: `internal/scanner/doc.go`
- Create: `internal/pricing/doc.go`
- Create: `internal/shell/doc.go`
- Create: `internal/platform/doc.go`
- Create: `internal/cli/doc.go`
- Create: `internal/server/doc.go`
- Create: `internal/tui/doc.go`
- Create: `internal/doctor/doc.go`
- Create: `internal/dashboard/doc.go`

Note: we deliberately do NOT create `cmd/ccx/` in Phase 0. A `package main` directory without a `func main` fails `go build ./cmd/ccx`. The real `cmd/ccx/main.go` is added in Phase 2 (integration). Until then, `go build ./...` simply skips a non-existent path.

- [ ] **Step 1: Create `internal/profile/doc.go`**

```go
// Package profile manages the ccx profile registry (~/.ccx/profiles.toml).
// It implements profile CRUD, validation, and active-profile detection.
// See docs/superpowers/plans (plan A1) for the implementation plan.
package profile
```

- [ ] **Step 2: Create `internal/scanner/doc.go`**

```go
// Package scanner walks a profile's JSONL session files and emits parsed
// contracts.Events. It is defensive: unknown event types and malformed lines
// are logged and skipped, never panicked on.
// See docs/superpowers/plans (plan A2) for the implementation plan.
package scanner
```

- [ ] **Step 3: Create `internal/pricing/doc.go`**

```go
// Package pricing loads the embedded model→USD rate table and computes
// estimated cost for a given model + usage + timestamp.
// See docs/superpowers/plans (plan A4) for the implementation plan.
package pricing
```

- [ ] **Step 4: Create `internal/shell/doc.go`**

```go
// Package shell emits shell-specific snippets for `ccx use` (eval-style) and
// `ccx init` (rc-file wrapper). It supports zsh, bash, fish, and PowerShell.
// See docs/superpowers/plans (plan A5) for the implementation plan.
package shell
```

- [ ] **Step 5: Create `internal/platform/doc.go`**

```go
// Package platform contains OS-specific helpers: default Claude Code config
// directory resolution, shell detection, and OS-conditional file mode bits.
// See docs/superpowers/plans (plan A6) for the implementation plan.
package platform
```

- [ ] **Step 6: Create `internal/cli/doc.go`**

```go
// Package cli holds the cobra command tree for ccx. Each subcommand is one
// file. The package wires together every other internal/* package and is
// built during Phase 2 integration.
// See docs/superpowers/plans (plan P2) for the implementation plan.
package cli
```

- [ ] **Step 7: Create `internal/server/doc.go`**

```go
// Package server implements the local HTTP API consumed by the embedded
// dashboard. It binds to 127.0.0.1 only and serves the contract defined in
// api/openapi.yaml.
// See docs/superpowers/plans (plan P2) for the implementation plan.
package server
```

- [ ] **Step 8: Create `internal/tui/doc.go`**

```go
// Package tui provides the bubbletea-based profile picker used by `ccx use`
// when invoked with no profile name.
// See docs/superpowers/plans (plan P2) for the implementation plan.
package tui
```

- [ ] **Step 9: Create `internal/doctor/doc.go`**

```go
// Package doctor implements `ccx doctor` — a structured set of diagnostic
// checks reported as ✅/⚠/❌.
// See docs/superpowers/plans (plan P2) for the implementation plan.
package doctor
```

- [ ] **Step 10: Create `internal/dashboard/doc.go`**

```go
// Package dashboard exposes the embedded Next.js static build as an fs.FS via
// go:embed. The go:embed directive itself is added in Phase 2 once the web/
// build pipeline exists.
// See docs/superpowers/plans (plan P2) for the implementation plan.
package dashboard
```

- [ ] **Step 11: Verify everything compiles**

Run:
```bash
go build ./internal/...
go test ./...
```
Expected:
- `go build ./internal/...`: succeeds with no output
- `go test ./...`: PASS for `internal/contracts`, `[no test files]` for stub packages

- [ ] **Step 12: Commit**

```bash
git add internal/ cmd/
git commit -m "feat: add Phase 1 package stubs"
```

---

## Task 16: Add CODEOWNERS, SECURITY.md, CONTRIBUTING.md, LICENSE

**Files:**
- Create: `LICENSE`
- Create: `SECURITY.md`
- Create: `CONTRIBUTING.md`
- Create: `.github/CODEOWNERS`

- [ ] **Step 1: Write `LICENSE` (MIT)**

```
MIT License

Copyright (c) 2026 arafa-dev

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Write `SECURITY.md`**

```markdown
# Security policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security-sensitive reports.

Instead, email security reports to `<your-security-email@arafa-dev.com>`
(update before launch) with the following information:

- A description of the issue
- Steps to reproduce
- Impact assessment
- Any suggested mitigation

We aim to acknowledge reports within 48 hours.

## Scope

ccx is a local-only tool. The dashboard HTTP server binds to 127.0.0.1 only.
The most relevant attack surfaces are:

- Local file inclusion via malformed profile paths
- Code execution via crafted JSONL fixtures
- Privilege escalation via misuse of credentials owned by the active profile
- Credential exfiltration through telemetry (we don't have telemetry in v0.1,
  but the constraint is documented for future versions)

## Out of scope

- Bugs in upstream `claude` itself — report those to Anthropic
- Issues that require local root or physical machine access
- Denial-of-service against the local dashboard (it's local)
```

- [ ] **Step 3: Write `CONTRIBUTING.md`**

```markdown
# Contributing to ccx

Thanks for your interest! ccx is in active early development. The contribution
flow is:

1. **Read** [`docs/conventions.md`](docs/conventions.md) for style and workflow
   rules. They are not negotiable.
2. **Open an issue** before starting non-trivial work. Tag with the relevant
   `area/<package>` label.
3. **Fork + branch** off `main`. Branch naming: `feat/<topic>` or `fix/<topic>`.
4. **Write tests first** for any new behavior.
5. **Run** `make ci` locally before pushing.
6. **Open a PR.** The PR template asks two questions; please answer both.

## Setting up a dev environment

```bash
git clone https://github.com/arafa-dev/ccx.git
cd ccx
go install mvdan.cc/gofumpt@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
# (optional) lefthook install
make test
```

## Phase 1 worktrees

If you're picking up a specific Phase 1 package (e.g., `internal/scanner`):

```bash
git worktree add ../ccx-scanner -b feat/scanner main
cd ../ccx-scanner
```

Work in your own worktree. Do not touch files outside your assigned package.
If you discover that a shared contract (`internal/contracts/`, `api/openapi.yaml`,
`docs/conventions.md`, `internal/storage/schema.sql`) needs to change, open an
issue and pause your worktree until a contract-amendment PR is merged to main.
```

- [ ] **Step 4: Write `.github/CODEOWNERS`**

```
* @arafa-dev

# Contracts changes require explicit owner approval
/internal/contracts/    @arafa-dev
/api/openapi.yaml       @arafa-dev
/internal/storage/schema.sql @arafa-dev
/docs/conventions.md    @arafa-dev
```

- [ ] **Step 5: Commit**

```bash
git add LICENSE SECURITY.md CONTRIBUTING.md .github/CODEOWNERS
git commit -m "docs: add LICENSE, SECURITY, CONTRIBUTING, CODEOWNERS"
```

---

## Task 17: Add PR template

**Files:**
- Create: `.github/pull_request_template.md`

- [ ] **Step 1: Write the template**

```markdown
## What

<!-- One paragraph: what this PR changes. -->

## Why

<!-- One paragraph: why this change is needed. Link issue if applicable. -->

## Contract impact

- [ ] This PR does NOT modify `internal/contracts/`, `api/openapi.yaml`,
      `internal/storage/schema.sql`, or `docs/conventions.md`
- [ ] If it does, this is a contract-amendment PR (other worktrees will rebase)

## Checklist

- [ ] Tests added/updated and all pass locally (`make test`)
- [ ] Lint clean locally (`make lint`)
- [ ] No new dependencies without justification in the description
- [ ] Updates to user-visible behavior reflected in `README.md` or `docs/`

## Phase 1 worktree?

If this PR comes from a Phase 1 worktree, list:

- Package: `internal/<name>`
- Plan: `docs/superpowers/plans/<file>`
```

- [ ] **Step 2: Commit**

```bash
git add .github/pull_request_template.md
git commit -m "ci: add PR template"
```

---

## Task 18: Add dependabot config

**Files:**
- Create: `.github/dependabot.yml`

- [ ] **Step 1: Write the dependabot config**

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 5
    commit-message:
      prefix: "build"

  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 5
    commit-message:
      prefix: "ci"
```

(Note: we'll add `npm` ecosystem for `web/` in Phase 1 plan A7 when the Next.js project is scaffolded.)

- [ ] **Step 2: Commit**

```bash
git add .github/dependabot.yml
git commit -m "ci: add dependabot config"
```

---

## Task 19: Run full local CI gate

- [ ] **Step 1: Format check**

```bash
gofumpt -l .
```
Expected: no output (no files need formatting).

If files are listed, run `gofumpt -w .`, review the diff, and commit if changes are real:
```bash
gofumpt -w .
git diff
git add -u && git commit -m "style: gofumpt"
```

- [ ] **Step 2: Lint**

```bash
golangci-lint run
```
Expected: exit 0, no issues.

- [ ] **Step 3: Test (with race detector)**

```bash
go test -race -count=1 ./...
```
Expected: all packages either PASS or report `[no test files]`.

- [ ] **Step 4: Build all internal packages**

```bash
go build ./internal/...
```
Expected: no output, exit 0.

- [ ] **Step 5: Vet**

```bash
go vet ./...
```
Expected: no output.

If any of the above fail, fix the issue and re-run from Step 1 before continuing.

---

## Task 20: Push to GitHub and verify CI

- [ ] **Step 1: Create the GitHub repo**

If the repo doesn't exist on GitHub yet, create it via the web UI or:

```bash
gh repo create arafa-dev/ccx --public --source=. --remote=origin
```

If the repo already exists:

```bash
git remote add origin git@github.com:arafa-dev/ccx.git
```

- [ ] **Step 2: Push**

```bash
git push -u origin main
```

- [ ] **Step 3: Watch the CI run**

```bash
gh run watch
```

Expected: lint, test (×3 OSes), build (×3 OSes) all pass.

If any job fails, address the failure on a feature branch, open a PR, and only merge after CI is green. Do **not** push directly to `main` to fix CI.

- [ ] **Step 4: Add branch protection (manual, GitHub UI)**

In GitHub repo settings → Branches → Add branch protection rule for `main`:
- Require a pull request before merging
- Require status checks to pass: `lint`, `test (ubuntu-22.04)`, `test (macos-14)`, `test (windows-2022)`
- Require linear history (no merge commits)
- Restrict who can push to matching branches (just you)

This locks `main` so Phase 1 worktrees cannot accidentally bypass CI.

---

## Phase 0 done definition

All of the following are true:

- [ ] `go build ./...` succeeds locally
- [ ] `go test ./...` succeeds locally with race detector
- [ ] `golangci-lint run` reports zero issues
- [ ] `gofumpt -l .` produces no output
- [ ] CI run on `main` shows green for lint + test (3 OSes) + build (3 OSes)
- [ ] Branch protection enabled on `main`
- [ ] All files from this plan exist and are committed:
  - `go.mod`
  - `Makefile`
  - `.gitignore`
  - `.golangci.yml`
  - `lefthook.yml`
  - `LICENSE`, `SECURITY.md`, `CONTRIBUTING.md`
  - `.github/workflows/ci.yml`, `.github/dependabot.yml`, `.github/CODEOWNERS`, `.github/pull_request_template.md`
  - `internal/contracts/{types.go,errors.go,interfaces.go,types_test.go,errors_test.go}`
  - `internal/storage/{schema.sql,doc.go}`
  - `internal/{profile,scanner,pricing,shell,platform,cli,server,tui,doctor,dashboard}/doc.go`
  - `api/openapi.yaml`
  - `docs/conventions.md`
- [ ] Tag `phase-0` pushed to mark the checkpoint:
  ```bash
  git tag -a phase-0 -m "Phase 0 complete: contracts locked"
  git push origin phase-0
  ```

After this checkpoint, request the next plan (A1, A2, … A9) before dispatching Phase 1 agents.
