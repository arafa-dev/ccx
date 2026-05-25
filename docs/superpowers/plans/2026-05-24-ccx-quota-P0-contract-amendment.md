# ccx v0.2 P0 — Contract Amendment Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the single contract amendment that all of v0.2 (B1, B2, B3a, B3b) depends on. Three frozen files are modified additively: `internal/contracts/types.go`, `api/openapi.yaml`, and (as a regenerated artifact) `web/lib/api-types.ts`. No existing fields are removed or renamed; only new optional fields and new types/routes are added.

**Architecture:** Pure additive contract change. The new fields are TOML/JSON-optional (`omitempty`), the new types are standalone, the new routes are net-new paths. Existing consumers do not need to change. The Go side adds five constants (`RecommendationLevel` enum); the OpenAPI side adds two routes (`/api/quota`, `/api/recommendations/live`), three schemas (`QuotaWindow`, `ProfileQuota`, `RecommendationEvent`), and extends one schema (`HeadroomCandidate`).

**Tech Stack:** Go 1.22+ stdlib only on the contracts side. `openapi-typescript@7.x` (already pinned in `web/package.json`) on the regen side.

**Spec reference:** [`docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`](../specs/2026-05-24-ccx-plan-aware-quota.md) — §7.1 (Go contract additions), §7.2 (OpenAPI additions), §6.3 (no headers — defines `RecommendationLevel`), §6.4 (threshold thinking — informs the level enum).

**Worktree:** This PR is **on `main` directly via a one-shot amendment branch**, per the v0.1 convention for editing frozen files.

```bash
cd /Users/arafa/Developer/ccx          # the main checkout, not a worktree
git fetch origin
git checkout -b contract/quota-amendment origin/main
```

Do not run this work from a feature worktree.

**Exit criteria:**

- [ ] `gofumpt -l internal/contracts/types.go` reports nothing.
- [ ] `go build ./...` succeeds at the repo root.
- [ ] `go test -race -count=1 ./internal/contracts/...` passes.
- [ ] `golangci-lint run ./internal/contracts/...` reports `0 issues.`
- [ ] `cd web && pnpm gen:api && pnpm check:api-types && pnpm typecheck` all succeed.
- [ ] `cd web && pnpm test` passes (existing tests; this PR should not break them).
- [ ] `make ci` passes from repo root.
- [ ] PR opened against `main`, CI green, merged.
- [ ] Status updated in `docs/superpowers/plans/2026-05-24-ccx-quota-plan-index.md`.

**Conventions:**

- All Go code uses tabs (gofumpt enforced).
- Go doc comments start with the symbol's name (`revive` exported rule).
- Commit message format: `type(scope): subject`, scope is `contracts` for Go changes, `api` for openapi changes.
- One commit per task; do not batch.
- This branch may modify only: `internal/contracts/types.go`, `api/openapi.yaml`, `web/lib/api-types.ts`. No other files.
- Do **not** add new constants/types to `internal/contracts/errors.go` or `internal/contracts/interfaces.go` for this PR. Errors and interfaces are out of scope for v0.2 contract additions.

---

## Pre-flight

Confirm you are on the right branch off a clean working tree.

```bash
pwd                                              # → /Users/arafa/Developer/ccx
git status                                       # → On branch contract/quota-amendment, working tree clean
git rev-parse --abbrev-ref HEAD                  # → contract/quota-amendment
git rev-parse --short origin/main                # → matches your current HEAD (i.e., branched from main tip)
test -f internal/contracts/types.go && echo OK   # → OK
test -f api/openapi.yaml && echo OK              # → OK
test -f web/lib/api-types.ts && echo OK          # → OK
go build ./...                                   # → succeeds
cd web && pnpm install --frozen-lockfile && cd ..  # → idempotent on a clean checkout
```

If any check fails, resolve before proceeding.

---

## Task 1: Extend `ProfileLimits` with plan-tier fields

