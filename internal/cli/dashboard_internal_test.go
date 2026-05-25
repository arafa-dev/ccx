package cli

import (
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestDashboardQuotaProviderRejectsNonStorageStore(t *testing.T) {
	_, err := dashboardQuotaProvider(&Deps{Store: dashboardQuotaStore{}})
	if err == nil {
		t.Fatal("dashboardQuotaProvider returned nil error, want type assertion error")
	}
	if !strings.Contains(err.Error(), "*storage.Store") {
		t.Fatalf("error = %q, want storage.Store guidance", err.Error())
	}
}

type dashboardQuotaStore struct {
	contracts.Store
}
