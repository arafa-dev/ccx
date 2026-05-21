package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestPriceUsageRowsCollectsWarnings(t *testing.T) {
	rows := []contracts.UsageRow{{
		Profile: "work",
		Project: "project-a",
		Model:   "model-a",
		Day:     time.Now().UTC(),
		Usage:   contracts.Usage{InputTokens: 1},
	}}
	total, warnings := priceUsageRows(rows, usagePricing{err: errors.New("pricing unavailable")})
	if total != 0 {
		t.Fatalf("total = %f, want 0", total)
	}
	if rows[0].EstimatedUSD != 0 {
		t.Fatalf("EstimatedUSD = %f, want 0", rows[0].EstimatedUSD)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "model-a") ||
		!strings.Contains(warnings[0], "pricing unavailable") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestRenderUsageTableSortsProfiles(t *testing.T) {
	rows := []contracts.UsageRow{
		{Profile: "zeta", Usage: contracts.Usage{InputTokens: 1}, EstimatedUSD: 1},
		{Profile: "alpha", Usage: contracts.Usage{InputTokens: 1}, EstimatedUSD: 1},
	}
	var out bytes.Buffer
	if err := renderUsageTable(&out, rows, 2, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	alpha := strings.Index(got, "alpha")
	zeta := strings.Index(got, "zeta")
	if alpha < 0 || zeta < 0 {
		t.Fatalf("missing rows in output: %q", got)
	}
	if alpha > zeta {
		t.Fatalf("profiles not sorted:\n%s", got)
	}
}

type usagePricing struct {
	cost float64
	err  error
}

func (p usagePricing) Cost(_ string, _ time.Time, _ contracts.Usage) (float64, error) {
	return p.cost, p.err
}

func (p usagePricing) LastUpdated() time.Time {
	return time.Time{}
}
