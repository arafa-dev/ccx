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
