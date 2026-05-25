package quotawire

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/storage"
)

func TestAdapterQuotaEmptyProfileListReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()
	adapter := newTestAdapter(t)

	rows, err := adapter.Quota(ctx, "")
	if err != nil {
		t.Fatalf("Quota: %v", err)
	}
	if rows == nil {
		t.Fatal("rows is nil, want empty slice")
	}
	if len(rows) != 0 {
		t.Fatalf("rows length = %d, want 0", len(rows))
	}
}

func TestAdapterQuotaMissingProfileFilterReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()
	adapter := newTestAdapter(t)
	addQuotaProfile(t, adapter.Profiles, "work")

	rows, err := adapter.Quota(ctx, "personal")
	if err != nil {
		t.Fatalf("Quota: %v", err)
	}
	if rows == nil {
		t.Fatal("rows is nil, want empty slice")
	}
	if len(rows) != 0 {
		t.Fatalf("rows length = %d, want 0", len(rows))
	}
}

func TestAdapterQuotaProfileFilterReturnsOnlyMatchingProfile(t *testing.T) {
	ctx := context.Background()
	adapter := newTestAdapter(t)
	addQuotaProfile(t, adapter.Profiles, "work")
	addQuotaProfile(t, adapter.Profiles, "personal")

	rows, err := adapter.Quota(ctx, "work")
	if err != nil {
		t.Fatalf("Quota: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows length = %d, want 1", len(rows))
	}
	if rows[0].Profile != "work" {
		t.Fatalf("profile = %q, want work", rows[0].Profile)
	}
	if rows[0].PlanTier != "pro" || rows[0].Window5h.Cap != 45 {
		t.Fatalf("quota row = %+v, want pro defaults", rows[0])
	}
}

func TestAdapterQuotaReturnsPerProfileComputeFailures(t *testing.T) {
	ctx := context.Background()
	adapter := newTestAdapter(t)
	addQuotaProfile(t, adapter.Profiles, "work")
	if err := adapter.Store.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	_, err := adapter.Quota(ctx, "work")
	if err == nil {
		t.Fatal("Quota returned nil error, want compute failure")
	}
	if !strings.Contains(err.Error(), "work") {
		t.Fatalf("error = %q, want profile context", err.Error())
	}
}

func newTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	store, err := storage.NewStore(ctx, filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := profile.NewManager(filepath.Join(root, "profiles"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return &Adapter{Store: store, Profiles: mgr}
}

func addQuotaProfile(t *testing.T, mgr *profile.Manager, name string) {
	t.Helper()
	if err := mgr.Add(context.Background(), contracts.Profile{
		Name:      name,
		ConfigDir: filepath.Join(t.TempDir(), name),
		Limits:    contracts.ProfileLimits{PlanTier: "pro"},
	}); err != nil {
		t.Fatalf("Add(%s): %v", name, err)
	}
}