**Files:**
- Modify: `internal/contracts/types.go`
- Modify: `internal/contracts/types_test.go`

- [ ] **Step 1: Add failing test for JSON/TOML round-trip with new fields**

Append to `internal/contracts/types_test.go`:

```go
func TestProfileLimitsPlanFieldsRoundtrip(t *testing.T) {
	in := contracts.ProfileLimits{
		DailyTokenBudget:  1_000_000,
		PlanTier:          "max20",
		WeeklyAnchor:      "monday",
		Caps5hTurns:       900,
		CapsWeeklyTurns:   4500,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out contracts.ProfileLimits
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.PlanTier != "max20" {
		t.Errorf("PlanTier = %q, want %q", out.PlanTier, "max20")
	}
	if out.WeeklyAnchor != "monday" {
		t.Errorf("WeeklyAnchor = %q, want %q", out.WeeklyAnchor, "monday")
	}
	if out.Caps5hTurns != 900 {
		t.Errorf("Caps5hTurns = %d, want 900", out.Caps5hTurns)
	}
	if out.CapsWeeklyTurns != 4500 {
		t.Errorf("CapsWeeklyTurns = %d, want 4500", out.CapsWeeklyTurns)
	}
}

func TestProfileLimitsPlanFieldsOmitEmpty(t *testing.T) {
	in := contracts.ProfileLimits{DailyTokenBudget: 100}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{"plan_tier", "weekly_anchor", "caps_5h_turns", "caps_weekly_turns"} {
		if strings.Contains(s, key) {
			t.Errorf("expected %q to be omitted from %q", key, s)
		}
	}
}
```

If `types_test.go` does not already import `encoding/json` and `strings`, add them.

- [ ] **Step 2: Run tests, confirm failure**

```bash
go test -race -count=1 ./internal/contracts/...
```

Expected: FAIL — `PlanTier`, `WeeklyAnchor`, `Caps5hTurns`, `CapsWeeklyTurns` undefined on `ProfileLimits`.

- [ ] **Step 3: Apply the contract change**

Edit `internal/contracts/types.go`. Find the existing `ProfileLimits` struct (around line 24–31) and replace it with:

```go
// ProfileLimits configures optional per-profile budget and headroom behavior.
// Zero values mean no explicit limit is configured.
type ProfileLimits struct {
	DailyTokenBudget  int     `json:"daily_token_budget"  toml:"daily_token_budget,omitempty"`
	WeeklyTokenBudget int     `json:"weekly_token_budget" toml:"weekly_token_budget,omitempty"`
	MonthlyUSDBudget  float64 `json:"monthly_usd_budget"  toml:"monthly_usd_budget,omitempty"`
	Priority          int     `json:"priority"            toml:"priority,omitempty"`
	SuggestEnabled    *bool   `json:"suggest_enabled"     toml:"suggest_enabled,omitempty"`
	RateLimitCooldown string  `json:"rate_limit_cooldown" toml:"rate_limit_cooldown,omitempty"`

	// PlanTier identifies the Anthropic subscription tier this profile uses.
	// One of "pro", "max5", "max20", "api". Empty disables plan-aware quota
	// tracking for this profile.
	PlanTier string `json:"plan_tier,omitempty" toml:"plan_tier,omitempty"`

	// WeeklyAnchor controls how the weekly quota window is computed. "rolling"
	// (default) counts the trailing 7 days. A weekday name ("monday".."sunday")
	// anchors the window to the most recent occurrence of that weekday at
	// 00:00 UTC.
	WeeklyAnchor string `json:"weekly_anchor,omitempty" toml:"weekly_anchor,omitempty"`

	// Caps5hTurns overrides the shipped default 5-hour-window turn cap. Zero
	// means "use the shipped default for PlanTier".
	Caps5hTurns int `json:"caps_5h_turns,omitempty" toml:"caps_5h_turns,omitempty"`

	// CapsWeeklyTurns overrides the shipped default weekly turn cap. Zero
	// means "use the shipped default for PlanTier".
	CapsWeeklyTurns int `json:"caps_weekly_turns,omitempty" toml:"caps_weekly_turns,omitempty"`
}
```

