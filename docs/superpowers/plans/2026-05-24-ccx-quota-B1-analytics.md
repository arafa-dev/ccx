# ccx v0.2 B1 — Analytics Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make plan-aware quota usage *visible* — surface 5h-rolling and weekly turn counts per profile in the CLI (`ccx usage --quota`), the HTTP API (`GET /api/quota`), and the dashboard (new "Plan Quota" panel). No behavior changes yet (pressure-aware suggest is B2; switching is B3a/B3b); this plan only adds counters, math, and visualization.

**Architecture:**

- **New leaf package `internal/quota/`** (imports `contracts` + stdlib only) owns: plan-tier default caps, window math (rolling 5h, rolling/anchored weekly), reset-time computation, and the `Compute` helper that turns raw turn counts into `contracts.ProfileQuota` values.
- **`internal/storage/`** gains one new public method `QueryTurnsInWindow(ctx, profile, since, until) (int, error)` that counts `hook_events.event_name IN ('Stop','StopFailure')` rows in the window. No new tables; no migration.
- **`internal/server/`** gains a `QuotaProvider` interface dep (parallel to existing `HeadroomEvaluator`), a `handleQuota` handler, and a `/api/quota` route registration. The wiring layers (`internal/daemon/runtime.go` and `internal/cli/dashboard.go`) construct a concrete `QuotaProvider` that fans `*storage.Store` into the `internal/quota` helper.
- **`internal/cli/usage.go`** gains a `--quota` flag that prints the per-profile windows in tabwriter format.
- **`web/components/quota-panel.tsx`** is a new client component, slotted into `web/components/dashboard.tsx` between the existing `ProfileCards` and `RecommendationPanel`. Data fetched via a new `getQuota()` in `web/lib/api.ts`.

**Tech Stack:** Go 1.22+ stdlib + `modernc.org/sqlite` (existing). React 18 / Next.js 15 (existing).

**Spec reference:** [`docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`](../specs/2026-05-24-ccx-plan-aware-quota.md) — §6.1 (count turns not tokens), §6.2 (hook_events as source), §7.4 (default caps), §8.2 (`ccx usage --quota` UX), §8.5 (dashboard panel).

**Worktree:**

```bash
git fetch origin
git worktree add ../ccx-quota-analytics -b feat/quota-analytics origin/main
cd ../ccx-quota-analytics
```

P0 must already be merged to `origin/main` before this worktree is created (the contracts and openapi types this plan uses live in P0).

**Exit criteria:**

- [ ] `go build ./...` succeeds
- [ ] `go test -race -count=1 ./...` succeeds
- [ ] `golangci-lint run ./...` reports `0 issues.`
- [ ] `cd web && pnpm typecheck && pnpm test` succeed
- [ ] `make ci` green
- [ ] Manual smoke: `./dist/ccx dashboard --no-open` + `curl http://127.0.0.1:7777/api/quota` returns valid JSON for at least one profile
- [ ] Manual smoke: `./dist/ccx usage --quota` prints the new columns
- [ ] PR opened against `main`, CI green, merged
- [ ] Plan index status updated to merged

**Conventions:**

- All Go code uses tabs (gofumpt enforced).
- Every exported symbol has a doc comment starting with the symbol's name.
- Commit message format: `type(scope): subject`. Scopes used in this plan: `quota`, `storage`, `server`, `cli`, `web`.
- One commit per task; do not batch.
- Do **not** edit `internal/contracts/`, `internal/storage/schema.sql`, `api/openapi.yaml`, or `docs/conventions.md`. They are frozen; their changes for this feature landed in P0.

---

## Pre-flight

```bash
pwd                                                  # → /Users/arafa/Developer/ccx-quota-analytics
git status                                           # → On branch feat/quota-analytics, working tree clean
git rev-parse --short HEAD                           # → matches origin/main tip with P0 merged
grep -l "PlanTier" internal/contracts/types.go       # → file path printed (P0 landed)
grep -l "/api/quota" api/openapi.yaml                # → file path printed (P0 landed)
test -d internal/storage && echo OK                   # → OK
test -d internal/server && echo OK                    # → OK
test -d internal/cli && echo OK                       # → OK
test -d web/components && echo OK                     # → OK
go build ./... && echo OK                             # → OK
cd web && pnpm install --frozen-lockfile && cd ..
```

If `PlanTier` is missing from `internal/contracts/types.go`, **stop** — P0 has not landed. Wait for it.

---

## Task 1: Create `internal/quota/` package with plan-tier defaults (TDD)

**Files:**
- Create: `internal/quota/doc.go`
- Create: `internal/quota/plans.go`
- Create: `internal/quota/plans_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/quota/plans_test.go`:

```go
package quota_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/quota"
)

func TestDefaultCapsKnownTiers(t *testing.T) {
	cases := []struct {
		tier       string
		want5h     int
		wantWeekly int
	}{
		{"pro", 45, 0},
		{"max5", 225, 0},
		{"max20", 900, 0},
		{"api", 0, 0},
		{"", 0, 0},
	}
	for _, tc := range cases {
		got5h, gotWeekly := quota.DefaultCaps(tc.tier)
		if got5h != tc.want5h {
			t.Errorf("DefaultCaps(%q).5h = %d, want %d", tc.tier, got5h, tc.want5h)
		}
		if gotWeekly != tc.wantWeekly {
			t.Errorf("DefaultCaps(%q).weekly = %d, want %d", tc.tier, gotWeekly, tc.wantWeekly)
		}
	}
}

func TestDefaultCapsUnknownTierReturnsZero(t *testing.T) {
	got5h, gotWeekly := quota.DefaultCaps("bedrock-premium")
	if got5h != 0 || gotWeekly != 0 {
		t.Errorf("unknown tier: got (%d, %d), want (0, 0)", got5h, gotWeekly)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test -race -count=1 ./internal/quota/...
```

Expected: FAIL — package `quota` does not exist.

- [ ] **Step 3: Create the package**

`internal/quota/doc.go`:

```go
// Package quota computes per-profile plan-aware quota windows from local hook
// telemetry. It owns the shipped default caps per Anthropic subscription tier
// and the math for rolling-5h and rolling-or-anchored-weekly windows.
package quota
```

`internal/quota/plans.go`:

```go
package quota

// DefaultCaps returns the shipped default per-window turn caps for the given
// Anthropic plan tier. These are best-effort defaults; users may override them
// per profile via ProfileLimits.Caps5hTurns and CapsWeeklyTurns. Unknown tiers
// (including the empty string and "api") return zeros, which disables
// plan-aware quota tracking for that profile in downstream consumers.
//
// Numeric values current as of 2026-05-24; revisit when Anthropic publishes
// authoritative caps (see spec §13).
func DefaultCaps(tier string) (turns5h, turnsWeekly int) {
	switch tier {
	case "pro":
		return 45, 0
	case "max5":
		return 225, 0
	case "max20":
		return 900, 0
	default:
		// "api", "", and unknown tiers all opt out.
		return 0, 0
	}
}
```

- [ ] **Step 4: Run, confirm pass**

```bash
gofumpt -w internal/quota/
go test -race -count=1 ./internal/quota/...
golangci-lint run ./internal/quota/...
```

Expected: tests pass, lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/quota/doc.go internal/quota/plans.go internal/quota/plans_test.go
git commit -m "feat(quota): create package with plan-tier default caps"
```

---

## Task 2: Window math — `WindowBounds`, `ResetTime`, `Pct` (TDD)

**Files:**
- Create: `internal/quota/windows.go`
- Create: `internal/quota/windows_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/quota/windows_test.go`:

```go
package quota_test

import (
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/quota"
)

