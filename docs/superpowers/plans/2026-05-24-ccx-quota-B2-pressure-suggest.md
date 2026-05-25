# ccx v0.2 B2 — Pressure-Aware Suggest Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fold plan-aware quota pressure into the existing `ccx suggest` ranking. After this phase, profiles near their 5h or weekly cap rank lower; profiles at hard cap are excluded with a cooldown-until timestamp. The existing token-budget, priority, auth-health, and recent-failure signals stay in place — quota pressure becomes one more dimension in the same scoring formula.

**Architecture:**

- **Thresholds live as constants** in `internal/headroom/thresholds.go` — `Warn=0.75`, `Soft=0.90`, `Hard=1.00`. Editable in a single file; not exposed in `ProfileLimits` per spec §6.4.
- **`headroom.Store` interface grows two methods**: `QueryTurnsInWindow` and `QueryOldestTurnInWindow` (already implemented on `*storage.Store` in B1).
- **`headroom.Candidate` gains** `Quota5h` and `QuotaWeekly` `*contracts.QuotaWindow` fields (pointer so JSON omits them when nil — i.e., when the profile has no `PlanTier`).
- **`headroomPercent`** is extended to fold per-cap pressure into the existing `min()` over budgets. Pressure dimensions only contribute when the profile has caps configured.
- **A new gate function** `applyQuotaGates` runs alongside the existing `applyFailureGates` to set `Available=false` and a `CooldownUntil` when a profile is at `pct >= 100`.
- **A new scoring term** subtracts a linear penalty for `90 <= pct < 100` (`penalty = (pct − 90) × 2`, capped at 20).
- **CLI rendering**: `internal/cli/suggest.go::renderSuggest` is updated to print the quota windows in the candidate table and as Reason strings.

**Tech Stack:** Go 1.22+ stdlib only.

**Spec reference:** [`docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`](../specs/2026-05-24-ccx-plan-aware-quota.md) — §6.4 (tiered thresholds), §6.5 (advisory vs auto-switch), §8.3 (`ccx suggest` output mockup).

**Worktree:**

```bash
git fetch origin
git worktree add ../ccx-quota-pressure -b feat/quota-pressure-suggest origin/main
cd ../ccx-quota-pressure
```

B1 must already be merged before this worktree is created — the `*storage.Store` methods and `internal/quota` helpers this plan uses live in B1.

**Exit criteria:**

- [ ] `go build ./...` succeeds
- [ ] `go test -race -count=1 ./...` succeeds
- [ ] `golangci-lint run ./...` reports `0 issues.`
- [ ] `cd web && pnpm typecheck && pnpm test` succeed (existing tests still pass after `HeadroomCandidate` gains optional fields)
- [ ] `make ci` green
- [ ] Manual smoke: `./dist/ccx suggest --json` shows `quota_5h`/`quota_weekly` on profiles with a `plan_tier`, and Reason strings include `"5h turns N/M"`
- [ ] Manual smoke: a profile with `Caps5hTurns: 1` and one `Stop` event becomes `available: false` in suggest output
- [ ] PR opened against `main`, CI green, merged
- [ ] Plan index status updated

**Conventions:**

- Same as v0.1: tabs, gofumpt, conventional commits, exported-symbol doc comments, one commit per task.
- Scopes: `headroom`, `cli`.
- Do not edit `internal/contracts/`, `internal/storage/schema.sql`, `api/openapi.yaml`, or `docs/conventions.md`.

---

## Pre-flight

```bash
pwd                                                  # → .../ccx-quota-pressure
git status                                           # → On branch feat/quota-pressure-suggest, working tree clean
grep -l "QueryTurnsInWindow" internal/storage/turns.go && echo OK   # → OK (B1 merged)
grep -l "func DefaultCaps" internal/quota/plans.go && echo OK       # → OK (B1 merged)
go build ./... && echo OK
```

If `internal/quota/plans.go` is missing, **stop** — B1 hasn't merged.

---

## Task 1: Add threshold constants (TDD)

**Files:**
- Create: `internal/headroom/thresholds.go`
- Create: `internal/headroom/thresholds_test.go`

- [ ] **Step 1: Failing test**

Create `internal/headroom/thresholds_test.go`:

```go
package headroom_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/headroom"
)

func TestPressureLevelFromPct(t *testing.T) {
	cases := []struct {
		pct  float64
		want headroom.PressureLevel
	}{
		{0, headroom.PressureNone},
		{74.9, headroom.PressureNone},
		{75, headroom.PressureWarn},
		{89.9, headroom.PressureWarn},
		{90, headroom.PressureSoft},
		{99.9, headroom.PressureSoft},
		{100, headroom.PressureHard},
		{150, headroom.PressureHard},
	}
	for _, tc := range cases {
		got := headroom.PressureLevelFromPct(tc.pct)
		if got != tc.want {
			t.Errorf("PressureLevelFromPct(%v) = %v, want %v", tc.pct, got, tc.want)
		}
	}
}

func TestSoftPenalty(t *testing.T) {
	cases := []struct {
		pct  float64
		want float64
	}{
		{0, 0},
		{75, 0},
		{89, 0},
		{90, 0},
		{95, 10},
		{99, 18},
		{100, 20}, // capped at 20
		{200, 20},
	}
	for _, tc := range cases {
		got := headroom.SoftPenalty(tc.pct)
		if got != tc.want {
			t.Errorf("SoftPenalty(%v) = %v, want %v", tc.pct, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run, confirm fail**

```bash
go test -race -count=1 ./internal/headroom/...
```

Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implementation**

Create `internal/headroom/thresholds.go`:

```go
package headroom