- [ ] **Step 4: Run gofumpt, lint, and tests**

```bash
gofumpt -w internal/contracts/types.go
go test -race -count=1 ./internal/contracts/...
golangci-lint run ./internal/contracts/...
```

Expected: tests PASS, lint reports `0 issues.`

- [ ] **Step 5: Commit**

```bash
git add internal/contracts/types.go internal/contracts/types_test.go
git commit -m "feat(contracts): add plan-tier fields to ProfileLimits"
```

---

## Task 2: Add `QuotaWindow` and `ProfileQuota` types (TDD)

**Files:**
- Modify: `internal/contracts/types.go`
- Modify: `internal/contracts/types_test.go`

- [ ] **Step 1: Add failing test**

Append to `internal/contracts/types_test.go`:

```go
func TestQuotaWindowJSON(t *testing.T) {
	in := contracts.QuotaWindow{
		Used:     142,
		Cap:      900,
		Pct:      15.78,
		ResetsAt: time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC),
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out contracts.QuotaWindow
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Used != 142 || out.Cap != 900 {
		t.Errorf("counts: got used=%d cap=%d, want 142/900", out.Used, out.Cap)
	}
	if !out.ResetsAt.Equal(in.ResetsAt) {
		t.Errorf("ResetsAt: got %v, want %v", out.ResetsAt, in.ResetsAt)
	}
}

func TestProfileQuotaJSON(t *testing.T) {
	in := contracts.ProfileQuota{
		Profile:  "work",
		PlanTier: "max20",
		Window5h: contracts.QuotaWindow{Used: 142, Cap: 900, Pct: 15.78},
		WindowWeekly: contracts.QuotaWindow{Used: 1203, Cap: 4500, Pct: 26.73},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{`"profile":"work"`, `"plan_tier":"max20"`, `"window_5h"`, `"window_weekly"`} {
		if !strings.Contains(s, key) {
			t.Errorf("expected %q in %q", key, s)
		}
	}
}
```

- [ ] **Step 2: Run tests, confirm failure**

```bash
go test -race -count=1 ./internal/contracts/...
```

Expected: FAIL — `QuotaWindow` and `ProfileQuota` undefined.

- [ ] **Step 3: Apply the contract change**

Insert the following block in `internal/contracts/types.go` **immediately after the `ProfileLimits` struct closing brace** and before the existing `// DaemonStatus is the daemon's...` comment:

```go
// QuotaWindow describes turn usage within a single quota window (rolling 5h
// or weekly). Cap is zero when the owning profile has no PlanTier configured.
type QuotaWindow struct {
	Used     int       `json:"used"`
	Cap      int       `json:"cap"`
	Pct      float64   `json:"pct"`
	ResetsAt time.Time `json:"resets_at"`
}

// ProfileQuota is the per-profile response shape returned by GET /api/quota.
// Both windows are always present; their Cap field is zero when no plan tier
// is configured for the profile.
type ProfileQuota struct {
	Profile      string      `json:"profile"`
	PlanTier     string      `json:"plan_tier"`
	Window5h     QuotaWindow `json:"window_5h"`
	WindowWeekly QuotaWindow `json:"window_weekly"`
}
```

- [ ] **Step 4: gofumpt, test, lint**

```bash
gofumpt -w internal/contracts/types.go
go test -race -count=1 ./internal/contracts/...
golangci-lint run ./internal/contracts/...
```

Expected: tests PASS, lint reports `0 issues.`

- [ ] **Step 5: Commit**

```bash
git add internal/contracts/types.go internal/contracts/types_test.go
git commit -m "feat(contracts): add QuotaWindow and ProfileQuota types"
```

---

