package daemon

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/quota"
	"github.com/arafa-dev/ccx/internal/recstream"
	"github.com/arafa-dev/ccx/internal/storage"
)

func TestIngestProfileFlushesBufferedEventsBeforeScannerError(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	wantErr := errors.New("scan failed")
	profile := contracts.Profile{Name: "work", ConfigDir: t.TempDir()}
	deps := &runtimeDeps{
		Store: store,
		Scanner: scriptedScanner{
			event: contracts.Event{
				UUID:      "event-1",
				SessionID: "session-1",
				Timestamp: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
				Type:      "assistant",
				Project:   "ccx",
				Model:     "claude-sonnet-4-6",
				Usage:     &contracts.Usage{InputTokens: 100},
			},
			err: wantErr,
		},
	}

	if err := ingestProfile(ctx, deps, profile); !errors.Is(err, wantErr) {
		t.Fatalf("ingestProfile error = %v, want %v", err, wantErr)
	}
	rows, err := store.QueryUsage(ctx, contracts.UsageQuery{Profile: "work"})
	if err != nil {
		t.Fatalf("QueryUsage: %v", err)
	}
	if len(rows) != 1 || rows[0].Usage.InputTokens != 100 {
		t.Fatalf("usage rows = %+v, want flushed scanner event", rows)
	}
}

func TestObservePressurePublishesUpwardTransition(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	deps := newPressureTestDeps(t, ctx)
	defer func() { _ = deps.Close() }()

	workDir := filepath.Join(t.TempDir(), "work")
	personalDir := filepath.Join(t.TempDir(), "personal")
	for _, dir := range []string{workDir, personalDir} {
		if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	workProfile := contracts.Profile{
		Name:      "work",
		ConfigDir: workDir,
		Limits: contracts.ProfileLimits{
			PlanTier:        "max20",
			Caps5hTurns:     1,
			CapsWeeklyTurns: 10,
		},
	}
	if err := deps.Profiles.Add(ctx, workProfile); err != nil {
		t.Fatal(err)
	}
	if err := deps.Store.SaveProfile(ctx, workProfile); err != nil {
		t.Fatal(err)
	}
	personalProfile := contracts.Profile{
		Name:      "personal",
		ConfigDir: personalDir,
		Limits: contracts.ProfileLimits{
			PlanTier:        "max20",
			Caps5hTurns:     10,
			CapsWeeklyTurns: 10,
		},
	}
	if err := deps.Profiles.Add(ctx, personalProfile); err != nil {
		t.Fatal(err)
	}
	if err := deps.Store.SaveProfile(ctx, personalProfile); err != nil {
		t.Fatal(err)
	}
	if err := deps.Store.InsertHookEvent(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "Stop",
		Timestamp: now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	hub := recstream.NewHub()
	defer hub.Close()
	sub := hub.Subscribe(ctx)
	observePressure(
		ctx, deps,
		quota.Computer{Store: deps.Store, Now: func() time.Time { return now }},
		headroom.Evaluator{
			Store:   deps.Store,
			Pricing: testPricing{},
			Now:     func() time.Time { return now },
		},
		recstream.NewStateMachine(),
		hub,
		log.New(io.Discard, "", 0),
	)

	select {
	case ev := <-sub:
		if ev.Profile != "work" {
			t.Fatalf("Profile = %q, want work", ev.Profile)
		}
		if ev.Level != contracts.RecommendationHard {
			t.Fatalf("Level = %q, want hard", ev.Level)
		}
		if ev.Suggested != "personal" {
			t.Fatalf("Suggested = %q, want personal", ev.Suggested)
		}
		if ev.Quota5hPct != 100 {
			t.Fatalf("Quota5hPct = %v, want 100", ev.Quota5hPct)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recommendation event")
	}
}

func TestObservePressureSkipsProfilesWithoutPlanTier(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	deps := newPressureTestDeps(t, ctx)
	defer func() { _ = deps.Close() }()

	cfgDir := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := deps.Profiles.Add(ctx, contracts.Profile{Name: "work", ConfigDir: cfgDir}); err != nil {
		t.Fatal(err)
	}

	hub := recstream.NewHub()
	defer hub.Close()
	sub := hub.Subscribe(ctx)
	observePressure(
		ctx, deps,
		quota.Computer{Store: deps.Store, Now: func() time.Time { return now }},
		headroom.Evaluator{Store: deps.Store, Pricing: testPricing{}, Now: func() time.Time { return now }},
		recstream.NewStateMachine(),
		hub,
		log.New(io.Discard, "", 0),
	)

	select {
	case ev := <-sub:
		t.Fatalf("unexpected event: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPrimePressureSeedsStateWithoutPublishing(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	deps := newPressureTestDeps(t, ctx)
	defer func() { _ = deps.Close() }()

	cfgDir := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	workProfile := contracts.Profile{
		Name:      "work",
		ConfigDir: cfgDir,
		Limits: contracts.ProfileLimits{
			PlanTier:        "max20",
			Caps5hTurns:     1,
			CapsWeeklyTurns: 10,
		},
	}
	if err := deps.Profiles.Add(ctx, workProfile); err != nil {
		t.Fatal(err)
	}
	if err := deps.Store.SaveProfile(ctx, workProfile); err != nil {
		t.Fatal(err)
	}
	if err := deps.Store.InsertHookEvent(ctx, "work", contracts.HookEvent{
		Session:   "s1",
		Event:     "Stop",
		Timestamp: now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	computer := quota.Computer{Store: deps.Store, Now: func() time.Time { return now }}
	sm := recstream.NewStateMachine()
	primePressure(ctx, deps, computer, sm, nil)

	hub := recstream.NewHub()
	defer hub.Close()
	sub := hub.Subscribe(ctx)
	observePressure(
		ctx, deps,
		computer,
		headroom.Evaluator{Store: deps.Store, Pricing: testPricing{}, Now: func() time.Time { return now }},
		sm,
		hub,
		nil,
	)

	select {
	case ev := <-sub:
		t.Fatalf("unexpected event after priming same pressure state: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

type scriptedScanner struct {
	event contracts.Event
	err   error
}

func (s scriptedScanner) Scan(_ context.Context, _ contracts.Profile) (<-chan contracts.Event, <-chan error) {
	events := make(chan contracts.Event)
	errs := make(chan error)
	go func() {
		defer close(events)
		defer close(errs)
		events <- s.event
		errs <- s.err
	}()
	return events, errs
}

func newPressureTestDeps(t *testing.T, ctx context.Context) *runtimeDeps {
	t.Helper()
	store, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := profile.NewManager(t.TempDir())
	if err != nil {
		_ = store.Close()
		t.Fatalf("NewManager: %v", err)
	}
	return &runtimeDeps{Store: store, Profiles: mgr}
}

type testPricing struct{}

func (testPricing) Cost(string, time.Time, contracts.Usage) (float64, error) {
	return 0, nil
}

func (testPricing) LastUpdated() time.Time {
	return time.Time{}
}
