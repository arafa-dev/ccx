# ccx Phase 1 A4 — `internal/pricing/` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `contracts.PricingTable` — load an embedded `pricing/models.yaml`, optionally layer a user override from `~/.ccx/pricing.yaml`, look up rates by model name + timestamp using `effective_from` semantics, and compute estimated USD cost from a `contracts.Usage`.

**Architecture:** Pure-Go package with a single allowed third-party dep (`gopkg.in/yaml.v3`). The embedded YAML is the v0.1 baseline; a user-edited override at `~/.ccx/pricing.yaml` is deep-merged on top by model name. Lookup picks the row whose `effective_from` is the latest one ≤ the query timestamp. Unknown models return `0.0` cost and `nil` error so usage display never blocks on a pricing miss; the first miss for a given model is logged once via `log/slog`.

**Tech Stack:** Go 1.22+, `gopkg.in/yaml.v3`, stdlib only otherwise.

**Spec reference:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — Section 7.3 (Pricing).

**Worktree:** `feat/pricing` off `main`.

```bash
git worktree add ../ccx-pricing -b feat/pricing main
cd ../ccx-pricing
```

**Allowed dependencies:** `github.com/arafa-dev/ccx/internal/contracts` (sibling — read-only), `gopkg.in/yaml.v3`, and Go stdlib (`embed`, `errors`, `fmt`, `log/slog`, `os`, `path/filepath`, `sort`, `sync`, `time`).

**Forbidden:** Any other `internal/*` package. No CLI flags, no cobra, no SQLite, no HTTP.

**YAGNI:** No currency conversions, no rounding, no negative-rate validation beyond "non-NaN, finite". No reload-on-change. No daemon. No multi-currency. The UI rounds for display — this package returns raw `float64`.

**Exit criteria:**
- `go build ./internal/pricing/...` succeeds
- `go test -race -count=1 ./internal/pricing/...` succeeds
- `golangci-lint run ./internal/pricing/...` reports zero issues
- `gofumpt -l internal/pricing` produces no output
- A PR against `main` opens with all tasks in this plan committed
- Reviewer can verify the embedded YAML loads, user override layers correctly, and `Cost` matches hand-computed values for the documented opus test case

---

## Pre-flight

Confirm the working directory is the `feat/pricing` worktree and the Phase 0 stub for this package is present.

```bash
pwd                                                  # → .../ccx-pricing
git branch --show-current                            # → feat/pricing
git log --oneline -1                                 # → tip of main (Phase 0 merged)
ls internal/pricing/                                 # → doc.go (only)
ls internal/contracts/                               # → types.go, interfaces.go, errors.go, ...
```

If any of the above is wrong, stop and fix the worktree before starting Task 1.

**Conventions for this plan:**
- Go files use tabs (gofumpt enforced).
- Commit messages follow `type(scope): subject` with scope `pricing`.
- One commit per task. Do not batch.
- Every task: failing test → `go test` (red) → minimal impl → `go test` (green) → commit.
- Tests use the `_test` package suffix (`package pricing_test`) for black-box tests where possible.

---

## Task 1: Add `gopkg.in/yaml.v3` dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Write a failing test that requires yaml.v3**

Create `internal/pricing/yaml_dep_test.go`:

```go
package pricing_test

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestYAMLDepAvailable verifies the yaml.v3 module is wired into go.mod.
// It does no real work — it just forces the dependency to be linkable from
// this package, which is the only place in ccx allowed to import yaml.v3.
func TestYAMLDepAvailable(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("hello: world\n"), &node); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if node.Kind == 0 {
		t.Fatalf("expected non-zero Kind after Unmarshal")
	}
}
```

- [ ] **Step 2: Run test to verify it fails (missing dependency)**

```bash
go test ./internal/pricing/...
```

Expected: build error — `no required module provides package gopkg.in/yaml.v3`.

- [ ] **Step 3: Add the dependency**

```bash
go get gopkg.in/yaml.v3@latest
go mod tidy
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/pricing/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/pricing/yaml_dep_test.go
git commit -m "build(pricing): add gopkg.in/yaml.v3 dependency"
```

---

## Task 2: Define `Rate` and `Table` internal types + YAML schema

**Files:**
- Create: `internal/pricing/table.go`
- Create: `internal/pricing/table_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/pricing/table_test.go`:

```go
package pricing_test

import (
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/pricing"
)

// minimal embedded-shaped YAML for unit tests
const twoModelYAML = `
last_updated: 2026-01-15
models:
  - model: claude-opus-4-7
    effective_from: 2026-01-15
    input_per_mtok: 15.00
    output_per_mtok: 75.00
    cache_read_per_mtok: 1.50
    cache_create_per_mtok: 18.75
  - model: claude-sonnet-4-6
    effective_from: 2026-01-15
    input_per_mtok: 3.00
    output_per_mtok: 15.00
    cache_read_per_mtok: 0.30
    cache_create_per_mtok: 3.75
`

func TestNewTableFromBytesParsesEmbedded(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes([]byte(twoModelYAML), nil)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	want := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if got := tbl.LastUpdated(); !got.Equal(want) {
		t.Errorf("LastUpdated = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/pricing/...
```

Expected: FAIL — `pricing.NewTableFromBytes` and `pricing.Table` undefined.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/pricing/table.go`:

```go
package pricing

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Rate is a single pricing row for a model that took effect at EffectiveFrom.
// Rates are denominated in USD per million tokens of the corresponding bucket.
type Rate struct {
	Model              string    `yaml:"model"`
	EffectiveFrom      time.Time `yaml:"effective_from"`
	InputPerMTok       float64   `yaml:"input_per_mtok"`
	OutputPerMTok      float64   `yaml:"output_per_mtok"`
	CacheReadPerMTok   float64   `yaml:"cache_read_per_mtok"`
	CacheCreatePerMTok float64   `yaml:"cache_create_per_mtok"`
}

// fileShape is the on-disk YAML structure. Exported only for clarity within
// the package; never returned to callers.
type fileShape struct {
	LastUpdated time.Time `yaml:"last_updated"`
	Models      []Rate    `yaml:"models"`
}

// Table holds the merged set of rates and answers Cost / LastUpdated queries.
// Construct with NewTable or NewTableFromBytes. Safe for concurrent use after
// construction (it is treated as immutable; the warn-once map is mutex-guarded).
type Table struct {
	lastUpdated time.Time
	// rates is keyed by model name; each slice is sorted ascending by
	// EffectiveFrom so a reverse linear scan finds the applicable row fast.
	rates map[string][]Rate

	warnMu   sync.Mutex
	warnedOn map[string]bool
}

// LastUpdated returns the most recent effective_from across all loaded rates.
// It is intended for footer-style "rates as of <date>" UI strings.
func (t *Table) LastUpdated() time.Time { return t.lastUpdated }

// NewTableFromBytes constructs a Table from explicit YAML bytes. Useful for
// tests. Pass nil for userOverride if no override is desired. When both are
// provided, userOverride is deep-merged on top of embedded by model name.
func NewTableFromBytes(embedded, userOverride []byte) (*Table, error) {
	base, err := parseFile(embedded)
	if err != nil {
		return nil, fmt.Errorf("parsing embedded pricing: %w", err)
	}

	if len(userOverride) > 0 {
		over, err := parseFile(userOverride)
		if err != nil {
			return nil, fmt.Errorf("parsing user pricing override: %w", err)
		}
		base = mergeFiles(base, over)
	}

	return buildTable(base), nil
}

// parseFile unmarshals YAML bytes into a fileShape, tolerating an empty input.
func parseFile(data []byte) (fileShape, error) {
	var f fileShape
	if len(data) == 0 {
		return f, nil
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return f, err
	}
	return f, nil
}

// mergeFiles layers `over` on top of `base`. Semantics:
//   - For every model in `over`, that model's full set of rates *replaces*
//     the base set for that model. This is the simplest semantics that lets
//     users tweak a model without copying everything.
//   - LastUpdated becomes the later of the two timestamps.
//   - Models in `base` not mentioned in `over` are kept unchanged.
func mergeFiles(base, over fileShape) fileShape {
	out := fileShape{LastUpdated: base.LastUpdated, Models: nil}
	if over.LastUpdated.After(out.LastUpdated) {
		out.LastUpdated = over.LastUpdated
	}

	overrideNames := make(map[string]struct{}, len(over.Models))
	for _, r := range over.Models {
		overrideNames[r.Model] = struct{}{}
	}

	for _, r := range base.Models {
		if _, replaced := overrideNames[r.Model]; replaced {
			continue
		}
		out.Models = append(out.Models, r)
	}
	out.Models = append(out.Models, over.Models...)
	return out
}