func TestWindow5hBoundsIsRolling(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	since, until := quota.Window5hBounds(now)
	if !until.Equal(now) {
		t.Errorf("until = %v, want now", until)
	}
	if want := now.Add(-5 * time.Hour); !since.Equal(want) {
		t.Errorf("since = %v, want %v", since, want)
	}
}

func TestWindowWeeklyBoundsRolling(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	since, until := quota.WindowWeeklyBounds(now, "rolling")
	if !until.Equal(now) {
		t.Errorf("until = %v, want now", until)
	}
	if want := now.Add(-7 * 24 * time.Hour); !since.Equal(want) {
		t.Errorf("since = %v, want %v", since, want)
	}
}

func TestWindowWeeklyBoundsAnchoredToMonday(t *testing.T) {
	// Sunday May 24, 2026 18:42 UTC. Most recent Monday at 00:00 UTC is May 18.
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	since, until := quota.WindowWeeklyBounds(now, "monday")
	if !until.Equal(now) {
		t.Errorf("until = %v, want now", until)
	}
	wantSince := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	if !since.Equal(wantSince) {
		t.Errorf("since = %v, want %v", since, wantSince)
	}
}

func TestWindowWeeklyBoundsAnchoredEmptyDefaultsRolling(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC)
	since, _ := quota.WindowWeeklyBounds(now, "")
	if want := now.Add(-7 * 24 * time.Hour); !since.Equal(want) {
		t.Errorf("empty anchor should default to rolling; since = %v, want %v", since, want)
	}
}

func TestResetTime5h(t *testing.T) {
	// Oldest turn was 4h ago → resets in 1h from now.
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	oldest := now.Add(-4 * time.Hour)
	want := oldest.Add(5 * time.Hour)
	got := quota.ResetTime5h(oldest)
	if !got.Equal(want) {
		t.Errorf("ResetTime5h = %v, want %v", got, want)
	}
}

func TestResetTimeWeeklyRolling(t *testing.T) {
	oldest := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	want := oldest.Add(7 * 24 * time.Hour)
	got := quota.ResetTimeWeekly(oldest, "rolling")
	if !got.Equal(want) {
		t.Errorf("ResetTimeWeekly(rolling) = %v, want %v", got, want)
	}
}

func TestResetTimeWeeklyAnchored(t *testing.T) {
	// Now is Sunday May 24; next Monday 00:00 UTC is May 25.
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	want := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	got := quota.ResetTimeWeeklyAnchored(now, "monday")
	if !got.Equal(want) {
		t.Errorf("ResetTimeWeeklyAnchored = %v, want %v", got, want)
	}
}