// PressureLevel categorizes a profile's quota pressure into one of four bands.
// Boundaries are defined by the threshold constants below.
type PressureLevel int

const (
	// PressureNone means the profile is below the warn threshold.
	PressureNone PressureLevel = iota
	// PressureWarn means the profile crossed the warn threshold but is still
	// below the soft-penalty threshold. No score impact.
	PressureWarn
	// PressureSoft means the profile is between the soft-penalty and hard
	// thresholds. Score is reduced by SoftPenalty.
	PressureSoft
	// PressureHard means the profile is at or above the hard cap. Profile is
	// marked Available=false until the relevant window resets.
	PressureHard
)

// Threshold pct values that segment the four pressure bands.
const (
	ThresholdWarnPct = 75.0
	ThresholdSoftPct = 90.0
	ThresholdHardPct = 100.0
)

// SoftPenaltyMax is the maximum score penalty applied in the soft band.
const SoftPenaltyMax = 20.0

// PressureLevelFromPct returns the band a pressure percentage falls into.
func PressureLevelFromPct(pct float64) PressureLevel {
	switch {
	case pct >= ThresholdHardPct:
		return PressureHard
	case pct >= ThresholdSoftPct:
		return PressureSoft
	case pct >= ThresholdWarnPct:
		return PressureWarn
	default:
		return PressureNone
	}
}

// SoftPenalty returns the linear score penalty for the soft band. Below 90%
// or at/above 100%: returns the boundary value (0 or SoftPenaltyMax).
func SoftPenalty(pct float64) float64 {
	if pct < ThresholdSoftPct {
		return 0
	}
	if pct >= ThresholdHardPct {
		return SoftPenaltyMax
	}
	p := (pct - ThresholdSoftPct) * 2
	if p > SoftPenaltyMax {
		return SoftPenaltyMax
	}
	return p
}
```

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/headroom/
go test -race -count=1 ./internal/headroom/...
golangci-lint run ./internal/headroom/...
```

Expected: tests pass, lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/headroom/thresholds.go internal/headroom/thresholds_test.go
git commit -m "feat(headroom): pressure level bands and soft penalty"
```

---

## Task 2: Extend `headroom.Store` interface with turn-query methods

**Files:**
- Modify: `internal/headroom/evaluator.go`

- [ ] **Step 1: Add the methods to the interface**

In `internal/headroom/evaluator.go`, find the `Store` interface (around line 21–27). Add two methods:

```go
type Store interface {
	QueryUsage(ctx context.Context, q contracts.UsageQuery) ([]contracts.UsageRow, error)
	QuerySessions(ctx context.Context, q contracts.SessionQuery) ([]contracts.SessionTelemetry, error)
	QueryRecentFailures(ctx context.Context, profileName string, since time.Time) ([]contracts.HookEvent, error)
	GetProfileHealth(ctx context.Context, profileName string) (contracts.ProfileHealth, error)

	// QueryTurnsInWindow returns the number of completed turns (Stop or
	// StopFailure hook events) for a profile in the given interval.
	QueryTurnsInWindow(ctx context.Context, profileName string, since, until time.Time) (int, error)

	// QueryOldestTurnInWindow returns the timestamp of the earliest turn for
	// a profile inside the interval, or the zero time if none.
	QueryOldestTurnInWindow(ctx context.Context, profileName string, since, until time.Time) (time.Time, error)
}
```

`*storage.Store` already satisfies these via the methods added in B1.

- [ ] **Step 2: Extend `fakeStore` in `evaluator_test.go` to satisfy the new interface**

Adding the two methods to `headroom.Store` immediately breaks `internal/headroom/evaluator_test.go:363` where `fakeStore` is defined — the test file will fail to compile because `fakeStore` no longer satisfies the interface. We extend `fakeStore` with:

1. A `turns []turnEvent` slice and an `addTurn(profile string, at time.Time)` setter (parallel to the existing `addUsage`).
2. Two interface methods that filter the slice by profile + time range.

Open `internal/headroom/evaluator_test.go`. **After** the existing `usageEvent` struct (around line 372) and **before** `func newFakeStore`, insert:

```go
type turnEvent struct {
	profile string
	at      time.Time
}
```

In the `fakeStore` struct (line 363–369), add the `turns` field:

```go
type fakeStore struct {
	now      time.Time
	usage    []usageEvent
	turns    []turnEvent
	failures map[string][]contracts.HookEvent
	sessions map[string][]contracts.SessionTelemetry
	health   map[string]contracts.ProfileHealth
}
```

**After** the existing `addWorkSession` method (around line 399), insert the new methods:

```go
func (s *fakeStore) addTurn(profile string, at time.Time) {
	s.turns = append(s.turns, turnEvent{profile: profile, at: at})
}

func (s *fakeStore) QueryTurnsInWindow(_ context.Context, profile string, since, until time.Time) (int, error) {
	n := 0
	for _, t := range s.turns {
		if t.profile != profile {
			continue
		}
		if t.at.Before(since) || t.at.After(until) {
			continue
		}
		n++
	}
	return n, nil
}

