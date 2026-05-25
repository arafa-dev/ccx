package daemon

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/storage"
	"github.com/fsnotify/fsnotify"
)

func TestRefreshProfilesRemovesDeletedProfilesAndWatches(t *testing.T) {
	ctx := context.Background()
	mgr, err := profile.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	workCfg := filepath.Join(t.TempDir(), "work")
	sideCfg := filepath.Join(t.TempDir(), "side")
	for _, cfg := range []string{workCfg, sideCfg} {
		if err := os.MkdirAll(filepath.Join(cfg, "projects", "repo"), 0o700); err != nil {
			t.Fatalf("MkdirAll(%s): %v", cfg, err)
		}
	}
	if err := mgr.Add(ctx, contracts.Profile{Name: "work", ConfigDir: workCfg}); err != nil {
		t.Fatalf("Add work: %v", err)
	}
	if err := mgr.Add(ctx, contracts.Profile{Name: "side", ConfigDir: sideCfg}); err != nil {
		t.Fatalf("Add side: %v", err)
	}

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer func() { _ = fsWatcher.Close() }()
	w := &profileWatcher{
		deps:         &runtimeDeps{Profiles: mgr},
		logger:       log.New(io.Discard, "", 0),
		fs:           fsWatcher,
		profiles:     map[string]contracts.Profile{},
		projectsDirs: map[string]string{},
		watched:      map[string]struct{}{},
		timers:       map[string]*time.Timer{},
	}
	w.refreshProfiles(ctx)
	workRoot := filepath.Clean(filepath.Join(workCfg, "projects"))
	sideRoot := filepath.Clean(filepath.Join(sideCfg, "projects"))
	if _, ok := w.profiles["work"]; !ok {
		t.Fatal("work profile missing after initial refresh")
	}
	if _, ok := w.watched[workRoot]; !ok {
		t.Fatalf("work root %q not watched after initial refresh", workRoot)
	}

	if err := mgr.Remove(ctx, "work"); err != nil {
		t.Fatalf("Remove work: %v", err)
	}
	w.refreshProfiles(ctx)
	if _, ok := w.profiles["work"]; ok {
		t.Fatal("deleted work profile still cached after refresh")
	}
	if _, ok := w.projectsDirs["work"]; ok {
		t.Fatal("deleted work projects dir still cached after refresh")
	}
	if _, ok := w.watched[workRoot]; ok {
		t.Fatalf("deleted work root %q still watched after refresh", workRoot)
	}
	if got := w.profileForPath(filepath.Join(workRoot, "repo", "session.jsonl")); got != "" {
		t.Fatalf("profileForPath(deleted work path) = %q, want empty", got)
	}
	if _, ok := w.profiles["side"]; !ok {
		t.Fatal("side profile should remain cached")
	}
	if _, ok := w.watched[sideRoot]; !ok {
		t.Fatalf("side root %q should remain watched", sideRoot)
	}
}

func TestScanWorkerCoalescesRequestsAndRunsOneAtATime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	stats := &scanWorkerStats{}
	worker := newScanWorker(ctx, func(context.Context) {
		stats.enter()
		started <- struct{}{}
		<-release
		stats.leave()
	}, func(context.Context, string) {
		stats.enter()
		started <- struct{}{}
		<-release
		stats.leave()
	})
	defer worker.stop()

	worker.requestAll()
	<-started
	for i := 0; i < 25; i++ {
		worker.requestAll()
		worker.requestProfile("work")
	}
	close(release)

	eventually(t, func() bool {
		return stats.totalScans() == 2
	})
	// Hold briefly after the coalesced follow-up to catch accidental extra
	// scans from pending profile requests racing behind the all-profile scan.
	time.Sleep(50 * time.Millisecond)
	if got := stats.totalScans(); got != 2 {
		t.Fatalf("total scans = %d, want exactly active plus one coalesced follow-up", got)
	}
	if got := stats.maxActiveScans(); got != 1 {
		t.Fatalf("max concurrent scans = %d, want 1", got)
	}
}

func TestScanWorkerStopWaitsForActiveScan(t *testing.T) {
	ctx := context.Background()
	started := make(chan struct{})
	release := make(chan struct{})
	worker := newScanWorker(ctx, func(context.Context) {
		close(started)
		<-release
	}, func(context.Context, string) {})

	worker.requestAll()
	<-started

	stopped := make(chan struct{})
	go func() {
		worker.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("worker stopped before active scan completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after active scan completed")
	}
}

func TestProfileWatcherRunsAfterIngestForScanPaths(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := profile.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	p := contracts.Profile{Name: "work", ConfigDir: t.TempDir()}
	if err := mgr.Add(ctx, p); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var calls atomic.Int64
	w := &profileWatcher{
		deps: &runtimeDeps{
			Store:    store,
			Profiles: mgr,
			Scanner:  emptyScanner{},
		},
		logger:   log.New(io.Discard, "", 0),
		profiles: map[string]contracts.Profile{},
		afterIngest: func(context.Context) {
			calls.Add(1)
		},
	}

	w.scanAll(ctx)
	if got := calls.Load(); got != 1 {
		t.Fatalf("after scanAll calls = %d, want 1", got)
	}

	w.profiles = map[string]contracts.Profile{
		"work": p,
	}
	w.scanProfile(ctx, "work")
	if got := calls.Load(); got != 2 {
		t.Fatalf("after scanProfile calls = %d, want 2", got)
	}
}

type emptyScanner struct{}

func (emptyScanner) Scan(context.Context, contracts.Profile) (<-chan contracts.Event, <-chan error) {
	events := make(chan contracts.Event)
	errs := make(chan error)
	close(events)
	close(errs)
	return events, errs
}

type scanWorkerStats struct {
	mu        sync.Mutex
	active    int
	maxActive int
	total     int
}

func (s *scanWorkerStats) enter() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active++
	s.total++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
}

func (s *scanWorkerStats) leave() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
}

func (s *scanWorkerStats) totalScans() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

func (s *scanWorkerStats) maxActiveScans() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxActive
}
