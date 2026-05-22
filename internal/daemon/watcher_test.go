package daemon

import (
	"context"
	"sync"
	"testing"
	"time"
)

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