func (s *fakeStore) QueryOldestTurnInWindow(_ context.Context, profile string, since, until time.Time) (time.Time, error) {
	var oldest time.Time
	for _, t := range s.turns {
		if t.profile != profile {
			continue
		}
		if t.at.Before(since) || t.at.After(until) {
			continue
		}
		if oldest.IsZero() || t.at.Before(oldest) {
			oldest = t.at
		}
	}
	return oldest, nil
}
```

- [ ] **Step 3: Confirm builds + existing tests still pass**

```bash
go build ./...
go test -race -count=1 ./internal/headroom/...
```

Expected: green. The new methods aren't called by any existing test; the change is purely interface-satisfaction.

- [ ] **Step 4: Commit**

```bash
git add internal/headroom/evaluator.go internal/headroom/evaluator_test.go
git commit -m "feat(headroom): add QueryTurnsInWindow/QueryOldestTurnInWindow to Store interface"
```

---

## Task 3: Extend `Candidate` with optional quota fields and `Pricing` no-op when caps-only (TDD)

**Files:**
- Modify: `internal/headroom/evaluator.go`
- Modify: `internal/headroom/evaluator_test.go`

- [ ] **Step 1: Add a test-helper for seeding turns at specific window positions**

The repo's existing `evaluate(t, store, profiles)` helper (defined at
`evaluator_test.go:307`) sets `Evaluator.Now: func() time.Time { return store.now }`,
so seeding turns relative to `store.now` works cleanly. Append a small helper
to `evaluator_test.go` (near the other helpers):

```go
// seedTurns adds count5h turns within the last 5h and (countWeekly - count5h)
// additional turns between -5h and -7d. Use to pin specific Used values per
// window in pressure tests. Requires fakeStore.now to be set.
func seedTurns(s *fakeStore, profile string, count5h, countWeekly int) {
	for i := 0; i < count5h; i++ {
		// Spread 1 minute apart so the oldest-in-window is deterministic.
		s.addTurn(profile, s.now.Add(-time.Duration(i+1)*time.Minute))
	}
	extra := countWeekly - count5h
	for i := 0; i < extra; i++ {
		s.addTurn(profile, s.now.Add(-6*time.Hour-time.Duration(i+1)*time.Minute))
	}
}
```

- [ ] **Step 2: Failing test**

Append to `internal/headroom/evaluator_test.go`:

```go
func TestEvaluatePopulatesQuotaFieldsWhenPlanTierSet(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	seedTurns(store, "work", 142, 1203)

	result := evaluate(t, store, []contracts.Profile{
		profile("work", contracts.ProfileLimits{PlanTier: "max20"}),
	})
	c := mustCandidate(t, result, "work")
	if c.Quota5h == nil {
		t.Fatal("Quota5h should be populated when PlanTier is set")
	}
	if c.Quota5h.Used != 142 || c.Quota5h.Cap != 900 {
		t.Errorf("Quota5h: got %+v, want Used=142 Cap=900", *c.Quota5h)
	}
	if c.QuotaWeekly == nil || c.QuotaWeekly.Used != 1203 {
		t.Errorf("QuotaWeekly: got %+v, want Used=1203", c.QuotaWeekly)
	}
}

func TestEvaluateOmitsQuotaFieldsWhenNoPlanTier(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)

	result := evaluate(t, store, []contracts.Profile{
		profile("api-dev", contracts.ProfileLimits{}),
	})
	if result.Candidates[0].Quota5h != nil {
		t.Errorf("Quota5h should be nil when PlanTier is empty")
	}
	if result.Candidates[0].QuotaWeekly != nil {
		t.Errorf("QuotaWeekly should be nil when PlanTier is empty")
	}
}
```

- [ ] **Step 3: Run, confirm fail**

Expected: FAIL — `Candidate.Quota5h` undefined.

- [ ] **Step 4: Implementation**

In `internal/headroom/evaluator.go`, modify the `Candidate` struct to add two pointer fields:

```go
type Candidate struct {
	Profile         string                 `json:"profile"`
	Available       bool                   `json:"available"`
	Score           float64                `json:"score"`
	HeadroomPercent float64                `json:"headroom_percent"`
	AuthStatus      string                 `json:"auth_status"`
	CooldownUntil   *time.Time             `json:"cooldown_until,omitempty"`
	Reasons         []string               `json:"reasons"`
	Priority        int                    `json:"priority"`
	Tokens24h       int                    `json:"tokens_24h"`
	Tokens7d        int                    `json:"tokens_7d"`
	USD30d          float64                `json:"usd_30d"`
	Quota5h         *contracts.QuotaWindow `json:"quota_5h,omitempty"`
	QuotaWeekly    *contracts.QuotaWindow `json:"quota_weekly,omitempty"`
}
```

Then, in `evaluateProfile`, after the existing `usage` block and before the `Reasons` are constructed, add:

```go
// Compute quota windows for profiles with a configured PlanTier. Empty tier
// leaves Quota5h / QuotaWeekly nil, omitting them from JSON.
if p.Limits.PlanTier != "" {
	q5h, qWeekly, err := e.computeQuota(ctx, p, now)
	if err != nil {
		return Candidate{}, err
	}
	c.Quota5h = &q5h
	c.QuotaWeekly = &qWeekly
}
```

Add a private helper method on `Evaluator`:

```go
func (e Evaluator) computeQuota(ctx context.Context, p *contracts.Profile, now time.Time) (contracts.QuotaWindow, contracts.QuotaWindow, error) {
	computer := quota.Computer{Store: e.Store, Now: func() time.Time { return now }}
	pq, err := computer.For(ctx, *p)
	if err != nil {
		return contracts.QuotaWindow{}, contracts.QuotaWindow{}, err
	}
	return pq.Window5h, pq.WindowWeekly, nil
}
```

Add the import: `"github.com/arafa-dev/ccx/internal/quota"`.

`headroom.Store` already satisfies `quota.Store` because both interfaces require the same two methods (added in Task 2). If the type system complains, add `// compile-time assertion` to make the relationship explicit:

```go
var _ quota.Store = (Store)(nil) // does not compile because Store is an interface — see note
```

If interface-satisfying-interface is awkward, embed the relevant methods directly:

```go
type quotaSourceAdapter struct{ store Store }

func (a quotaSourceAdapter) QueryTurnsInWindow(ctx context.Context, p string, s, u time.Time) (int, error) {
	return a.store.QueryTurnsInWindow(ctx, p, s, u)
}
func (a quotaSourceAdapter) QueryOldestTurnInWindow(ctx context.Context, p string, s, u time.Time) (time.Time, error) {
	return a.store.QueryOldestTurnInWindow(ctx, p, s, u)
}
```

And use `quotaSourceAdapter{e.Store}` as the `quota.Computer.Store`.

- [ ] **Step 5: Verify**

```bash
gofumpt -w internal/headroom/
go test -race -count=1 ./internal/headroom/...
golangci-lint run ./internal/headroom/...
```

Expected: tests pass, lint clean.

- [ ] **Step 6: Commit**

```bash
git add internal/headroom/evaluator.go internal/headroom/evaluator_test.go
git commit -m "feat(headroom): populate Candidate quota fields when plan tier configured"
```

---

## Task 4: Fold quota pressure into `headroomPercent` (TDD)

**Files:**
- Modify: `internal/headroom/evaluator.go`
- Modify: `internal/headroom/evaluator_test.go`

- [ ] **Step 1: Failing test**

Append to `evaluator_test.go`:

```go
func TestHeadroomPercentIncludesQuota(t *testing.T) {
	// Profile at 80% 5h pressure → headroom = 20%.
	// Compare to profile at 20% pressure → headroom = 80%.
	now := testNow()
	store := newFakeStore(now)
	seedTurns(store, "hot",  720, 720) // 720/900 = 80%
	seedTurns(store, "cold", 180, 180) // 180/900 = 20%

	result := evaluate(t, store, []contracts.Profile{
		profile("hot",  contracts.ProfileLimits{PlanTier: "max20"}),
		profile("cold", contracts.ProfileLimits{PlanTier: "max20"}),
	})
	hot  := mustCandidate(t, result, "hot")
	cold := mustCandidate(t, result, "cold")
	if hot.HeadroomPercent >= cold.HeadroomPercent {
		t.Errorf("hot %v >= cold %v; quota pressure should lower hot's headroom",
			hot.HeadroomPercent, cold.HeadroomPercent)
	}
}
```

(`mustCandidate` already exists at `evaluator_test.go:332`; no need to add `candidateByName`.)

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL — `headroomPercent` ignores quota pressure today.

- [ ] **Step 3: Update `headroomPercent`**

In `internal/headroom/evaluator.go`, the existing `headroomPercent(limits, usage)` takes only token-budget arguments. Extend it (or wrap it) to fold quota windows when they are present.

Easiest path: do not modify the existing function signature. Instead, after `c.HeadroomPercent = headroomPercent(p.Limits, usage)` in `evaluateProfile`, add:

```go
if c.Quota5h != nil {
    if h := 100 - c.Quota5h.Pct; h < c.HeadroomPercent {
        c.HeadroomPercent = round2(h)
    }
}
if c.QuotaWeekly != nil {
    if h := 100 - c.QuotaWeekly.Pct; h < c.HeadroomPercent {
        c.HeadroomPercent = round2(h)
    }
}
```

This preserves the existing token-budget behavior and lowers HeadroomPercent further when quota pressure is higher.

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/headroom/
go test -race -count=1 ./internal/headroom/...
golangci-lint run ./internal/headroom/...
```

Expected: tests pass, lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/headroom/evaluator.go internal/headroom/evaluator_test.go
git commit -m "feat(headroom): fold quota pressure into HeadroomPercent"
```

---

## Task 5: Apply soft penalty and hard exclude to scoring (TDD)

**Files:**
- Modify: `internal/headroom/evaluator.go`
- Modify: `internal/headroom/evaluator_test.go`

- [ ] **Step 1: Failing tests**

Append to `evaluator_test.go`:

```go
func TestScoreAppliesSoftPenaltyBetween90And100(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	seedTurns(store, "hot", 855, 855) // 855/900 = 95% → SoftPenalty(95) = 10

	result := evaluate(t, store, []contracts.Profile{
		profile("hot", contracts.ProfileLimits{PlanTier: "max20"}),
	})
	c := mustCandidate(t, result, "hot")
	if !c.Available {
		t.Errorf("at 95%% should still be available; got %+v", c)
	}
	// HeadroomPercent = min(5, ...) = 5; auth-unknown penalty 1; quota soft
	// penalty SoftPenalty(95)=10; Score = 5 + 0 - 0 - 1 - 10 = -6 → negative.
	if c.Score >= 0 {
		t.Errorf("Score = %v, expected negative after soft penalty", c.Score)
	}
}

func TestAvailableFalseAtHardCap(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	seedTurns(store, "capped", 900, 900) // 900/900 = 100% hard cap

	result := evaluate(t, store, []contracts.Profile{
		profile("capped", contracts.ProfileLimits{PlanTier: "max20"}),
	})
	c := mustCandidate(t, result, "capped")
	if c.Available {
		t.Errorf("at 100%% should be unavailable")
	}
	if c.CooldownUntil == nil {
		t.Errorf("CooldownUntil should be set when at hard cap")
	}
}

func TestWeeklyHardCapAlsoExcludes(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	// 10 turns in 5h, plus enough more in the 7d window to hit weekly cap 4500.
	seedTurns(store, "week-capped", 10, 4500)

	result := evaluate(t, store, []contracts.Profile{
		profile("week-capped", contracts.ProfileLimits{
			PlanTier:        "max20",
			CapsWeeklyTurns: 4500,
		}),
	})
	if mustCandidate(t, result, "week-capped").Available {
		t.Errorf("weekly hard cap should make profile unavailable")
	}
}

func TestWarnAddsReasonNoScoreImpact(t *testing.T) {
	now := testNow()
	store := newFakeStore(now)
	seedTurns(store, "warm", 700, 700) // 700/900 ≈ 77.8% (warn band)

	// Baseline: zero-pressure profile for score comparison.
	store2 := newFakeStore(now)
	result := evaluate(t, store, []contracts.Profile{
		profile("warm", contracts.ProfileLimits{PlanTier: "max20"}),
	})
	baselineResult := evaluate(t, store2, []contracts.Profile{
		profile("warm", contracts.ProfileLimits{PlanTier: "max20"}),
	})

	c := mustCandidate(t, result, "warm")
	baseline := mustCandidate(t, baselineResult, "warm")

	// 1) Reason string includes the 5h-turn count line.
	hasReason := false
	for _, r := range c.Reasons {
		if strings.Contains(r, "5h turns") {
			hasReason = true
			break
		}
	}
	if !hasReason {
		t.Errorf("warm profile should have a 5h-turn reason; got %v", c.Reasons)
	}

	// 2) Warn band means NO score penalty — only HeadroomPercent changes.
	//    Both profiles get the auth-unknown penalty equally, so the only
	//    expected difference between warm and baseline is HeadroomPercent.
	expectedDelta := baseline.HeadroomPercent - c.HeadroomPercent
	actualDelta := baseline.Score - c.Score
	if diff := actualDelta - expectedDelta; diff > 0.01 || diff < -0.01 {
		t.Errorf("warn band applied a score penalty (delta %.2f, expected only HeadroomPercent delta %.2f)",
			actualDelta, expectedDelta)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

Expected: failures across the new tests.

- [ ] **Step 3: Implementation**

In `internal/headroom/evaluator.go`, add a new gate function:

```go
// applyQuotaGates computes the highest pressure across 5h and weekly windows
// for the candidate. Side effects:
//   - For Hard (>=100%): sets Available=false and CooldownUntil to the window
//     reset time, and appends a Reason.
//   - For Soft (90-99): appends a Reason. The score penalty is returned to
//     the caller for subtraction.
//   - For Warn (75-89): appends a Reason. Returns 0 penalty.
//   - For None: no side effect, returns 0.
func (e Evaluator) applyQuotaGates(c *Candidate) (penalty float64) {
	// Iterate both windows; the worst level wins.
	type windowInfo struct {
		name string
		w    *contracts.QuotaWindow
	}
	windows := []windowInfo{{"5h", c.Quota5h}, {"weekly", c.QuotaWeekly}}
	worst := PressureNone
	worstPct := 0.0
	for _, wi := range windows {
		if wi.w == nil || wi.w.Cap == 0 {
			continue
		}
		level := PressureLevelFromPct(wi.w.Pct)
		c.Reasons = append(c.Reasons, fmt.Sprintf("%s turns %d/%d (%.0f%%)", wi.name, wi.w.Used, wi.w.Cap, wi.w.Pct))
		if level > worst {
			worst = level
			worstPct = wi.w.Pct
		}
		if level == PressureHard {
			c.Available = false
			if !wi.w.ResetsAt.IsZero() && (c.CooldownUntil == nil || wi.w.ResetsAt.After(*c.CooldownUntil)) {
				resets := wi.w.ResetsAt
				c.CooldownUntil = &resets
			}
		}
	}
	if worst == PressureSoft {
		penalty = SoftPenalty(worstPct)
		c.Reasons = append(c.Reasons, fmt.Sprintf("quota pressure %.0f%% soft penalty %.0f", worstPct, penalty))
	} else if worst == PressureWarn {
		c.Reasons = append(c.Reasons, fmt.Sprintf("quota pressure %.0f%% (warn)", worstPct))
	} else if worst == PressureHard {
		c.Reasons = append(c.Reasons, fmt.Sprintf("quota pressure %.0f%% (hard cap)", worstPct))
	}
	return penalty
}
```

In `evaluateProfile`, after the existing failure-gate block and the headroom-percent calculation, invoke the new gate:

```go
quotaPenalty := e.applyQuotaGates(&c)
```

And update the final score computation to subtract it:

```go
c.Score = c.HeadroomPercent + float64(p.Limits.Priority) - failurePenalty - healthPenalty - quotaPenalty
```

#### Final shape of `evaluateProfile` after Tasks 3–5

Tasks 3, 4, and 5 each insert code at different points of `evaluateProfile`.
The order matters: quota fields must exist (Task 3) before `headroomPercent`
is reduced by them (Task 4), and before the quota gate runs (Task 5). For
reference, the assembled function should read:

```go
func (e Evaluator) evaluateProfile(ctx context.Context, p *contracts.Profile, now time.Time, opts Options) (Candidate, error) {
	c := Candidate{
		Profile:    p.Name,
		Available:  true,
		AuthStatus: "unknown",
		Priority:   p.Limits.Priority,
	}

	// (existing) SuggestEnabled, CheckConfigDir, opts.UnavailableReasons gates
	// ... unchanged ...

	// (existing) profileHealth lookup → c.AuthStatus, auth-fail gate
	// ... unchanged ...

	// (existing) usage aggregation → tokens24h, tokens7d, usd30d
	usage, err := e.usage(ctx, p.Name, now)
	if err != nil {
		return Candidate{}, err
	}
	c.Tokens24h = usage.tokens24h
	c.Tokens7d = usage.tokens7d
	c.USD30d = usage.usd30d

	// (Task 3) Compute quota windows when PlanTier is configured.
	if p.Limits.PlanTier != "" {
		q5h, qWeekly, err := e.computeQuota(ctx, p, now)
		if err != nil {
			return Candidate{}, err
		}
		c.Quota5h = &q5h
		c.QuotaWeekly = &qWeekly
	}

	// (existing) failure gates
	failures, err := e.Store.QueryRecentFailures(ctx, p.Name, now.Add(-failureLookback))
	if err != nil {
		return Candidate{}, fmt.Errorf("querying recent failures for %q: %w", p.Name, err)
	}
	sessions, err := e.recentSessions(ctx, p.Name, now)
	if err != nil {
		return Candidate{}, err
	}
	health, haveHealth, _ := e.profileHealth(ctx, p.Name)
	failurePenalty := e.applyFailureGates(&c, p, failures, sessions, health, haveHealth, now, opts.IncludeUnavailable)

	// (existing) base HeadroomPercent from token budgets
	c.HeadroomPercent = headroomPercent(p.Limits, usage)

	// (Task 4) Fold quota windows into HeadroomPercent.
	if c.Quota5h != nil {
		if h := 100 - c.Quota5h.Pct; h < c.HeadroomPercent {
			c.HeadroomPercent = round2(h)
		}
	}
	if c.QuotaWeekly != nil {
		if h := 100 - c.QuotaWeekly.Pct; h < c.HeadroomPercent {
			c.HeadroomPercent = round2(h)
		}
	}

	// (existing) Reasons composition for budgets/priority/health
	// ... unchanged: budgetReasons, priority reason, auth reason ...

	// (Task 5) Apply quota gates (Available, CooldownUntil, Reasons, penalty).
	quotaPenalty := e.applyQuotaGates(&c)

	healthPenalty := authHealthPenalty(c.AuthStatus)
	c.Score = c.HeadroomPercent + float64(p.Limits.Priority) - failurePenalty - healthPenalty - quotaPenalty
	return c, nil
}
```

**Do not paste this whole function over the existing one.** Use it as a
reference to verify the insertion points landed correctly. The existing
function body (with the unchanged blocks marked above) stays in place; only
the marked sections are new or moved.

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/headroom/
go test -race -count=1 ./internal/headroom/...
golangci-lint run ./internal/headroom/...
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/headroom/evaluator.go internal/headroom/evaluator_test.go
git commit -m "feat(headroom): tiered quota pressure gating and soft penalty"
```