func TestPct(t *testing.T) {
	cases := []struct {
		used, cap int
		want      float64
	}{
		{0, 100, 0},
		{50, 100, 50},
		{100, 100, 100},
		{150, 100, 100}, // clamps to 100
		{10, 0, 0},      // cap 0 → no tracking → pct 0
	}
	for _, tc := range cases {
		got := quota.Pct(tc.used, tc.cap)
		if got != tc.want {
			t.Errorf("Pct(%d, %d) = %v, want %v", tc.used, tc.cap, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test -race -count=1 ./internal/quota/...
```

Expected: FAIL — undefined symbols.

- [ ] **Step 3: Write the implementation**

Create `internal/quota/windows.go`:

```go
package quota

import (
	"strings"
	"time"
)

// FiveHourWindow is the duration of the rolling 5-hour subscription window.
const FiveHourWindow = 5 * time.Hour

// WeekWindow is the duration of the rolling 7-day subscription window.
const WeekWindow = 7 * 24 * time.Hour

// Window5hBounds returns the [since, until] bounds of the rolling 5-hour
// window ending at now.
func Window5hBounds(now time.Time) (since, until time.Time) {
	return now.Add(-FiveHourWindow), now
}

// WindowWeeklyBounds returns the [since, until] bounds of the weekly window
// ending at now. anchor is one of: "rolling" (or empty for default), or a
// lowercase weekday name ("monday".."sunday") for calendar-anchored windows.
// Unknown anchors fall back to rolling.
func WindowWeeklyBounds(now time.Time, anchor string) (since, until time.Time) {
	weekday, ok := parseWeekday(anchor)
	if !ok {
		return now.Add(-WeekWindow), now
	}
	since = mostRecentWeekdayMidnight(now, weekday)
	return since, now
}

// ResetTime5h returns when the 5-hour window resets, given the timestamp of
// the oldest turn currently inside the window. If oldest is the zero value,
// the returned time is also the zero value (no meaningful reset).
func ResetTime5h(oldestInWindow time.Time) time.Time {
	if oldestInWindow.IsZero() {
		return time.Time{}
	}
	return oldestInWindow.Add(FiveHourWindow)
}

// ResetTimeWeekly returns when the weekly window resets. For rolling anchors,
// this is oldest+7d. For weekday anchors, callers should use
// ResetTimeWeeklyAnchored instead — this function falls back to rolling for
// unknown anchors.
func ResetTimeWeekly(oldestInWindow time.Time, anchor string) time.Time {
	if _, ok := parseWeekday(anchor); ok {
		// Caller should use ResetTimeWeeklyAnchored, but be forgiving.
		return ResetTimeWeeklyAnchored(time.Now().UTC(), anchor)
	}
	if oldestInWindow.IsZero() {
		return time.Time{}
	}
	return oldestInWindow.Add(WeekWindow)
}

// ResetTimeWeeklyAnchored returns when the next anchored weekday-midnight
// boundary occurs after now. anchor must be a valid weekday name; unknown
// anchors return the zero time.
func ResetTimeWeeklyAnchored(now time.Time, anchor string) time.Time {
	weekday, ok := parseWeekday(anchor)
	if !ok {
		return time.Time{}
	}
	candidate := mostRecentWeekdayMidnight(now, weekday).Add(WeekWindow)
	return candidate
}

// Pct returns used/cap as a percentage, clamped to [0, 100]. Returns 0 when
// cap is zero (no tracking configured).
func Pct(used, cap int) float64 {
	if cap <= 0 {
		return 0
	}
	pct := float64(used) / float64(cap) * 100
	if pct > 100 {
		return 100
	}
	if pct < 0 {
		return 0
	}
	return pct
}

func parseWeekday(s string) (time.Weekday, bool) {
	switch strings.ToLower(s) {
	case "sunday":
		return time.Sunday, true
	case "monday":
		return time.Monday, true
	case "tuesday":
		return time.Tuesday, true
	case "wednesday":
		return time.Wednesday, true
	case "thursday":
		return time.Thursday, true
	case "friday":
		return time.Friday, true
	case "saturday":
		return time.Saturday, true
	default:
		return 0, false
	}
}

func mostRecentWeekdayMidnight(now time.Time, target time.Weekday) time.Time {
	now = now.UTC()
	daysBack := (int(now.Weekday()) - int(target) + 7) % 7
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, -daysBack)
}
```

- [ ] **Step 4: Run, confirm pass**

```bash
gofumpt -w internal/quota/
go test -race -count=1 ./internal/quota/...
golangci-lint run ./internal/quota/...
```

Expected: tests pass, lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/quota/windows.go internal/quota/windows_test.go
git commit -m "feat(quota): window-bounds, reset-time, and pct helpers"
```

---

## Task 3: Add `QueryTurnsInWindow` to storage (TDD)

**Files:**
- Create: `internal/storage/turns.go`
- Create: `internal/storage/turns_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/storage/turns_test.go`:

```go
package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestQueryTurnsInWindowCountsStopAndStopFailure(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")

	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	events := []contracts.HookEvent{
		{Profile: "work", Session: "s1", Event: "SessionStart", Timestamp: now.Add(-6 * time.Hour)},
		{Profile: "work", Session: "s1", Event: "Stop", Timestamp: now.Add(-4 * time.Hour)},
		{Profile: "work", Session: "s1", Event: "Stop", Timestamp: now.Add(-3 * time.Hour)},
		{Profile: "work", Session: "s1", Event: "StopFailure", Timestamp: now.Add(-2 * time.Hour), Error: "rate_limit"},
		{Profile: "work", Session: "s1", Event: "Stop", Timestamp: now.Add(-1 * time.Hour)},
		{Profile: "work", Session: "s1", Event: "SessionEnd", Timestamp: now.Add(-30 * time.Minute)},
		{Profile: "work", Session: "s1", Event: "Stop", Timestamp: now.Add(-10 * time.Minute)},
	}
	for _, e := range events {
		if err := s.InsertHookEvent(ctx, "work", e); err != nil {
			t.Fatalf("InsertHookEvent: %v", err)
		}
	}

	// Window: last 5h. Should include 4 Stops + 1 rate_limit StopFailure (the one at -6h is outside).
	since, until := now.Add(-5*time.Hour), now
	got, err := s.QueryTurnsInWindow(ctx, "work", since, until)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if got != 5 {
		t.Errorf("turn count: got %d, want 5", got)
	}
}

func TestQueryTurnsInWindowExcludesAuthFailures(t *testing.T) {
	// Per spec §4, auth-related StopFailures don't count because they never
	// reached Anthropic and didn't burn quota.
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	events := []contracts.HookEvent{
		{Profile: "work", Session: "s1", Event: "Stop", Timestamp: now.Add(-3 * time.Hour)},
		{Profile: "work", Session: "s1", Event: "StopFailure", Timestamp: now.Add(-2 * time.Hour), Error: "authentication_failed"},
		{Profile: "work", Session: "s1", Event: "StopFailure", Timestamp: now.Add(-1*time.Hour - 30*time.Minute), Error: "oauth_org_not_allowed"},
		{Profile: "work", Session: "s1", Event: "StopFailure", Timestamp: now.Add(-1 * time.Hour), Error: "rate_limit"},
		{Profile: "work", Session: "s1", Event: "StopFailure", Timestamp: now.Add(-30 * time.Minute), Error: "server_error"},
	}
	for _, e := range events {
		if err := s.InsertHookEvent(ctx, "work", e); err != nil {
			t.Fatalf("InsertHookEvent: %v", err)
		}
	}
	got, err := s.QueryTurnsInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	// 1 Stop + 2 non-auth StopFailures = 3. Auth failures excluded.
	if got != 3 {
		t.Errorf("turn count: got %d, want 3 (auth failures should be excluded)", got)
	}
}

func TestQueryTurnsInWindowEmptyReturnsZero(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")
	got, err := s.QueryTurnsInWindow(ctx, "work", time.Now().Add(-1*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if got != 0 {
		t.Errorf("empty turn count: got %d, want 0", got)
	}
}

func TestQueryTurnsInWindowOtherProfileNotCounted(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	mustSaveProfile(t, s, "work")
	mustSaveProfile(t, s, "personal")
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	if err := s.InsertHookEvent(ctx, "personal", contracts.HookEvent{
		Profile: "personal", Session: "s1", Event: "Stop", Timestamp: now.Add(-1 * time.Hour),
	}); err != nil {
		t.Fatalf("InsertHookEvent: %v", err)
	}
	got, err := s.QueryTurnsInWindow(ctx, "work", now.Add(-5*time.Hour), now)
	if err != nil {
		t.Fatalf("QueryTurnsInWindow: %v", err)
	}
	if got != 0 {
		t.Errorf("isolation: work turns = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test -race -count=1 ./internal/storage/...
```

Expected: FAIL — `QueryTurnsInWindow` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/storage/turns.go`:

```go
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// turnEventFilter is the SQL fragment that defines what counts as a "turn"
// against subscription quota. Per spec §4, we count every Stop event and most
// StopFailure events. Auth-related failures (authentication_failed and
// oauth_org_not_allowed) are excluded because those requests never reached
// Anthropic and therefore did not burn plan quota.
const turnEventFilter = `(
    event_name = 'Stop'
 OR (event_name = 'StopFailure' AND error NOT IN ('authentication_failed', 'oauth_org_not_allowed'))
)`

// QueryTurnsInWindow returns the number of completed Claude Code assistant
// turns for the given profile in the half-open interval [since, until]. Used
// by internal/quota to compute rolling-5h and weekly turn windows.
func (s *Store) QueryTurnsInWindow(ctx context.Context, profileName string, since, until time.Time) (int, error) {
	q := `SELECT COUNT(*) FROM hook_events
WHERE profile_name = ?
  AND ts BETWEEN ? AND ?
  AND ` + turnEventFilter
	var n int
	if err := s.db.QueryRowContext(ctx, q, profileName, since.UnixNano(), until.UnixNano()).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting turns for %q: %w", profileName, err)
	}
	return n, nil
}

// QueryOldestTurnInWindow returns the timestamp of the earliest counted turn
// for the given profile inside [since, until], or the zero time if no rows
// match. "Counted" follows the same filter as QueryTurnsInWindow.
func (s *Store) QueryOldestTurnInWindow(ctx context.Context, profileName string, since, until time.Time) (time.Time, error) {
	q := `SELECT MIN(ts) FROM hook_events
WHERE profile_name = ?
  AND ts BETWEEN ? AND ?
  AND ` + turnEventFilter
	var nsec sql.NullInt64
	if err := s.db.QueryRowContext(ctx, q, profileName, since.UnixNano(), until.UnixNano()).Scan(&nsec); err != nil {
		return time.Time{}, fmt.Errorf("oldest turn for %q: %w", profileName, err)
	}
	if !nsec.Valid {
		return time.Time{}, nil
	}
	return time.Unix(0, nsec.Int64).UTC(), nil
}
```

- [ ] **Step 4: Run, confirm pass**

```bash
gofumpt -w internal/storage/turns.go
go test -race -count=1 ./internal/storage/...
golangci-lint run ./internal/storage/...
```

Expected: tests pass, lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/turns.go internal/storage/turns_test.go
git commit -m "feat(storage): add QueryTurnsInWindow and QueryOldestTurnInWindow"
```

---

## Task 4: `quota.Compute` and `quota.Computer` (TDD)

**Files:**
- Create: `internal/quota/compute.go`
- Create: `internal/quota/compute_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/quota/compute_test.go`:

```go
package quota_test

import (
	"context"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/quota"
)

type fakeTurnsStore struct {
	turns       map[string]int
	oldestTurns map[string]time.Time
}

func (f *fakeTurnsStore) QueryTurnsInWindow(_ context.Context, profile string, _, _ time.Time) (int, error) {
	return f.turns[profile], nil
}

func (f *fakeTurnsStore) QueryOldestTurnInWindow(_ context.Context, profile string, _, _ time.Time) (time.Time, error) {
	return f.oldestTurns[profile], nil
}

func TestComputeUsesDefaultCapsWhenOverrideZero(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	store := &fakeTurnsStore{
		turns:       map[string]int{"work:5h": 142, "work:weekly": 1203},
		oldestTurns: map[string]time.Time{"work:5h": now.Add(-4 * time.Hour)},
	}
	// Note: fakeTurnsStore in this test ignores the window distinction —
	// use a different key per profile to keep the test simple.
	// We model the fake with two separate "profile names" for the two windows.
	computer := quota.Computer{Store: profileWindowAdapter{store}, Now: func() time.Time { return now }}

	profile := contracts.Profile{
		Name: "work",
		Limits: contracts.ProfileLimits{
			PlanTier: "max20",
		},
	}
	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if got.Profile != "work" || got.PlanTier != "max20" {
		t.Errorf("Profile/PlanTier: got %+v", got)
	}
	if got.Window5h.Cap != 900 {
		t.Errorf("default Caps5h: got %d, want 900", got.Window5h.Cap)
	}
	if got.Window5h.Used != 142 {
		t.Errorf("Window5h.Used: got %d, want 142", got.Window5h.Used)
	}
	wantPct := 142.0 / 900.0 * 100
	if diff := got.Window5h.Pct - wantPct; diff > 0.01 || diff < -0.01 {
		t.Errorf("Window5h.Pct: got %v, want %v", got.Window5h.Pct, wantPct)
	}
}

func TestComputeOverridesCapsViaProfileLimits(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	computer := quota.Computer{
		Store: profileWindowAdapter{&fakeTurnsStore{turns: map[string]int{"x:5h": 50}}},
		Now:   func() time.Time { return now },
	}
	profile := contracts.Profile{
		Name: "x",
		Limits: contracts.ProfileLimits{
			PlanTier:    "max20",
			Caps5hTurns: 100,
		},
	}
	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if got.Window5h.Cap != 100 {
		t.Errorf("override Caps5h: got %d, want 100", got.Window5h.Cap)
	}
	if got.Window5h.Pct != 50 {
		t.Errorf("Window5h.Pct: got %v, want 50", got.Window5h.Pct)
	}
}

func TestComputeEmptyPlanTierReturnsZeroCaps(t *testing.T) {
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	computer := quota.Computer{
		Store: profileWindowAdapter{&fakeTurnsStore{turns: map[string]int{"x:5h": 30}}},
		Now:   func() time.Time { return now },
	}
	profile := contracts.Profile{Name: "x"} // no PlanTier
	got, err := computer.For(context.Background(), profile)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if got.Window5h.Cap != 0 || got.Window5h.Pct != 0 {
		t.Errorf("no plan tier: got Cap=%d Pct=%v, want 0/0", got.Window5h.Cap, got.Window5h.Pct)
	}
}

func TestComputeEmptyPlanTierIgnoresOverrides(t *testing.T) {
	// Per spec §3.1/§6.1: empty PlanTier disables tracking regardless of
	// Caps5hTurns or CapsWeeklyTurns overrides. A user who sets caps but
	// forgets to set a tier should not get surprise quota tracking.
	now := time.Date(2026, 5, 24, 18, 0, 0, 0, time.UTC)
	computer := quota.Computer{
		Store: profileWindowAdapter{&fakeTurnsStore{turns: map[string]int{"x:5h": 30}}},
		Now:   func() time.Time { return now },
	}
	profile := contracts.Profile{
		Name: "x",
		Limits: contracts.ProfileLimits{
			Caps5hTurns:     500, // would normally take effect
			CapsWeeklyTurns: 1000,
			// PlanTier deliberately empty
		},
	}
	got, _ := computer.For(context.Background(), profile)
	if got.Window5h.Cap != 0 || got.WindowWeekly.Cap != 0 {
		t.Errorf("empty tier with overrides: got Cap5h=%d CapWeekly=%d, want 0/0 (tier gate is upstream)",
			got.Window5h.Cap, got.WindowWeekly.Cap)
	}
}

// profileWindowAdapter lets fakeTurnsStore answer per-(profile, window) by
// mapping the window to a synthetic profile name. Real Store impl doesn't
// need this; it just sees the bounds.
type profileWindowAdapter struct{ inner *fakeTurnsStore }

func (a profileWindowAdapter) QueryTurnsInWindow(_ context.Context, profile string, since, until time.Time) (int, error) {
	key := profile + ":5h"
	if until.Sub(since) > 24*time.Hour {
		key = profile + ":weekly"
	}
	return a.inner.turns[key], nil
}

func (a profileWindowAdapter) QueryOldestTurnInWindow(_ context.Context, profile string, since, until time.Time) (time.Time, error) {
	key := profile + ":5h"
	if until.Sub(since) > 24*time.Hour {
		key = profile + ":weekly"
	}
	return a.inner.oldestTurns[key], nil
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test -race -count=1 ./internal/quota/...
```

Expected: FAIL — `Computer`, `For` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/quota/compute.go`:

```go
package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Store is the storage surface Computer needs. Implemented by *storage.Store
// via its QueryTurnsInWindow and QueryOldestTurnInWindow methods.
type Store interface {
	QueryTurnsInWindow(ctx context.Context, profileName string, since, until time.Time) (int, error)
	QueryOldestTurnInWindow(ctx context.Context, profileName string, since, until time.Time) (time.Time, error)
}

// Computer builds ProfileQuota values for one or many profiles by combining
// declared plan-tier caps (plus optional per-profile overrides) with live
// turn counts from Store.
type Computer struct {
	Store Store
	Now   func() time.Time // injectable for tests; defaults to time.Now().UTC()
}

func (c Computer) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

// For computes the ProfileQuota for one profile.
func (c Computer) For(ctx context.Context, p contracts.Profile) (contracts.ProfileQuota, error) {
	if c.Store == nil {
		return contracts.ProfileQuota{}, fmt.Errorf("quota: Computer.Store is nil")
	}
	now := c.now()
	cap5h, capWeekly := effectiveCaps(p.Limits)

	since5h, until5h := Window5hBounds(now)
	used5h, err := c.Store.QueryTurnsInWindow(ctx, p.Name, since5h, until5h)
	if err != nil {
		return contracts.ProfileQuota{}, fmt.Errorf("5h count for %q: %w", p.Name, err)
	}
	oldest5h, err := c.Store.QueryOldestTurnInWindow(ctx, p.Name, since5h, until5h)
	if err != nil {
		return contracts.ProfileQuota{}, fmt.Errorf("oldest 5h for %q: %w", p.Name, err)
	}

	sinceWeekly, untilWeekly := WindowWeeklyBounds(now, p.Limits.WeeklyAnchor)
	usedWeekly, err := c.Store.QueryTurnsInWindow(ctx, p.Name, sinceWeekly, untilWeekly)
	if err != nil {
		return contracts.ProfileQuota{}, fmt.Errorf("weekly count for %q: %w", p.Name, err)
	}
	oldestWeekly, err := c.Store.QueryOldestTurnInWindow(ctx, p.Name, sinceWeekly, untilWeekly)
	if err != nil {
		return contracts.ProfileQuota{}, fmt.Errorf("oldest weekly for %q: %w", p.Name, err)
	}

	resetsWeekly := weeklyResetTime(now, oldestWeekly, p.Limits.WeeklyAnchor)

	return contracts.ProfileQuota{
		Profile:  p.Name,
		PlanTier: p.Limits.PlanTier,
		Window5h: contracts.QuotaWindow{
			Used:     used5h,
			Cap:      cap5h,
			Pct:      Pct(used5h, cap5h),
			ResetsAt: ResetTime5h(oldest5h),
		},
		WindowWeekly: contracts.QuotaWindow{
			Used:     usedWeekly,
			Cap:      capWeekly,
			Pct:      Pct(usedWeekly, capWeekly),
			ResetsAt: resetsWeekly,
		},
	}, nil
}

// All computes ProfileQuota for every profile in the slice. Errors from one
// profile do not short-circuit the others; per-profile errors are returned
// alongside the successful values via the failures map.
func (c Computer) All(ctx context.Context, profiles []contracts.Profile) (results []contracts.ProfileQuota, failures map[string]error, err error) {
	results = make([]contracts.ProfileQuota, 0, len(profiles))
	failures = map[string]error{}
	for i := range profiles {
		q, err := c.For(ctx, profiles[i])
		if err != nil {
			failures[profiles[i].Name] = err
			continue
		}
		results = append(results, q)
	}
	return results, failures, nil
}

func effectiveCaps(limits contracts.ProfileLimits) (cap5h, capWeekly int) {
	// Empty PlanTier disables plan-aware tracking regardless of any override
	// values. Spec §3.1 / §6.1: empty tier means "no quota tracking".
	if limits.PlanTier == "" {
		return 0, 0
	}
	d5h, dWeekly := DefaultCaps(limits.PlanTier)
	if limits.Caps5hTurns > 0 {
		cap5h = limits.Caps5hTurns
	} else {
		cap5h = d5h
	}
	if limits.CapsWeeklyTurns > 0 {
		capWeekly = limits.CapsWeeklyTurns
	} else {
		capWeekly = dWeekly
	}
	return cap5h, capWeekly
}

func weeklyResetTime(now, oldest time.Time, anchor string) time.Time {
	if _, ok := parseWeekday(anchor); ok {
		return ResetTimeWeeklyAnchored(now, anchor)
	}
	return ResetTimeWeekly(oldest, anchor)
}
```

- [ ] **Step 4: Run, confirm pass**

```bash
gofumpt -w internal/quota/
go test -race -count=1 ./internal/quota/...
golangci-lint run ./internal/quota/...
```

Expected: tests pass, lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/quota/compute.go internal/quota/compute_test.go
git commit -m "feat(quota): Computer.For and Computer.All"
```

---

## Task 5: Wire `internal/server` with QuotaProvider and add `handleQuota` (TDD)

**Files:**
- Modify: `internal/server/server.go`
- Create: `internal/server/quota.go`
- Modify: `internal/server/handlers.go` (just to expose `writeJSON`/`writeError` if needed; they already exist — confirm)
- Modify: `internal/server/server_test.go` (or create `internal/server/quota_test.go`)

- [ ] **Step 1: Write the failing test**

Create `internal/server/quota_test.go`:

```go
package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/server"
)

type stubQuotaProvider struct {
	rows []contracts.ProfileQuota
	err  error
}

func (s *stubQuotaProvider) Quota(ctx context.Context, profileFilter string) ([]contracts.ProfileQuota, error) {
	if s.err != nil {
		return nil, s.err
	}
	if profileFilter == "" {
		return s.rows, nil
	}
	out := []contracts.ProfileQuota{}
	for _, r := range s.rows {
		if r.Profile == profileFilter {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestHandleQuotaAllProfiles(t *testing.T) {
	rows := []contracts.ProfileQuota{
		{Profile: "work", PlanTier: "max20", Window5h: contracts.QuotaWindow{Used: 142, Cap: 900, Pct: 15.78, ResetsAt: time.Now().Add(time.Hour)}},
		{Profile: "personal", PlanTier: "pro", Window5h: contracts.QuotaWindow{Used: 45, Cap: 45, Pct: 100}},
	}
	srv := server.New(server.Deps{Quota: &stubQuotaProvider{rows: rows}}, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/quota", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []contracts.ProfileQuota
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("rows: got %d, want 2", len(got))
	}
}

func TestHandleQuotaProfileFilter(t *testing.T) {
	rows := []contracts.ProfileQuota{
		{Profile: "work"}, {Profile: "personal"},
	}
	srv := server.New(server.Deps{Quota: &stubQuotaProvider{rows: rows}}, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/quota?profile=work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var got []contracts.ProfileQuota
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Profile != "work" {
		t.Errorf("got %+v, want single 'work' row", got)
	}
}

func TestHandleQuotaProviderMissingReturns503(t *testing.T) {
	// 503 (not 500): "Quota dep not wired" is operational, not an internal
	// failure. Helps users diagnose mis-configured ccx dashboard / daemon.
	srv := server.New(server.Deps{}, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/quota", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test -race -count=1 ./internal/server/...
```

Expected: FAIL — `server.Deps.Quota` undefined; `/api/quota` route missing.

- [ ] **Step 3: Extend `Deps` and add interface**

In `internal/server/server.go`:

1. Add to the `Deps` struct:
   ```go
   // Quota provides per-profile quota window data for /api/quota.
   Quota QuotaProvider
   ```

2. Add the interface declaration (alongside `HookStatusProvider`, etc.):
   ```go
   // QuotaProvider returns per-profile plan-aware quota windows. profileFilter
   // is the value of the `profile` query parameter; empty means "all profiles".
   type QuotaProvider interface {
       Quota(ctx context.Context, profileFilter string) ([]contracts.ProfileQuota, error)
   }
   ```

3. Register the route in `(s *Server) routes()`:
   ```go
   s.mux.Get("/api/quota", s.handleQuota)
   ```

- [ ] **Step 4: Write the handler**

Create `internal/server/quota.go`:

```go
package server

import (
	"fmt"
	"net/http"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func (s *Server) handleQuota(w http.ResponseWriter, r *http.Request) {
	if s.deps.Quota == nil {
		// 503: operational misconfiguration (dep not wired), not internal
		// failure. Helps users diagnose offline-mode dashboards.
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("quota provider unavailable"))
		return
	}
	rows, err := s.deps.Quota.Quota(r.Context(), r.URL.Query().Get("profile"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if rows == nil {
		rows = []contracts.ProfileQuota{}
	}
	writeJSON(w, http.StatusOK, rows)
}
```

- [ ] **Step 5: Run, confirm pass**

```bash
gofumpt -w internal/server/
go test -race -count=1 ./internal/server/...
golangci-lint run ./internal/server/...
```

Expected: tests pass, lint clean.

- [ ] **Step 6: Commit**

```bash
git add internal/server/server.go internal/server/quota.go internal/server/quota_test.go
git commit -m "feat(server): add QuotaProvider dep and /api/quota handler"
```

---

## Task 6: Wire daemon and dashboard CLI to satisfy the new `Quota` dep

**Files:**
- Create: `internal/quotawire/doc.go`
- Create: `internal/quotawire/adapter.go`
- Create: `internal/quotawire/adapter_test.go`
- Modify: `internal/daemon/runtime.go`
- Modify: `internal/cli/dashboard.go`

> **Why `internal/quotawire/` and not `internal/cli/`?** Both the daemon and
> the foreground dashboard CLI need this adapter. Putting it in `internal/cli/`
> would force `internal/daemon/` to import `internal/cli/` — a layering
> violation (and a real cycle risk; CLI already imports daemon for the
> `daemon` subcommand). A standalone `internal/quotawire/` leaf package
> imports only `internal/contracts`, `internal/profile`, `internal/quota`,
> `internal/storage` — no cycles. It is allowed to fan in from multiple call
> sites because it is integration glue, like `internal/cli/` itself.

- [ ] **Step 1: Create the adapter package**

Create `internal/quotawire/doc.go`:

```go
// Package quotawire constructs a server.QuotaProvider from concrete repo
// types (profile.Manager + *storage.Store + quota.Computer). It exists so
// the daemon and the foreground dashboard CLI can share the wiring without
// either importing the other.
package quotawire
```

Create `internal/quotawire/adapter.go`:

```go
package quotawire

import (
	"context"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/quota"
	"github.com/arafa-dev/ccx/internal/storage"
)

// Adapter satisfies server.QuotaProvider by combining a *storage.Store
// (turn-count source), a profile.Manager (profile list), and a quota.Computer.
type Adapter struct {
	Store    *storage.Store
	Profiles *profile.Manager
}

// Quota returns per-profile ProfileQuota rows for the given profile filter
// ("" means all). Per-profile compute errors are logged inside quota.Computer
// and absent from the result; transport-level errors propagate.
func (a *Adapter) Quota(ctx context.Context, profileFilter string) ([]contracts.ProfileQuota, error) {
	profiles, err := a.Profiles.List(ctx)
	if err != nil {
		return nil, err
	}
	if profileFilter != "" {
		filtered := profiles[:0]
		for i := range profiles {
			if profiles[i].Name == profileFilter {
				filtered = append(filtered, profiles[i])
			}
		}
		profiles = filtered
	}
	computer := quota.Computer{Store: a.Store}
	rows, _, err := computer.All(ctx, profiles)
	return rows, err
}
```

Add a unit test (`adapter_test.go`) covering: empty profile list → empty slice; filter for missing profile → empty slice; filter for one of N → exactly one row.

- [ ] **Step 2: Inject into daemon runtime**

In `internal/daemon/runtime.go`, find the `server.Deps{...}` construction (around line 112) and add the new field:

```go
srv := server.New(server.Deps{
    Store:    deps.Store,
    Pricing:  deps.Pricing,
    Profiles: deps.Profiles,
    WebRoot:  webFS,
    Daemon:   statusProvider,
    Hooks:    &hooks.Service{Profiles: deps.Profiles},
    Headroom: headroom.Evaluator{Store: deps.Store, Pricing: deps.Pricing},
    Quota:    &quotawire.Adapter{Store: deps.Store, Profiles: deps.Profiles},
}, opts.Version)
```

Add the import: `"github.com/arafa-dev/ccx/internal/quotawire"`.

Note: `deps.Store` in `internal/daemon/` is already the concrete `*storage.Store` (see `buildRuntimeDeps`), so the field types line up without a type assertion.

- [ ] **Step 3: Inject into the dashboard CLI**

Open `internal/cli/dashboard.go`. Find the `server.New(server.Deps{...})` construction. Add the same `Quota` field:

```go
Quota: &quotawire.Adapter{Store: deps.Store.(*storage.Store), Profiles: deps.Profiles},
```

The `Store` field in `cli.Deps` is `contracts.Store` (an interface). Cast to the concrete `*storage.Store` to satisfy `Adapter`. The cast is safe because `buildDeps` always uses `storage.NewStore` to construct it — add a comment to that effect:

```go
// Safe assertion: buildDeps always uses storage.NewStore. The interface
// indirection in cli.Deps is for testability, not for swapping backends.
Quota: &quotawire.Adapter{Store: deps.Store.(*storage.Store), Profiles: deps.Profiles},
```

Add imports: `"github.com/arafa-dev/ccx/internal/quotawire"`, `"github.com/arafa-dev/ccx/internal/storage"` (if not already present).

- [ ] **Step 4: Verify builds and tests**

```bash
go build ./...
go test -race -count=1 ./...
golangci-lint run ./...
```

Expected: all green. If the daemon import causes a cycle, fall back to `internal/quotawire/` per Step 2's note.

- [ ] **Step 5: Commit**

```bash
git add internal/quotawire/ internal/cli/dashboard.go internal/daemon/runtime.go
git commit -m "feat(quotawire,daemon,cli): wire quotawire.Adapter into the server"
```

---

## Task 7: Add `ccx usage --quota` flag (TDD)

**Files:**
- Modify: `internal/cli/usage.go`
- Modify: `internal/cli/usage_test.go`

- [ ] **Step 1: Add failing test**

The repo already has a `runCLI(t *testing.T, args ...string) string` helper in
`internal/cli/profile_test.go:59` that returns stdout and uses `t.Setenv` to
redirect HOME. Other `internal/cli/*_test.go` files (e.g. `usage_test.go`'s
`TestUsageEmpty`) follow the same pattern — register a profile via `runCLI`,
then invoke the command under test. Reuse it.

Append to `internal/cli/usage_test.go`:

```go
func TestUsageQuotaFlagPrintsHeaders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)
	runCLI(t, "profile", "set", "work", "--plan-tier", "max20")

	out := runCLI(t, "usage", "--quota")
	for _, want := range []string{"PROFILE", "PLAN", "5H WINDOW", "WEEKLY WINDOW"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected header %q in output:\n%s", want, out)
		}
	}
}
```

(`profile set --plan-tier` is a flag the `profile` plan already supports as
part of P0 → see `ccx profile set` updates in B1's prerequisites. If that
flag is not yet wired, add `"--plan-tier", "max20"` support to the existing
`profile set` command in this same task — it's a one-line cobra flag add and
matches the existing `--daily-tokens`, `--weekly-tokens`, etc. pattern.)

- [ ] **Step 2: Run, confirm fail**

```bash
go test -race -count=1 ./internal/cli/...
```

Expected: FAIL — flag undefined, no headers printed.

- [ ] **Step 3: Implement the flag**

In `internal/cli/usage.go`, add a new boolean flag `quota` and, when set, defer to a new `renderUsageQuota(...)` function that:

- Builds a `quota.Computer` from the existing `deps.Store` (cast to `*storage.Store`) — or instantiate `quotawire.Adapter` from Task 6 and call its `Quota` method directly.
- Calls `Compute.All(ctx, profiles)`.
- Renders a tabwriter table with columns: PROFILE, PLAN, 5H WINDOW, WEEKLY WINDOW, TOKENS 24H, USD 30D.

Example skeleton (the implementation agent fills in the details to match the existing usage.go style):

```go
cmd.Flags().BoolVar(&showQuota, "quota", false, "show plan-aware quota windows alongside token usage")

// inside RunE:
if showQuota {
    return renderUsageQuota(ctx, deps, c.OutOrStdout())
}
```

`renderUsageQuota` should:

1. List profiles.
2. For each profile, call `quota.Compute.For` and also the existing usage aggregation (`store.QueryUsage`) for the TOKENS 24H column.
3. Print a tabwriter table. When `Cap == 0`, render `—`. When `Pct >= 100`, append `⛔`.

- [ ] **Step 4: Verify the test passes**

```bash
gofumpt -w internal/cli/usage.go
go test -race -count=1 ./internal/cli/...
golangci-lint run ./internal/cli/...
```

Expected: tests pass, lint clean.

- [ ] **Step 5: Manual smoke**

(Only if a real `~/.ccx/state.db` and at least one profile exist on your machine.)

```bash
go build -o /tmp/ccx ./cmd/ccx
/tmp/ccx usage --quota
```

Expected: a table is printed; profiles without `plan_tier` show `—` in the window columns.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/usage.go internal/cli/usage_test.go
git commit -m "feat(cli): ccx usage --quota prints plan-aware windows"
```

---

## Task 8: Add `web/components/quota-panel.tsx` (TDD)

**Files:**
- Create: `web/components/quota-panel.tsx`
- Create: `web/components/quota-panel.test.tsx`
- Modify: `web/lib/api.ts` (add `getQuota()` and `ProfileQuota` type re-export)
- Modify: `web/mocks/handlers.ts` (or wherever MSW handlers live) to serve `/api/quota`

- [ ] **Step 1: Add the API client and MSW handler**

In `web/lib/api.ts` (the existing API client wrapper), add:

```ts
import type { components, operations } from './api-types';

export type ProfileQuota = components['schemas']['ProfileQuota'];
export type QuotaWindow  = components['schemas']['QuotaWindow'];

export async function getQuota(params?: { profile?: string }): Promise<ProfileQuota[]> {
  const url = new URL('/api/quota', window.location.origin);
  if (params?.profile) url.searchParams.set('profile', params.profile);
  const r = await fetch(url.toString());
  if (!r.ok) throw new Error(`getQuota: ${r.status}`);
  return r.json();
}
```

(Adjust the exact pattern to match `getProfiles`, `getHeadroom`, etc. already in this file.)

In `web/mocks/handlers.ts`, add a handler returning a stub `ProfileQuota[]`:

```ts
http.get('/api/quota', () => {
  return HttpResponse.json([
    {
      profile: 'work',
      plan_tier: 'max20',
      window_5h:    { used: 142, cap: 900, pct: 15.78, resets_at: new Date(Date.now() + 3_600_000).toISOString() },
      window_weekly: { used: 1203, cap: 4500, pct: 26.73, resets_at: new Date(Date.now() + 24*3_600_000).toISOString() },
    },
    {
      profile: 'personal',
      plan_tier: 'pro',
      window_5h:    { used: 45, cap: 45, pct: 100, resets_at: new Date(Date.now() + 1_200_000).toISOString() },
      window_weekly: { used: 0, cap: 0, pct: 0, resets_at: new Date(0).toISOString() },
    },
  ]);
}),
```

- [ ] **Step 2: Write failing component test**

Create `web/components/quota-panel.test.tsx`:

```tsx
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QuotaPanel } from './quota-panel';
import type { ProfileQuota } from '@/lib/api';

const rows: ProfileQuota[] = [
  {
    profile: 'work', plan_tier: 'max20',
    window_5h: { used: 142, cap: 900, pct: 15.78, resets_at: '2026-05-24T19:00:00Z' },
    window_weekly: { used: 1203, cap: 4500, pct: 26.73, resets_at: '2026-05-31T00:00:00Z' },
  },
  {
    profile: 'personal', plan_tier: 'pro',
    window_5h: { used: 45, cap: 45, pct: 100, resets_at: '2026-05-24T18:50:00Z' },
    window_weekly: { used: 0, cap: 0, pct: 0, resets_at: '1970-01-01T00:00:00Z' },
  },
];

describe('QuotaPanel', () => {
  it('renders one row per profile', () => {
    render(<QuotaPanel quotas={rows} />);
    expect(screen.getByText('work')).toBeInTheDocument();
    expect(screen.getByText('personal')).toBeInTheDocument();
  });

  it('shows used/cap labels for windows with non-zero cap', () => {
    render(<QuotaPanel quotas={rows} />);
    expect(screen.getByText(/142\s*\/\s*900/)).toBeInTheDocument();
    expect(screen.getByText(/1203\s*\/\s*4500/)).toBeInTheDocument();
  });

  it('shows em-dash for windows with cap=0', () => {
    render(<QuotaPanel quotas={rows} />);
    // personal weekly cap is 0 → render as em-dash. Anchor on the per-profile
    // <li>, not the panel <section> (the panel always contains em-dashes from
    // other rows, so closest('section') would pass spuriously).
    const row = screen.getByText('personal').closest('li');
    if (!row) throw new Error('expected personal row');
    expect(row.textContent).toMatch(/—/);
  });

  it('highlights profiles at hard cap', () => {
    const { container } = render(<QuotaPanel quotas={rows} />);
    // Some indication of hard cap on `personal` (class, aria-label, etc.).
    expect(container.textContent).toMatch(/personal/);
    expect(container.textContent).toMatch(/100/);
  });

  it('renders explicit "no plan tier" label for profiles with empty tier', () => {
    // Verify the dedicated empty-tier copy (NOT a coincidental substring
    // match against component framing). The label must be present in the
    // profile-name region of the row, not the panel header.
    render(
      <QuotaPanel quotas={[{
        profile: 'api-dev', plan_tier: '',
        window_5h: { used: 0, cap: 0, pct: 0, resets_at: '1970-01-01T00:00:00Z' },
        window_weekly: { used: 0, cap: 0, pct: 0, resets_at: '1970-01-01T00:00:00Z' },
      }]} />,
    );
    const row = screen.getByText('api-dev').closest('li');
    if (!row) throw new Error('expected api-dev row');
    expect(row.textContent).toMatch(/no plan tier/i);
    // And both windows render em-dash only (no used/cap numbers).
    expect(row.textContent).toContain('—');
    expect(row.textContent).not.toMatch(/\d+\s*\/\s*\d+/);
  });
});
```

- [ ] **Step 3: Run, confirm fail**

```bash
cd web
pnpm test quota-panel
```

Expected: FAIL — component does not exist.

- [ ] **Step 4: Implement the component**

Create `web/components/quota-panel.tsx`. Match the visual style of existing panels (`profile-cards.tsx`, `recommendation-panel.tsx`). One row per profile with two horizontal progress bars (5h, weekly), colored by pct:

- `< 75` → green
- `< 90` → yellow
- `< 100` → orange
- `>= 100` → red (with a small ⛔ or similar indicator)

Skeleton (the implementation agent fills in tailwind classes and exact JSX):

```tsx
'use client';

import type { ProfileQuota, QuotaWindow } from '@/lib/api';

export function QuotaPanel({ quotas }: { quotas: ProfileQuota[] }) {
  if (quotas.length === 0) return null;
  return (
    <section className="rounded-xl border border-card-border bg-card p-4">
      <h2 className="mb-3 text-sm font-medium uppercase tracking-wide text-muted">Plan Quota</h2>
      <ul className="space-y-3">
        {quotas.map((q) => (
          <QuotaRow key={q.profile} quota={q} />
        ))}
      </ul>
    </section>
  );
}

function QuotaRow({ quota }: { quota: ProfileQuota }) {
  return (
    <li className="flex flex-col gap-2 sm:flex-row sm:items-center">
      <div className="w-40 shrink-0">
        <div className="font-medium">{quota.profile}</div>
        <div className="text-xs text-muted">{quota.plan_tier || 'no plan tier'}</div>
      </div>
      <div className="flex-1 space-y-1">
        <Bar label="5h"  window={quota.window_5h} />
        <Bar label="7d"  window={quota.window_weekly} />
      </div>
    </li>
  );
}

function Bar({ label, window }: { label: string; window: QuotaWindow }) {
  if (window.cap === 0) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted">
        <span className="w-8 font-mono">{label}</span>
        <span>—</span>
      </div>
    );
  }
  const pct = Math.min(100, Math.round(window.pct));
  const color =
    pct >= 100 ? 'bg-red-500' :
    pct >=  90 ? 'bg-orange-500' :
    pct >=  75 ? 'bg-yellow-500' :
                 'bg-green-500';
  return (
    <div className="flex items-center gap-2">
      <span className="w-8 font-mono text-xs text-muted">{label}</span>
      <div className="relative h-2 flex-1 overflow-hidden rounded-full bg-grid">
        <div className={`absolute inset-y-0 left-0 ${color}`} style={{ width: `${pct}%` }} />
      </div>
      <span className="w-28 text-right font-mono text-xs">
        {window.used} / {window.cap} ({pct}%){pct >= 100 ? ' ⛔' : ''}
      </span>
    </div>
  );
}
```

- [ ] **Step 5: Verify test passes**

```bash
pnpm test quota-panel
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd ..
git add web/components/quota-panel.tsx web/components/quota-panel.test.tsx web/lib/api.ts web/mocks/handlers.ts
git commit -m "feat(web): QuotaPanel component and getQuota API client"
```

---

## Task 9: Wire QuotaPanel into `dashboard.tsx`

**Files:**
- Modify: `web/components/dashboard.tsx`
- Modify: `web/components/dashboard.test.tsx`

- [ ] **Step 1: Add failing test**

Append to `web/components/dashboard.test.tsx`:

```tsx
it('renders the quota panel when quotas are loaded', async () => {
  // Standard render-with-MSW pattern used elsewhere in this file.
  render(<Dashboard />);
  expect(await screen.findByText(/plan quota/i)).toBeInTheDocument();
});
```

(If the file uses a different pattern, mimic it.)

- [ ] **Step 2: Run, confirm fail**

```bash
cd web && pnpm test dashboard
```

Expected: FAIL — `/plan quota/i` not in DOM.

- [ ] **Step 3: Update dashboard.tsx**

In `web/components/dashboard.tsx`:

1. Add imports:
   ```ts
   import { QuotaPanel } from './quota-panel';
   import { getQuota, type ProfileQuota } from '@/lib/api';
   ```

2. Add state:
   ```ts
   const [quotas, setQuotas] = useState<ProfileQuota[]>([]);
   ```

3. In `refreshAll` (the existing function near the top), add `getQuota()` to the `Promise.all` block and set state when it resolves.

4. In `refreshLiveMetadata` (the existing periodic-refresh function), same addition.

5. In the JSX, insert `<QuotaPanel quotas={quotas} />` between `<ProfileCards .../>` and `<RecommendationPanel ... />`.

- [ ] **Step 4: Verify**

```bash
pnpm test dashboard
pnpm typecheck
pnpm test  # full suite
```

Expected: all green.

- [ ] **Step 5: Commit**

```bash
cd ..
git add web/components/dashboard.tsx web/components/dashboard.test.tsx
git commit -m "feat(web): mount QuotaPanel in dashboard layout"
```

---

## Task 10: Final verification + manual smoke

**Files:** none modified.

- [ ] **Step 1: Full CI gate**

```bash
make ci
```

Expected: lint clean, all tests pass with `-race -count=1`.

- [ ] **Step 2: Web side**

```bash
cd web
pnpm typecheck
pnpm test
cd ..
```

Expected: all green.

- [ ] **Step 3: Build with staged web assets**

```bash
cd web && pnpm build && cd ..
make stage-web
make build
```

Expected: `dist/ccx` produced.

- [ ] **Step 4: End-to-end manual smoke**

(Requires a real profile and at least one Claude Code session under it with hooks installed.)

```bash
./dist/ccx profile set work --plan-tier max20
./dist/ccx dashboard --no-open &
DASH_PID=$!
sleep 2