## Task 3: Add `RecommendationLevel` enum and `RecommendationEvent` type (TDD)

**Files:**
- Modify: `internal/contracts/types.go`
- Modify: `internal/contracts/types_test.go`

- [ ] **Step 1: Add failing test**

Append to `internal/contracts/types_test.go`:

```go
func TestRecommendationLevelConstants(t *testing.T) {
	cases := []struct {
		level contracts.RecommendationLevel
		want  string
	}{
		{contracts.RecommendationWarn, "warn"},
		{contracts.RecommendationSoft, "soft"},
		{contracts.RecommendationHard, "hard"},
	}
	for _, tc := range cases {
		if string(tc.level) != tc.want {
			t.Errorf("level %v string = %q, want %q", tc.level, string(tc.level), tc.want)
		}
	}
}

func TestRecommendationEventJSON(t *testing.T) {
	in := contracts.RecommendationEvent{
		Profile:        "personal",
		Level:          contracts.RecommendationSoft,
		Reason:         "5h pressure 92%",
		Suggested:      "work",
		Quota5hPct:     92.0,
		QuotaWeeklyPct: 41.5,
		Timestamp:      time.Date(2026, 5, 24, 18, 42, 0, 0, time.UTC),
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{
		`"profile":"personal"`, `"level":"soft"`, `"suggested":"work"`,
		`"quota_5h_pct":92`, `"quota_weekly_pct":41.5`,
	} {
		if !strings.Contains(s, key) {
			t.Errorf("expected %q in %q", key, s)
		}
	}
}

func TestRecommendationEventSuggestedOmitEmpty(t *testing.T) {
	in := contracts.RecommendationEvent{
		Profile:   "personal",
		Level:     contracts.RecommendationHard,
		Reason:    "all siblings at hard cap",
		Timestamp: time.Now().UTC(),
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), `"suggested"`) {
		t.Errorf("expected suggested to be omitted when empty; got %s", data)
	}
}
```

- [ ] **Step 2: Run tests, confirm failure**

```bash
go test -race -count=1 ./internal/contracts/...
```

Expected: FAIL — `RecommendationLevel`, `RecommendationWarn`, `RecommendationSoft`, `RecommendationHard`, `RecommendationEvent` undefined.

- [ ] **Step 3: Apply the contract change**

Insert the following block in `internal/contracts/types.go` **immediately after the `ProfileQuota` struct** added in Task 2:

```go
// RecommendationLevel categorizes the urgency of a streamed pressure-driven
// recommendation. Thresholds (warn/soft/hard) are evaluator-defined.
type RecommendationLevel string

const (
	// RecommendationWarn signals a profile crossed the early-warning threshold.
	RecommendationWarn RecommendationLevel = "warn"
	// RecommendationSoft signals a profile crossed the soft-penalty threshold.
	RecommendationSoft RecommendationLevel = "soft"
	// RecommendationHard signals a profile is at or above its hard cap.
	RecommendationHard RecommendationLevel = "hard"
)

// RecommendationEvent is the payload emitted over /api/recommendations/live
// when the daemon detects a profile crossing a pressure threshold and a
// switch may be warranted. Suggested is empty when no sibling has more
// headroom than the crossed profile.
type RecommendationEvent struct {
	Profile        string              `json:"profile"`
	Level          RecommendationLevel `json:"level"`
	Reason         string              `json:"reason"`
	Suggested      string              `json:"suggested,omitempty"`
	Quota5hPct     float64             `json:"quota_5h_pct"`
	QuotaWeeklyPct float64             `json:"quota_weekly_pct"`
	Timestamp      time.Time           `json:"timestamp"`
}
```

- [ ] **Step 4: gofumpt, test, lint**

```bash
gofumpt -w internal/contracts/types.go
go test -race -count=1 ./internal/contracts/...
golangci-lint run ./internal/contracts/...
```

Expected: tests PASS, lint reports `0 issues.`