---

## Task 6: Update CLI `suggest` rendering to show quota columns (TDD)

**Files:**
- Modify: `internal/cli/suggest.go`
- Modify: `internal/cli/suggest_test.go`

- [ ] **Step 1: Failing test**

Append to `internal/cli/suggest_test.go`:

```go
func TestRenderSuggestIncludesQuotaColumn(t *testing.T) {
	pct := 16.0
	used := 142
	cap := 900
	resets := time.Now().Add(time.Hour)
	result := headroom.Result{
		Recommendation: &headroom.Candidate{
			Profile:         "work",
			Available:       true,
			Score:           89.5,
			HeadroomPercent: 84.0,
			AuthStatus:      "ok",
			Reasons:         []string{"5h turns 142/900 (16%)"},
			Quota5h:         &contracts.QuotaWindow{Used: used, Cap: cap, Pct: pct, ResetsAt: resets},
		},
		Candidates: []headroom.Candidate{
			{Profile: "work", Available: true, Score: 89.5, HeadroomPercent: 84.0, AuthStatus: "ok",
				Reasons: []string{"5h turns 142/900 (16%)"},
				Quota5h: &contracts.QuotaWindow{Used: used, Cap: cap, Pct: pct, ResetsAt: resets}},
		},
	}
	var buf bytes.Buffer
	if err := renderSuggest(&buf, result); err != nil {
		t.Fatalf("renderSuggest: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"5H", "142/900", "16%"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL.

- [ ] **Step 3: Update `renderSuggest`**

In `internal/cli/suggest.go`, modify the tabwriter header and per-row format. Existing:

```go
_, _ = fmt.Fprintln(tw, "PROFILE\tAVAILABLE\tSCORE\tHEADROOM\tAUTH\tREASONS")
```

Replace with:

```go
_, _ = fmt.Fprintln(tw, "PROFILE\tAVAILABLE\tSCORE\tHEADROOM\t5H\tWEEKLY\tAUTH\tREASONS")
```

Per-row format updates:

```go
fmt.Fprintf(
    tw, "%s\t%t\t%.1f\t%.1f%%\t%s\t%s\t%s\t%s\n",
    c.Profile,
    c.Available,
    c.Score,
    c.HeadroomPercent,
    formatQuotaWindow(c.Quota5h),
    formatQuotaWindow(c.QuotaWeekly),
    c.AuthStatus,
    firstReason(c.Reasons),
)
```

Add the formatter:

```go
func formatQuotaWindow(w *contracts.QuotaWindow) string {
    if w == nil || w.Cap == 0 {
        return "—"
    }
    suffix := ""
    if w.Pct >= 100 {
        suffix = " ⛔"
    }
    return fmt.Sprintf("%d/%d (%.0f%%)%s", w.Used, w.Cap, w.Pct, suffix)
}
```

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/cli/suggest.go
go test -race -count=1 ./internal/cli/...
golangci-lint run ./internal/cli/...
```