curl -s http://127.0.0.1:7777/api/quota | jq .
# Expected: array of ProfileQuota objects; the `work` profile shows used > 0
# if any Stop hook events exist in the last 5h.

curl -s 'http://127.0.0.1:7777/api/quota?profile=work' | jq .
# Expected: just the work row.

./dist/ccx usage --quota
# Expected: tabular output with PLAN / 5H WINDOW / WEEKLY WINDOW columns.

kill $DASH_PID
```

- [ ] **Step 5: Inspect the final commit list**

```bash
git log --oneline origin/main..HEAD
```

Expected commits (order may vary slightly):

```
feat(web): mount QuotaPanel in dashboard layout
feat(web): QuotaPanel component and getQuota API client
feat(cli): ccx usage --quota prints plan-aware windows
feat(quotawire,daemon,cli): wire quotawire.Adapter into the server
feat(server): add QuotaProvider dep and /api/quota handler
feat(quota): Computer.For and Computer.All
feat(storage): add QueryTurnsInWindow and QueryOldestTurnInWindow
feat(quota): window-bounds, reset-time, and pct helpers
feat(quota): create package with plan-tier default caps
```

- [ ] **Step 6: Push and open PR**

```bash
git push -u origin feat/quota-analytics
gh pr create \
  --base main \
  --title "feat(quota): plan-aware analytics surface (v0.2 B1)" \
  --body "$(cat <<'EOF'