- [ ] **Step 5: Verify whole-module build is still clean**

```bash
go build ./...
```

Expected: no output, exit 0. (No callers break because all changes are additive.)

- [ ] **Step 6: Commit**

```bash
git add internal/contracts/types.go internal/contracts/types_test.go
git commit -m "feat(contracts): add RecommendationLevel and RecommendationEvent"
```

---

## Task 4: Add `/api/quota` route and `ProfileQuota` + `QuotaWindow` schemas to `api/openapi.yaml`

**Files:**
- Modify: `api/openapi.yaml`

- [ ] **Step 1: Insert the route**

Open `api/openapi.yaml`. Find the `/api/headroom:` path entry. **Immediately after** its closing block (i.e., before the `components:` top-level key), insert:

```yaml
  /api/quota:
    get:
      summary: Plan-aware per-profile quota usage
      operationId: getQuota
      description: |
        Returns 5-hour-rolling and weekly turn counts for every profile.
        Counts come from local hook telemetry (Stop and StopFailure events);
        ccx does not query Anthropic for remaining quota.
      parameters:
        - in: query
          name: profile
          schema: { type: string }
          description: Filter to one profile. Omit for all.
      responses:
        "200":
          description: Per-profile quota windows
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/ProfileQuota"
```

- [ ] **Step 2: Insert the schemas**

Find the `HeadroomResponse:` schema block. **After its closing properties**, append:

```yaml
    QuotaWindow:
      type: object
      required: [used, cap, pct, resets_at]
      description: |
        Turn usage within a single quota window. `cap` is 0 when the owning
        profile has no plan_tier configured; in that case `pct` is also 0.
      properties:
        used:      { type: integer }
        cap:       { type: integer }
        pct:       { type: number, format: double }
        resets_at: { type: string, format: date-time }

    ProfileQuota:
      type: object
      required: [profile, plan_tier, window_5h, window_weekly]
      properties:
        profile:   { type: string }
        plan_tier:
          type: string
          description: |
            Empty string disables plan-aware tracking; otherwise one of
            "pro", "max5", "max20", "api".
        window_5h:
          $ref: "#/components/schemas/QuotaWindow"
        window_weekly:
          $ref: "#/components/schemas/QuotaWindow"
```

- [ ] **Step 3: Validate YAML syntax**

```bash
cd web
pnpm gen:api
```

Expected: `✨ openapi-typescript … ✓ ../api/openapi.yaml → ./lib/api-types.ts` with no parse errors.

If `pnpm gen:api` errors, the YAML you inserted is malformed — fix indentation (must be 2-space, no tabs) and re-run.

- [ ] **Step 4: Commit the partial openapi.yaml + regenerated types**

```bash
cd ..
git add api/openapi.yaml web/lib/api-types.ts
git commit -m "feat(api): add /api/quota route and ProfileQuota/QuotaWindow schemas"
```

---

## Task 5: Add `/api/recommendations/live` SSE route and `RecommendationEvent` schema

**Files:**
- Modify: `api/openapi.yaml`

- [ ] **Step 1: Insert the SSE route**

Find the `/api/quota:` entry added in Task 4. **Immediately after its closing block** (i.e., still before `components:`), insert:

```yaml
  /api/recommendations/live:
    get:
      summary: Server-Sent Events stream of pressure-driven recommendations
      operationId: getRecommendationsLive
      description: |
        Emits a `recommendation` event when the daemon detects a profile
        crossing a pressure threshold (warn/soft/hard). The data payload is
        a JSON-encoded RecommendationEvent.
      responses:
        "200":
          description: SSE stream
          content:
            text/event-stream:
              schema:
                type: string
```

- [ ] **Step 2: Insert the schema**

Find the `ProfileQuota:` schema added in Task 4. **After its closing block**, append:

```yaml
    RecommendationEvent:
      type: object
      required:
        - profile
        - level
        - reason
        - quota_5h_pct
        - quota_weekly_pct
        - timestamp
      properties:
        profile:   { type: string }
        level:
          type: string
          enum: [warn, soft, hard]
        reason:    { type: string }
        suggested:
          type: string
          description: |
            Recommended profile to switch to. Empty when no sibling has more
            headroom than the crossed profile.
        quota_5h_pct:     { type: number, format: double }
        quota_weekly_pct: { type: number, format: double }
        timestamp:        { type: string, format: date-time }
```

- [ ] **Step 3: Regenerate types**

```bash
cd web
pnpm gen:api
```

Expected: clean regen.

- [ ] **Step 4: Commit**

```bash
cd ..
git add api/openapi.yaml web/lib/api-types.ts
git commit -m "feat(api): add /api/recommendations/live SSE route and RecommendationEvent schema"
```

---

## Task 6: Extend `HeadroomCandidate` with optional quota fields

**Files:**
- Modify: `api/openapi.yaml`

- [ ] **Step 1: Locate and extend the schema**

Open `api/openapi.yaml`. Find the `HeadroomCandidate:` schema (around line 289–315 in the v0.1 baseline). The `properties:` block ends with:

```yaml
        usd_30d:          { type: number, format: double }
```

**Immediately after that line, before the schema's closing block**, append:

```yaml
        quota_5h:
          $ref: "#/components/schemas/QuotaWindow"
        quota_weekly:
          $ref: "#/components/schemas/QuotaWindow"
```

The two new properties are **not** added to the `required:` list — they are optional and only populated when the owning profile has a `plan_tier` configured.

- [ ] **Step 2: Regenerate types**

```bash
cd web
pnpm gen:api
pnpm check:api-types
```

Expected: `api-types.ts is up to date.`

- [ ] **Step 3: Confirm dashboard typecheck still passes**

```bash
pnpm typecheck
```