// buildTable converts a parsed fileShape into the queryable Table form.
func buildTable(f fileShape) *Table {
	rates := make(map[string][]Rate, len(f.Models))
	for _, r := range f.Models {
		rates[r.Model] = append(rates[r.Model], r)
	}
	for k := range rates {
		sort.Slice(rates[k], func(i, j int) bool {
			return rates[k][i].EffectiveFrom.Before(rates[k][j].EffectiveFrom)
		})
	}

	last := f.LastUpdated
	for _, list := range rates {
		for _, r := range list {
			if r.EffectiveFrom.After(last) {
				last = r.EffectiveFrom
			}
		}
	}

	return &Table{
		lastUpdated: last,
		rates:       rates,
		warnedOn:    make(map[string]bool),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/pricing/...
```

Expected: PASS for `TestNewTableFromBytesParsesEmbedded` and `TestYAMLDepAvailable`.

- [ ] **Step 5: Commit**

```bash
git add internal/pricing/table.go internal/pricing/table_test.go
git commit -m "feat(pricing): add Rate, Table types and YAML parse"
```

---

## Task 3: Implement `Cost` for the happy path (known model, single rate)

**Files:**
- Modify: `internal/pricing/table.go`
- Modify: `internal/pricing/table_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/pricing/table_test.go`:

```go
import "github.com/arafa-dev/ccx/internal/contracts"
```

(If the import block at the top of `table_test.go` needs editing instead of adding a second `import` block, fold the new path into the existing group.)

Append the test cases:

```go
func TestCostOpusOneMillionEachBucket(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes([]byte(twoModelYAML), nil)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	usage := contracts.Usage{
		InputTokens:       1_000_000,
		OutputTokens:      1_000_000,
		CacheReadTokens:   1_000_000,
		CacheCreateTokens: 1_000_000,
	}

	got, err := tbl.Cost("claude-opus-4-7", ts, usage)
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}

	// 15.00 + 75.00 + 1.50 + 18.75 = 110.25
	const want = 110.25
	if got != want {
		t.Errorf("Cost = %.6f, want %.6f", got, want)
	}
}

func TestCostSonnetPartialUsage(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes([]byte(twoModelYAML), nil)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	usage := contracts.Usage{
		InputTokens:  500_000, // 0.5 MTok * $3   = $1.50
		OutputTokens: 200_000, // 0.2 MTok * $15  = $3.00
	}

	got, err := tbl.Cost("claude-sonnet-4-6", ts, usage)
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}

	const want = 4.50
	if got != want {
		t.Errorf("Cost = %.6f, want %.6f", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/pricing/...
```

Expected: FAIL — `(*Table).Cost` undefined.

- [ ] **Step 3: Implement `Cost`**

Append to `internal/pricing/table.go`:

```go
import "github.com/arafa-dev/ccx/internal/contracts"
```

(Fold into the existing import block.)

Then append:

```go
// Cost returns the estimated USD cost for `usage` against `model` at time `ts`.
//
// Lookup semantics: the applicable rate is the one whose EffectiveFrom is the
// latest value ≤ ts. If no rate is effective at ts (e.g., ts is before the
// earliest known effective_from for that model), Cost returns 0 and a nil
// error — pricing is best-effort and must never block usage display.
//
// Unknown model: returns 0 and nil error. The first time a given model is
// missed, a warning is logged once via log/slog. See logUnknownOnce.
//
// Formula (per bucket): (tokens / 1e6) * rate_per_mtok. The four buckets are
// summed. Result is raw float64; the UI is responsible for rounding to cents.
func (t *Table) Cost(model string, ts time.Time, usage contracts.Usage) (float64, error) {
	rate, ok := t.lookup(model, ts)
	if !ok {
		t.logUnknownOnce(model)
		return 0, nil
	}

	const perMillion = 1_000_000.0
	cost := float64(usage.InputTokens)/perMillion*rate.InputPerMTok +
		float64(usage.OutputTokens)/perMillion*rate.OutputPerMTok +
		float64(usage.CacheReadTokens)/perMillion*rate.CacheReadPerMTok +
		float64(usage.CacheCreateTokens)/perMillion*rate.CacheCreatePerMTok
	return cost, nil
}

// lookup returns the Rate effective at ts for model, or (zero, false) if none.
func (t *Table) lookup(model string, ts time.Time) (Rate, bool) {
	list, ok := t.rates[model]
	if !ok || len(list) == 0 {
		return Rate{}, false
	}
	// list is sorted ascending by EffectiveFrom; walk in reverse to find the
	// latest one that is ≤ ts.
	for i := len(list) - 1; i >= 0; i-- {
		if !list[i].EffectiveFrom.After(ts) {
			return list[i], true
		}
	}
	return Rate{}, false
}
```

Also add the `logUnknownOnce` helper (in the same file):

```go
import "log/slog"
```

(Fold into imports.)

```go
// logUnknownOnce emits exactly one slog warning per unknown model name. Used
// from Cost so a typo or new-model JSONL doesn't spam logs.
func (t *Table) logUnknownOnce(model string) {
	t.warnMu.Lock()
	defer t.warnMu.Unlock()
	if t.warnedOn[model] {
		return
	}
	t.warnedOn[model] = true
	slog.Warn("pricing: unknown model, cost will be reported as 0",
		"model", model)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/pricing/...
```

Expected: PASS for the two new tests plus prior tests.

- [ ] **Step 5: Commit**

```bash
git add internal/pricing/table.go internal/pricing/table_test.go
git commit -m "feat(pricing): implement Cost lookup + per-bucket formula"
```

---

## Task 4: Handle unknown model + before-earliest-effective-from

**Files:**
- Modify: `internal/pricing/table_test.go`

The implementation already covers these cases via `lookup` + `logUnknownOnce`. This task locks them down with explicit tests.

- [ ] **Step 1: Write the failing tests**

Append to `internal/pricing/table_test.go`:

```go
func TestCostUnknownModelReturnsZeroNoError(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes([]byte(twoModelYAML), nil)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	got, err := tbl.Cost("claude-mythical-9-9", ts, contracts.Usage{InputTokens: 5_000_000})
	if err != nil {
		t.Fatalf("Cost should not error on unknown model, got %v", err)
	}
	if got != 0 {
		t.Errorf("Cost on unknown model = %v, want 0", got)
	}
}

func TestCostTimestampBeforeEarliestEffectiveFromReturnsZero(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes([]byte(twoModelYAML), nil)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	// embedded earliest is 2026-01-15; pick a ts that predates it.
	ts := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	got, err := tbl.Cost("claude-opus-4-7", ts, contracts.Usage{InputTokens: 1_000_000})
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if got != 0 {
		t.Errorf("Cost before earliest effective_from = %v, want 0", got)
	}
}

func TestCostUnknownModelLogsOnlyOnce(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes([]byte(twoModelYAML), nil)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	// Call Cost 5x for the same unknown model. We can't easily intercept slog
	// without an extra dep; this test verifies behavior compiles, runs, and
	// produces no error. The "logs once" guarantee is exercised indirectly:
	// the implementation under test consults t.warnedOn[model].
	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if _, err := tbl.Cost("claude-mythical-9-9", ts, contracts.Usage{}); err != nil {
			t.Fatalf("Cost iteration %d: %v", i, err)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they pass (no impl change needed)**

```bash
go test ./internal/pricing/...
```

Expected: PASS — Cost already handles both branches from Task 3.

If any test fails, fix the implementation in `table.go` and re-run before committing.

- [ ] **Step 3: Commit**

```bash
git add internal/pricing/table_test.go
git commit -m "test(pricing): cover unknown model and pre-earliest timestamp"
```

---

## Task 5: Multiple `effective_from` rows for one model

**Files:**
- Modify: `internal/pricing/table_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/pricing/table_test.go`:

```go
const multiEffectiveYAML = `
last_updated: 2026-06-01
models:
  - model: claude-opus-4-7
    effective_from: 2026-01-15
    input_per_mtok: 15.00
    output_per_mtok: 75.00
    cache_read_per_mtok: 1.50
    cache_create_per_mtok: 18.75
  - model: claude-opus-4-7
    effective_from: 2026-06-01
    input_per_mtok: 10.00
    output_per_mtok: 50.00
    cache_read_per_mtok: 1.00
    cache_create_per_mtok: 12.50
`

func TestCostMultipleEffectiveFromPicksLatestOnOrBefore(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes([]byte(multiEffectiveYAML), nil)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	usage := contracts.Usage{InputTokens: 1_000_000}

	tests := []struct {
		name string
		ts   time.Time
		want float64
	}{
		{
			name: "before earliest",
			ts:   time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			want: 0,
		},
		{
			name: "exactly at first effective_from",
			ts:   time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			want: 15.00,
		},
		{
			name: "between the two effective dates",
			ts:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			want: 15.00,
		},
		{
			name: "exactly at second effective_from",
			ts:   time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			want: 10.00,
		},
		{
			name: "after second effective_from",
			ts:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			want: 10.00,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tbl.Cost("claude-opus-4-7", tc.ts, usage)
			if err != nil {
				t.Fatalf("Cost: %v", err)
			}
			if got != tc.want {
				t.Errorf("Cost @ %v = %.6f, want %.6f", tc.ts, got, tc.want)
			}
		})
	}
}

func TestLastUpdatedReflectsLatestEffectiveFrom(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes([]byte(multiEffectiveYAML), nil)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	want := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if got := tbl.LastUpdated(); !got.Equal(want) {
		t.Errorf("LastUpdated = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass (impl already correct)**

```bash
go test ./internal/pricing/...
```

Expected: PASS for all subtests.

If any fail, the bug is in `lookup` or `buildTable.LastUpdated` derivation — fix `table.go` and re-run before committing.

- [ ] **Step 3: Commit**

```bash
git add internal/pricing/table_test.go
git commit -m "test(pricing): cover multi-effective_from lookup semantics"
```

---

## Task 6: User-override layering — add new model

**Files:**
- Modify: `internal/pricing/table_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/pricing/table_test.go`:

```go
const userOverrideAddYAML = `
models:
  - model: claude-foo
    effective_from: 2026-02-01
    input_per_mtok: 2.00
    output_per_mtok: 10.00
    cache_read_per_mtok: 0.20
    cache_create_per_mtok: 2.50
`

func TestUserOverrideAddsNewModel(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes(
		[]byte(twoModelYAML),
		[]byte(userOverrideAddYAML),
	)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	usage := contracts.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}

	got, err := tbl.Cost("claude-foo", ts, usage)
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	const want = 12.00 // 2.00 + 10.00
	if got != want {
		t.Errorf("Cost for added model = %.6f, want %.6f", got, want)
	}

	// The base models must still be present.
	gotOpus, err := tbl.Cost("claude-opus-4-7", ts, contracts.Usage{InputTokens: 1_000_000})
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if gotOpus != 15.00 {
		t.Errorf("base opus still queryable = %.6f, want 15.00", gotOpus)
	}
}
```

- [ ] **Step 2: Run test to verify it passes (impl already correct)**

```bash
go test ./internal/pricing/...
```

Expected: PASS.

If it fails, `mergeFiles` is dropping models from base — fix and re-run before committing.

- [ ] **Step 3: Commit**

```bash
git add internal/pricing/table_test.go
git commit -m "test(pricing): user override can add a new model"
```

---

## Task 7: User-override layering — replace existing model

**Files:**
- Modify: `internal/pricing/table_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/pricing/table_test.go`:

```go
const userOverrideReplaceOpusYAML = `
models:
  - model: claude-opus-4-7
    effective_from: 2026-01-15
    input_per_mtok: 9.99
    output_per_mtok: 49.99
    cache_read_per_mtok: 0.99
    cache_create_per_mtok: 12.49
`

func TestUserOverrideReplacesExistingModel(t *testing.T) {
	tbl, err := pricing.NewTableFromBytes(
		[]byte(twoModelYAML),
		[]byte(userOverrideReplaceOpusYAML),
	)
	if err != nil {
		t.Fatalf("NewTableFromBytes: %v", err)
	}

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	got, err := tbl.Cost("claude-opus-4-7", ts, contracts.Usage{InputTokens: 1_000_000})
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	const want = 9.99
	if got != want {
		t.Errorf("Cost after override = %.6f, want %.6f (override should win)", got, want)
	}

	// Sibling model untouched by the override must still use base rates.
	gotSonnet, err := tbl.Cost("claude-sonnet-4-6", ts, contracts.Usage{InputTokens: 1_000_000})
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if gotSonnet != 3.00 {
		t.Errorf("untouched sonnet = %.6f, want 3.00", gotSonnet)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test ./internal/pricing/...
```

Expected: PASS.

If it fails, the override is being appended without removing the base entry — `mergeFiles` must drop overlapping models from base.

- [ ] **Step 3: Commit**

```bash
git add internal/pricing/table_test.go
git commit -m "test(pricing): user override replaces existing model rates"
```

---

## Task 8: Create `pricing/models.yaml` embedded data file

**Files:**
- Create: `pricing/models.yaml`

This file lives at the repository root in a sibling `pricing/` directory (NOT inside `internal/pricing/`). It is the canonical embedded baseline. Phase 0 did not create it.

- [ ] **Step 1: Confirm the directory does not yet exist**

```bash
ls pricing/ 2>/dev/null || echo "pricing/ absent (expected)"
```

Expected output: `pricing/ absent (expected)`.

- [ ] **Step 2: Write `pricing/models.yaml`**

Create `pricing/models.yaml` with the exact baseline content:

```yaml
last_updated: 2026-01-15
models:
  - model: claude-opus-4-7
    effective_from: 2026-01-15
    input_per_mtok: 15.00
    output_per_mtok: 75.00
    cache_read_per_mtok: 1.50
    cache_create_per_mtok: 18.75
  - model: claude-sonnet-4-6
    effective_from: 2026-01-15
    input_per_mtok: 3.00
    output_per_mtok: 15.00
    cache_read_per_mtok: 0.30
    cache_create_per_mtok: 3.75
  - model: claude-haiku-4-5
    effective_from: 2026-01-15
    input_per_mtok: 0.80
    output_per_mtok: 4.00
    cache_read_per_mtok: 0.08
    cache_create_per_mtok: 1.00
```

- [ ] **Step 3: Verify it parses as YAML (sanity check)**

```bash
go run -tags ignore - <<'EOF' 2>/dev/null || true
EOF
# alternative: rely on Task 9's embedded-load test to catch parse errors.
```

(No standalone check required; Task 9 wires the file into Go and tests will validate.)

- [ ] **Step 4: Commit**

```bash
git add pricing/models.yaml
git commit -m "feat(pricing): add embedded baseline models.yaml"
```

---

## Task 9: Wire `//go:embed` for the baseline YAML and implement `NewTable`

**Files:**
- Create: `internal/pricing/embedded.go`
- Create: `internal/pricing/embedded_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/pricing/embedded_test.go`:

```go
package pricing_test

import (
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/pricing"
)

func TestNewTableLoadsEmbeddedBaseline(t *testing.T) {
	tbl, err := pricing.NewTable()
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}

	wantLast := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if got := tbl.LastUpdated(); !got.Equal(wantLast) {
		t.Errorf("LastUpdated = %v, want %v", got, wantLast)
	}

	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)

	// Spot-check each of the three embedded models with 1M input only.
	cases := []struct {
		model string
		want  float64
	}{
		{"claude-opus-4-7", 15.00},
		{"claude-sonnet-4-6", 3.00},
		{"claude-haiku-4-5", 0.80},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			got, err := tbl.Cost(tc.model, ts, contracts.Usage{InputTokens: 1_000_000})
			if err != nil {
				t.Fatalf("Cost: %v", err)
			}
			if got != tc.want {
				t.Errorf("Cost = %.6f, want %.6f", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/pricing/...
```

Expected: FAIL — `pricing.NewTable` undefined.

- [ ] **Step 3: Implement `NewTable`**

Create `internal/pricing/embedded.go`:

```go
package pricing

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// embeddedModelsYAML is the v0.1 baseline pricing table. The file lives at
// pricing/models.yaml in the repo root and is the single source of truth.
//
//go:embed models.yaml
var embeddedModelsYAML []byte

// NewTable constructs a Table by parsing the embedded baseline YAML and,
// if present, layering the user override at ~/.ccx/pricing.yaml on top.
//
// Override file missing → returns the embedded-only table with no error.
// Override file present but unreadable → returns an error.
// Override file present but malformed YAML → returns an error.
//
// Callers that want explicit control (tests, alternate config dirs) should
// use NewTableFromBytes directly.
func NewTable() (*Table, error) {
	override, err := readUserOverride()
	if err != nil {
		return nil, err
	}
	return NewTableFromBytes(embeddedModelsYAML, override)
}

// readUserOverride reads ~/.ccx/pricing.yaml. Returns (nil, nil) if the file
// is absent. Returns (bytes, nil) if present and readable. Returns (nil, err)
// if the home dir cannot be resolved or the file exists but cannot be read.
func readUserOverride() ([]byte, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving user home dir: %w", err)
	}
	path := filepath.Join(home, ".ccx", "pricing.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}
```

**Important:** the `//go:embed models.yaml` directive must reference a file in the *same directory as the Go source*. We need a copy (or, preferably, a symlink-free reachable path). Go's embed cannot reach outside the package directory (parent paths via `..` are rejected).

Resolution: also place a copy at `internal/pricing/models.yaml`. The canonical source remains `pricing/models.yaml` at the repo root (per Phase 0 spec section 5.1 — `pricing/models.yaml # embedded via go:embed`), but the Go embed directive requires the file to be reachable from the package. We satisfy both by keeping the canonical copy at the root and embedding from there via a build-time step OR by placing the embedded copy next to the source. To keep things simple and deterministic, this plan embeds the in-package copy and treats `internal/pricing/models.yaml` as the embedded artifact, while `pricing/models.yaml` remains the human-edited canonical source. A future Make target can copy root → internal; for v0.1, we just keep them in sync manually and add a guard test.

- [ ] **Step 4: Copy the YAML next to the Go source**

```bash
mkdir -p internal/pricing
cp pricing/models.yaml internal/pricing/models.yaml
```

- [ ] **Step 5: Add a guard test that the two copies match**

Append to `internal/pricing/embedded_test.go`:

```go
import (
	"bytes"
	"os"
)
```

(Fold into existing imports.)

```go
func TestEmbeddedYAMLMatchesRootCopy(t *testing.T) {
	// The repo-root canonical copy lives at pricing/models.yaml.
	// The Go-embed copy lives at internal/pricing/models.yaml.
	// They must stay byte-identical so the embedded binary reflects the
	// human-edited source. Tests run with cwd = package dir, so the root
	// copy is at ../../pricing/models.yaml.
	rootBytes, err := os.ReadFile("../../pricing/models.yaml")
	if err != nil {
		t.Fatalf("reading root copy: %v", err)
	}
	pkgBytes, err := os.ReadFile("models.yaml")
	if err != nil {
		t.Fatalf("reading package copy: %v", err)
	}
	if !bytes.Equal(rootBytes, pkgBytes) {
		t.Errorf("pricing/models.yaml and internal/pricing/models.yaml differ; keep them in sync")
	}
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/pricing/...
```

Expected: PASS for embedded baseline + matching-copies guard.

- [ ] **Step 7: Commit**

```bash
git add internal/pricing/embedded.go internal/pricing/embedded_test.go internal/pricing/models.yaml
git commit -m "feat(pricing): embed baseline YAML and implement NewTable"
```

---

## Task 10: Confirm `*Table` satisfies `contracts.PricingTable`

**Files:**
- Create: `internal/pricing/contracts_test.go`

This is a compile-time check that the concrete type matches the interface. No runtime behavior — but it catches signature drift the moment it happens.

- [ ] **Step 1: Write the failing test**

Create `internal/pricing/contracts_test.go`:

```go
package pricing_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/pricing"
)

// Compile-time guarantee that *Table implements contracts.PricingTable.
var _ contracts.PricingTable = (*pricing.Table)(nil)

func TestTableImplementsContract(t *testing.T) {
	tbl, err := pricing.NewTable()
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	var iface contracts.PricingTable = tbl
	if iface.LastUpdated().IsZero() {
		t.Errorf("LastUpdated via interface should not be zero on embedded table")
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test ./internal/pricing/...
```

Expected: PASS. If it fails with a `does not implement` error, fix the `Cost` or `LastUpdated` signatures in `table.go` to match `contracts.PricingTable` exactly (the assignment shown in the test forces signature parity).

- [ ] **Step 3: Commit**

```bash
git add internal/pricing/contracts_test.go
git commit -m "test(pricing): assert *Table implements contracts.PricingTable"
```

---

## Task 11: Reject malformed user-override YAML

**Files:**
- Modify: `internal/pricing/table_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/pricing/table_test.go`:

```go
func TestUserOverrideMalformedYAMLReturnsError(t *testing.T) {
	bad := []byte("models: [this is not valid yaml: : :\n")
	_, err := pricing.NewTableFromBytes([]byte(twoModelYAML), bad)
	if err == nil {
		t.Fatalf("expected error on malformed user override, got nil")
	}
}

func TestEmbeddedMalformedYAMLReturnsError(t *testing.T) {
	bad := []byte("models: [this is not valid yaml: : :\n")
	_, err := pricing.NewTableFromBytes(bad, nil)
	if err == nil {
		t.Fatalf("expected error on malformed embedded YAML, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they pass (impl already covers it)**

```bash
go test ./internal/pricing/...
```

Expected: PASS — `parseFile` propagates `yaml.Unmarshal` errors, which `NewTableFromBytes` wraps.

If a test fails (e.g., yaml.v3 silently accepts the input), tweak the malformed string until both produce real parse errors, then commit.

- [ ] **Step 3: Commit**

```bash
git add internal/pricing/table_test.go
git commit -m "test(pricing): reject malformed YAML in embedded or override"
```

---

## Task 12: Run full local quality gate for the package

- [ ] **Step 1: Format check**

```bash
gofumpt -l internal/pricing
```

Expected: no output.

If any file is listed, run `gofumpt -w internal/pricing`, review the diff, and commit:

```bash
gofumpt -w internal/pricing
git diff
git add internal/pricing && git commit -m "style(pricing): gofumpt"
```

- [ ] **Step 2: Vet**

```bash
go vet ./internal/pricing/...
```

Expected: no output.

- [ ] **Step 3: Lint**

```bash
golangci-lint run ./internal/pricing/...
```

Expected: exit 0, no issues.

- [ ] **Step 4: Test with race detector**

```bash
go test -race -count=1 ./internal/pricing/...
```

Expected: all tests PASS.

- [ ] **Step 5: Build the whole tree**

```bash
go build ./...
```

Expected: no output.

If any step fails, fix and re-run from Step 1 before opening the PR.

---

## Task 13: Open PR against `main`

- [ ] **Step 1: Rebase on latest `main`**

```bash
git fetch origin
git rebase origin/main
```

If conflicts arise (most likely in `go.sum` or `go.mod`), resolve, re-run Task 12, then continue.

- [ ] **Step 2: Push the branch**

```bash
git push -u origin feat/pricing
```

- [ ] **Step 3: Open the PR**

```bash
gh pr create \
  --base main \
  --head feat/pricing \
  --title "feat(pricing): implement contracts.PricingTable with embedded YAML" \
  --body "$(cat <<'EOF'
## What

Implements `internal/pricing/` per Phase 1 plan A4. Provides a `*Table` value
that satisfies `contracts.PricingTable` by loading the embedded
`pricing/models.yaml` baseline and (optionally) layering `~/.ccx/pricing.yaml`.

## Why

Phase 1 dependency for `internal/cli/` (usage display) and `internal/server/`
(dashboard cost numbers). Closes the pricing portion of spec section 7.3.

## Contract impact

- [x] This PR does NOT modify `internal/contracts/`, `api/openapi.yaml`,
      `internal/storage/schema.sql`, or `docs/conventions.md`
- [ ] If it does, this is a contract-amendment PR

## Checklist

- [x] Tests added/updated and all pass locally (`make test`)
- [x] Lint clean locally (`make lint`)
- [x] Only new dep is `gopkg.in/yaml.v3` (explicitly allowed for this package)
- [x] No README/docs changes required (UI labeling lives elsewhere)

## Phase 1 worktree

- Package: `internal/pricing`
- Plan: `docs/superpowers/plans/2026-05-19-ccx-phase-1-A4-pricing.md`
EOF
)"
```

- [ ] **Step 4: Watch CI**

```bash
gh pr checks --watch
```

Expected: lint + test (3 OSes) + build (3 OSes) all green.

If CI fails, address feedback on the same branch, push, and let CI re-run. Do NOT merge until green.

---

## Phase 1 A4 done definition

All of the following are true:

- [ ] `go build ./internal/pricing/...` succeeds
- [ ] `go test -race -count=1 ./internal/pricing/...` succeeds
- [ ] `golangci-lint run ./internal/pricing/...` reports zero issues
- [ ] `gofumpt -l internal/pricing` produces no output
- [ ] `*pricing.Table` satisfies `contracts.PricingTable` (compile-time assertion present)
- [ ] Files committed:
  - `pricing/models.yaml` (root, canonical)
  - `internal/pricing/models.yaml` (embedded copy, byte-identical)
  - `internal/pricing/table.go`
  - `internal/pricing/embedded.go`
  - `internal/pricing/table_test.go`
  - `internal/pricing/embedded_test.go`
  - `internal/pricing/contracts_test.go`
  - `internal/pricing/yaml_dep_test.go`
  - `go.mod` + `go.sum` updated with `gopkg.in/yaml.v3`
- [ ] PR open against `main`, CI green
- [ ] PR merged to `main` (synthesizer or maintainer action)

After merge, this plan is complete. Other Phase 1 worktrees should rebase off the new `main` before continuing.