## Summary

Adds plan-aware quota tracking visible in CLI + HTTP + dashboard. No behavior change yet (B2 brings pressure into ccx suggest; B3a/B3b add auto-switching).

- New leaf package `internal/quota/` owns plan-tier defaults and window math.
- `*storage.Store` gains `QueryTurnsInWindow` and `QueryOldestTurnInWindow` (no schema change).
- `internal/server` gains `QuotaProvider` dep + `GET /api/quota` handler.
- `ccx usage --quota` prints per-profile windows.
- New `<QuotaPanel>` dashboard component slotted above `<RecommendationPanel>`.

Spec: `docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`
Plan: `docs/superpowers/plans/2026-05-24-ccx-quota-B1-analytics.md`

## Test plan

- [x] `make ci` green
- [x] `pnpm test` and `pnpm typecheck` green
- [x] Manual: `curl http://127.0.0.1:7777/api/quota` returns rows after `ccx dashboard --no-open`
- [x] Manual: `ccx usage --quota` prints tabular output
EOF
)"
```

- [ ] **Step 7: After merge, update plan index status**

Edit `docs/superpowers/plans/2026-05-24-ccx-quota-plan-index.md` on a follow-up branch and mark the **B1** row's **Status** column `✅ Merged in #<PR-number>`.

