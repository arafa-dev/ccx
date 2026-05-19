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
