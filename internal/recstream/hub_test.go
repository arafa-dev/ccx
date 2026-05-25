package recstream_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/recstream"
)

func TestHubPublishToOneSubscriber(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	h := recstream.NewHub()
	defer h.Close()

	sub := h.Subscribe(ctx)
	go h.Publish(contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationWarn})
	select {
	case ev := <-sub:
		if ev.Profile != "work" || ev.Level != contracts.RecommendationWarn {
			t.Errorf("unexpected event: %+v", ev)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for event")
	}
}

func TestHubFanOutToManySubscribers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	h := recstream.NewHub()
	defer h.Close()

	a := h.Subscribe(ctx)
	b := h.Subscribe(ctx)
	go h.Publish(contracts.RecommendationEvent{Profile: "x", Level: contracts.RecommendationSoft})
	for _, ch := range []<-chan contracts.RecommendationEvent{a, b} {
		select {
		case ev := <-ch:
			if ev.Level != contracts.RecommendationSoft {
				t.Errorf("got %+v", ev)
			}
		case <-ctx.Done():
			t.Fatal("subscriber missed event")
		}
	}
}

func TestHubPublishDoesNotBlockOnFullSubscriber(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := recstream.NewHub()
	defer h.Close()

	slow := h.Subscribe(ctx)
	for i := 0; i < 16; i++ {
		h.Publish(contracts.RecommendationEvent{Profile: "slow", Level: contracts.RecommendationWarn})
	}
	fast := h.Subscribe(ctx)

	done := make(chan struct{})
	go func() {
		h.Publish(contracts.RecommendationEvent{Profile: "fast", Level: contracts.RecommendationHard})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked on full subscriber")
	}

	select {
	case ev := <-fast:
		if ev.Profile != "fast" {
			t.Fatalf("unexpected fast subscriber event: %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("fast subscriber missed published event")
	}

	select {
	case <-slow:
	default:
	}
}

func TestStateMachineOnlyEmitsUpwardTransitions(t *testing.T) {
	sm := recstream.NewStateMachine()
	cases := []struct {
		profile string
		pct     float64
		emit    bool
		level   contracts.RecommendationLevel
	}{
		{"work", 50, false, ""},
		{"work", 80, true, contracts.RecommendationWarn},
		{"work", 95, true, contracts.RecommendationSoft},
		{"work", 92, false, ""},
		{"work", 80, false, ""},
		{"work", 100, true, contracts.RecommendationHard},
		{"work", 100, false, ""},
		{"work", 50, false, ""},
	}
	for _, tc := range cases {
		emit, level := sm.Observe(tc.profile, tc.pct)
		if emit != tc.emit {
			t.Errorf("Observe(%s, %v): emit = %v, want %v", tc.profile, tc.pct, emit, tc.emit)
		}
		if emit && level != tc.level {
			t.Errorf("Observe(%s, %v): level = %v, want %v", tc.profile, tc.pct, level, tc.level)
		}
	}
}

func TestStateMachineIsolatedPerProfile(t *testing.T) {
	sm := recstream.NewStateMachine()
	if emit, _ := sm.Observe("a", 80); !emit {
		t.Error("a->warn should emit")
	}
	if emit, _ := sm.Observe("b", 80); !emit {
		t.Error("b->warn should emit (different profile)")
	}
}

func TestStateMachineConcurrentObserveIsSafe(t *testing.T) {
	sm := recstream.NewStateMachine()
	var wg sync.WaitGroup
	var emits atomic.Int64
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(profile string) {
			defer wg.Done()
			for _, pct := range []float64{50, 80, 95, 80, 100, 50} {
				emit, _ := sm.Observe(profile, pct)
				if emit {
					emits.Add(1)
				}
			}
		}("profile")
	}
	wg.Wait()
	if emits.Load() == 0 {
		t.Fatal("expected at least one upward transition")
	}
}

func TestHubSubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	h := recstream.NewHub()
	h.Close()
	sub := h.Subscribe(context.Background())
	select {
	case _, ok := <-sub:
		if ok {
			t.Error("expected closed channel; got a value")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Subscribe-after-Close should return a closed channel, not a hanging one")
	}
}

func TestHubCloseThenCtxCancelDoesNotDoubleClose(t *testing.T) {
	for i := 0; i < 100; i++ {
		h := recstream.NewHub()
		ctx, cancel := context.WithCancel(context.Background())
		_ = h.Subscribe(ctx)
		done := make(chan struct{})
		go func() {
			h.Close()
			close(done)
		}()
		cancel()
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Close did not return")
		}
	}
}

func TestHubConcurrentSubscribeCloseIsSafe(t *testing.T) {
	h := recstream.NewHub()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sub := h.Subscribe(ctx)
			select {
			case <-sub:
			case <-time.After(10 * time.Millisecond):
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		h.Close()
	}()
	close(start)
	wg.Wait()
}