---

## Verification criteria (definition of done)

A successful B1 leaves the repo in this state:

1. **`internal/quota/`** exists and exports `DefaultCaps`, `Window5hBounds`, `WindowWeeklyBounds`, `ResetTime5h`, `ResetTimeWeekly`, `ResetTimeWeeklyAnchored`, `Pct`, `Store` interface, `Computer{For, All}`.

2. **`*storage.Store`** has new methods `QueryTurnsInWindow` and `QueryOldestTurnInWindow` that count `event_name IN ('Stop','StopFailure')` rows.

3. **`GET /api/quota`** returns 200 with a `[]ProfileQuota` body. Empty profile list returns `[]`, not `null`. Unknown profile in filter returns `[]`. Missing `Quota` dep returns 500 with `{"error": "..."}`.

4. **`ccx usage --quota`** prints a tabular output including columns PROFILE, PLAN, 5H WINDOW, WEEKLY WINDOW, TOKENS 24H, USD 30D. Em-dash for windows with `cap == 0`. ⛔ marker for windows at `pct >= 100`.

5. **Dashboard** renders a `<QuotaPanel>` between `<ProfileCards>` and `<RecommendationPanel>`. Each profile has two color-coded progress bars (5h, 7d). Colors: green `<75`, yellow `<90`, orange `<100`, red `>=100`.