Expected: no errors. (No existing component reads `quota_5h` or `quota_weekly`; they're additive optional fields.)

- [ ] **Step 4: Commit**

```bash
cd ..
git add api/openapi.yaml web/lib/api-types.ts
git commit -m "feat(api): extend HeadroomCandidate with optional quota_5h and quota_weekly"
```

---

## Task 7: Final verification

**Files:** none modified.

- [ ] **Step 1: Run the full CI gate**

```bash
make ci
```

Expected: lint clean, tests pass with `-race -count=1` across all packages.

- [ ] **Step 2: Run web side end-to-end**

```bash
cd web
pnpm install --frozen-lockfile
pnpm gen:api
pnpm check:api-types
pnpm typecheck
pnpm test
cd ..
```

Expected: all four steps green.

- [ ] **Step 3: Build the binary with staged web assets to confirm embedded UI compiles against the new types**

```bash
cd web && pnpm build && cd ..
make stage-web
make build
./dist/ccx version
```

Expected: `dist/ccx` builds and prints a version string. (The dashboard is not exercised at runtime here; this is purely a compile-time gate.)

- [ ] **Step 4: Inspect the final diff**

```bash
git log --oneline origin/main..HEAD
git diff --stat origin/main..HEAD
```

Expected: 6 commits (one per Task 1–6), 3 files changed: `internal/contracts/types.go`, `api/openapi.yaml`, `web/lib/api-types.ts`. No other files touched.

- [ ] **Step 5: Push and open PR**

```bash
git push -u origin contract/quota-amendment
gh pr create \
  --base main \
  --title "contract: plan-aware quota amendment (v0.2 P0)" \
  --body "$(cat <<'EOF'
## Summary

Single contract amendment that unblocks v0.2 (plan-aware quota & auto-switching).

- `internal/contracts/types.go`: adds 4 optional fields to `ProfileLimits` (`PlanTier`, `WeeklyAnchor`, `Caps5hTurns`, `CapsWeeklyTurns`), and three new types (`QuotaWindow`, `ProfileQuota`, `RecommendationEvent`) plus a `RecommendationLevel` enum.
- `api/openapi.yaml`: adds `/api/quota` and `/api/recommendations/live` routes, the three corresponding schemas, and extends `HeadroomCandidate` with optional `quota_5h`/`quota_weekly` fields.
- `web/lib/api-types.ts`: regenerated.

Pure additive — no existing field removed, no existing route changed.

Spec: `docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`
Plan index: `docs/superpowers/plans/2026-05-24-ccx-quota-plan-index.md`

## Test plan

- [x] `make ci` green
- [x] `pnpm check:api-types` clean
- [x] `pnpm typecheck` clean
- [x] `pnpm test` green
- [x] `./dist/ccx version` builds and runs after `make stage-web && make build`
EOF
)"
```

- [ ] **Step 6: After PR merges, update plan index status**

This is a follow-up after the PR review: edit `docs/superpowers/plans/2026-05-24-ccx-quota-plan-index.md`, change the **P0** row's **Status** column from `Ready` to `✅ Merged in #<PR-number>`. Commit on a new short-lived branch (`docs/quota-p0-status`) or piggyback on the next PR — do **not** push it from this contract branch.

---

## Verification criteria (definition of done)

A successful P0 leaves the repo in this state:

1. **`internal/contracts/types.go`** contains:
   - `ProfileLimits` with 10 fields total: the 6 original + `PlanTier`, `WeeklyAnchor`, `Caps5hTurns`, `CapsWeeklyTurns`.
   - New named types: `QuotaWindow`, `ProfileQuota`, `RecommendationLevel`, `RecommendationEvent`.
   - Three exported constants: `RecommendationWarn`, `RecommendationSoft`, `RecommendationHard`.
   - All exported symbols have doc comments starting with the symbol name.
   - `gofumpt` clean; `golangci-lint` reports 0 issues.

2. **`api/openapi.yaml`** contains:
   - New paths `/api/quota` and `/api/recommendations/live`.
   - New schemas `QuotaWindow`, `ProfileQuota`, `RecommendationEvent`.
   - Extended `HeadroomCandidate` schema with optional `quota_5h`, `quota_weekly` properties.
   - Parses cleanly via `openapi-typescript`.

3. **`web/lib/api-types.ts`** is the regenerated artifact matching the new openapi.yaml exactly. `pnpm check:api-types` returns `api-types.ts is up to date.`

4. **Existing tests untouched and green:** `go test ./...` and `pnpm test` both pass with no test-file modifications outside the new contract assertions in `internal/contracts/types_test.go`.

5. **No other files modified.** `git diff origin/main..HEAD --name-only` returns exactly three paths:
   ```
   api/openapi.yaml
   internal/contracts/types.go
   web/lib/api-types.ts
   ```

6. **PR merged to `main`** with green CI. The plan index status row updated.

If any criterion fails, the amendment is not done — investigate, fix, push another commit, retry CI.

---

## Rollback

If the amendment lands and is later judged wrong (e.g., field naming needs adjustment), the rollback path is:

1. Open a new contract-amendment PR that reverts the offending fields/types. Same workflow as this plan: branch from `main`, additive or subtractive edits, regenerate `api-types.ts`, PR, merge.
2. If any of B1/B2/B3a/B3b already shipped against the old fields, their PRs must be amended in lockstep — open a follow-up PR per affected phase before the contract revert lands.
3. Do **not** force-push this contract branch after it's merged. Revert via a new commit instead.

For this PR's narrower failure modes:
- If `pnpm check:api-types` complains after regen: the `api-types.ts` you committed is stale. `cd web && pnpm gen:api`, recommit.
- If `gofumpt` keeps re-aligning columns on every commit: that's expected — gofumpt re-aligns tag columns when new long field names are added. Accept the alignment.
- If `golangci-lint` flags "exported X should have comment": add the missing doc comment. All exported symbols must have one (revive's `exported` rule).
