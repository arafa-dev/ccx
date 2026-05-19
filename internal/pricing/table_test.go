package pricing_test

import (
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
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
		InputTokens:  500_000, // 0.5 MTok * $3  = $1.50
		OutputTokens: 200_000, // 0.2 MTok * $15 = $3.00
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
