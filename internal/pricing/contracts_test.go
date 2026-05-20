package pricing_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/pricing"
)

// Compile-time guarantee that *Table implements contracts.PricingTable.
var _ contracts.PricingTable = (*pricing.Table)(nil)

func TestTableImplementsContract(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	tbl, err := pricing.NewTable()
	if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	var iface contracts.PricingTable = tbl
	if iface.LastUpdated().IsZero() {
		t.Errorf("LastUpdated via interface should not be zero on embedded table")
	}
}