Expected: tests pass, lint clean.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/suggest.go internal/cli/suggest_test.go
git commit -m "feat(cli): include 5h/weekly quota columns in suggest output"
```

---

## Task 7: Update server's `handleHeadroom` consumer doesn't break

**Files:**
- Modify: `internal/server/new_endpoints_test.go` (if needed)

- [ ] **Step 1: Verify existing handler tests still pass**

```bash
go test -race -count=1 ./internal/server/...
```

The `handleHeadroom` handler just JSON-encodes `headroom.Result`; the new optional `Quota5h`/`QuotaWeekly` fields are encoded automatically. Existing tests should pass without modification.

If a test asserts a *specific* JSON shape (e.g., a snapshot test) and breaks because the new fields appear when set, update the test fixture to allow them as optional fields. Do not change snapshot files unrelated to this work.

- [ ] **Step 2: Add a focused test that verifies quota fields surface through the handler**

Use a real in-memory `*storage.Store` and a thin `ProfileLister` stub. The
existing `internal/server` test files (e.g. `server_test.go`) already
demonstrate this pattern with `storage.NewStore(ctx, ":memory:")` + a struct
that implements `ProfileLister`. No new fake helpers needed.

Append to `internal/server/new_endpoints_test.go`:

```go
// staticProfileLister is a one-line ProfileLister for handler tests.
type staticProfileLister []contracts.Profile

func (s staticProfileLister) List(context.Context) ([]contracts.Profile, error) {
	return []contracts.Profile(s), nil
}

