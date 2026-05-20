package pricing_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/pricing"
)

func TestNewTableLoadsEmbeddedBaseline(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

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