6. **Tests:** every new public function has at least one unit test. The handler has table-driven test for: all profiles, single-profile filter, missing dep → 500. The web component has tests for: row count, used/cap labels, em-dash, hard-cap marker, empty-plan-tier empty state.

7. **No frozen files modified.** `git diff origin/main..HEAD --name-only | grep -E '^(internal/contracts/|internal/storage/schema.sql$|api/openapi.yaml$|docs/conventions.md$)'` returns nothing.

8. **No CGo deps added.** `git diff origin/main..HEAD go.mod | grep -E 'mattn|sqlite3'` returns nothing.

9. **PR merged to `main`** with green CI. Plan index updated.

If any criterion fails, B1 is not done.

---

## Rollback

If B1 ships and is judged wrong (e.g., counting `StopFailure` as a turn is incorrect):

1. Open a small follow-up PR that adjusts `internal/storage/turns.go` — change the SQL `event_name IN (...)` clause to whatever the new policy says. This is one commit, no contract change.
2. If the panel UX is the problem: another small PR adjusts `quota-panel.tsx`. No contract change.

If the new endpoint must be removed entirely:

1. New PR removes `internal/server/quota.go`, the `Quota` field on `Deps`, the route registration, and the wiring. Leaves `internal/quota/` package in place (downstream phases B2/B3a/B3b will use it).
2. If `/api/quota` was already merged in P0's openapi.yaml, that route declaration stays for now — implementing handlers later is fine.

This phase introduces no schema migration and no contract additions of its own, so rollback is mechanical.