func TestHandleHeadroomIncludesQuotaWhenPlanTierSet(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	profile := contracts.Profile{
		Name:      "work",
		ConfigDir: t.TempDir(),
		Limits:    contracts.ProfileLimits{PlanTier: "max20"},
		CreatedAt: time.Now().UTC(),
	}
	if err := store.SaveProfile(ctx, profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	// Seed one Stop event so QueryTurnsInWindow returns >0; the handler
	// pipes that through the evaluator which populates Quota5h.
	if err := store.InsertHookEvent(ctx, "work", contracts.HookEvent{
		Profile: "work", Session: "s1", Event: "Stop",
		Timestamp: time.Now().UTC().Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("InsertHookEvent: %v", err)
	}

	srv := server.New(server.Deps{
		Store:    store,
		Profiles: staticProfileLister([]contracts.Profile{profile}),
		Pricing:  fakePricing{},
		Headroom: headroom.Evaluator{Store: store, Pricing: fakePricing{}},
	}, "test")
	req := httptest.NewRequest(http.MethodGet, "/api/headroom", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"quota_5h"`) {
		t.Errorf("expected quota_5h in body:\n%s", rec.Body.String())
	}
}
```

`fakePricing` lives in `internal/headroom/evaluator_test.go`; if this test
file is `package server_test` (different package), copy a one-line equivalent
inline:

```go
type fakePricing map[string]float64
func (p fakePricing) Cost(string, time.Time, contracts.Usage) (float64, error) { return 0, nil }
func (p fakePricing) LastUpdated() time.Time                                    { return time.Time{} }
```

- [ ] **Step 3: Verify**

```bash
go test -race -count=1 ./internal/server/...
```

Expected: tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/server/new_endpoints_test.go
git commit -m "test(server): verify quota fields surface via /api/headroom"
```

---

## Task 8: Final verification + manual smoke

**Files:** none.

- [ ] **Step 1: `make ci`**

```bash
make ci
```

Expected: green.

- [ ] **Step 2: Web side** (no schema changes here, but verify TS still typechecks against the existing P0-generated types)

```bash
cd web
pnpm typecheck
pnpm test
cd ..
```

Expected: green.

- [ ] **Step 3: Manual smoke**

```bash
cd web && pnpm build && cd ..
make stage-web && make build

./dist/ccx profile set work --plan-tier max20
./dist/ccx suggest --json | jq '.candidates[] | {profile, available, score, headroom_percent, quota_5h, quota_weekly}'
```

Expected: `work` candidate has populated `quota_5h` (and possibly `quota_weekly`) objects. If you also have an over-cap profile (e.g., set `Caps5hTurns: 1` on a profile that has at least one Stop event), that profile appears with `available: false` and `cooldown_until`.

- [ ] **Step 4: Commit log inspection**

```bash
git log --oneline origin/main..HEAD
```

Expected commits (order may vary):

```
test(server): verify quota fields surface via /api/headroom
feat(cli): include 5h/weekly quota columns in suggest output
feat(headroom): tiered quota pressure gating and soft penalty
feat(headroom): fold quota pressure into HeadroomPercent
feat(headroom): populate Candidate quota fields when plan tier configured
feat(headroom): add QueryTurnsInWindow/QueryOldestTurnInWindow to Store interface
feat(headroom): pressure level bands and soft penalty
```

- [ ] **Step 5: Push and open PR**

```bash
git push -u origin feat/quota-pressure-suggest
gh pr create \
  --base main \
  --title "feat(headroom): pressure-aware suggest (v0.2 B2)" \
  --body "$(cat <<'EOF'
## Summary

Folds plan-aware quota pressure into the existing ccx suggest scoring.

- Three threshold bands: warn (>=75%), soft (>=90% → linear -score penalty up to 20), hard (>=100% → Available=false + CooldownUntil = window reset).
- `headroom.Candidate` gains optional `quota_5h`/`quota_weekly` fields (already in the API contract from P0).
- `ccx suggest` table gains 5H / WEEKLY columns; reasons string includes pressure context.

No new API routes; no schema changes. Builds on B1's `internal/quota` and `*storage.Store` methods.

Spec: `docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`
Plan: `docs/superpowers/plans/2026-05-24-ccx-quota-B2-pressure-suggest.md`

## Test plan

- [x] `make ci` green
- [x] `pnpm typecheck` and `pnpm test` green
- [x] Manual: `ccx suggest --json` shows quota fields on plan-tier profiles
- [x] Manual: `Caps5hTurns: 1` + one Stop event → profile becomes `available: false`
EOF
)"
```

- [ ] **Step 6: After merge, update plan index status**

Mark **B2** row's **Status** column `✅ Merged in #<PR-number>`.

---

## Verification criteria (definition of done)

A successful B2 leaves the repo in this state:

1. **`internal/headroom/thresholds.go`** exports `PressureLevel` (enum), `PressureNone/Warn/Soft/Hard`, `ThresholdWarnPct/SoftPct/HardPct`, `SoftPenaltyMax`, `PressureLevelFromPct`, `SoftPenalty`.

2. **`headroom.Store` interface** has the two new methods, and existing stub implementations in test files satisfy them.

3. **`headroom.Candidate`** has `Quota5h *contracts.QuotaWindow` and `QuotaWeekly *contracts.QuotaWindow` fields, with `omitempty` so JSON omits them when nil.

4. **`evaluator.Evaluate` correctness:**
   - Profiles without `PlanTier` get nil quota fields.
   - Profiles with `PlanTier` get populated quota windows.
   - Higher pressure profiles rank lower than lower-pressure profiles (all else equal).
   - Pressure in `[75, 90)` adds a Reason, no score impact.
   - Pressure in `[90, 100)` adds a Reason and a linear score penalty up to 20.
   - Pressure at `>= 100` sets `Available=false` and `CooldownUntil = window reset`.

5. **`ccx suggest`** prints a tabular output with new `5H` and `WEEKLY` columns; em-dash for missing/zero-cap; ⛔ marker for hard-cap.

6. **`GET /api/headroom`** JSON includes `quota_5h`/`quota_weekly` on profiles with PlanTier configured.

7. **Tests:** every new code path has a unit test. Test count for `internal/headroom/...` and `internal/cli/...` is strictly greater than before.

8. **No frozen files modified.** Same diff-check as B1.

9. **PR merged to `main`** with green CI. Plan index updated.

---

## Rollback

If B2 ships and is judged wrong:

- **Thresholds wrong** → adjust constants in `internal/headroom/thresholds.go`. One-line PR.
- **Soft penalty wrong shape** → adjust `SoftPenalty` math. One-file PR.
- **Quota-pressure scoring causes bad recommendations** → set the `quotaPenalty := e.applyQuotaGates(&c)` call to ignore its return (`_ = quotaPenalty`) and remove the subtraction in `c.Score = ...`. Reasons and quota fields still surface in JSON; only scoring impact reverts. Targeted, safe.
- **Whole feature must revert** → revert the PR. No schema migration to undo; contract additions (P0) stay in place because they're harmless.
