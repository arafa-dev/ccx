package recstream

import (
	"context"
	"sync"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
)

// Hub fans out RecommendationEvents to active subscribers.
type Hub struct {
	mu     sync.Mutex
	subs   map[chan contracts.RecommendationEvent]struct{}
	done   chan struct{}
	closed bool
}

// NewHub creates an empty recommendation event hub.
func NewHub() *Hub {
	return &Hub{
		subs: make(map[chan contracts.RecommendationEvent]struct{}),
		done: make(chan struct{}),
	}
}

// Subscribe registers a subscriber until ctx is canceled or the hub closes.
func (h *Hub) Subscribe(ctx context.Context) <-chan contracts.RecommendationEvent {
	ch := make(chan contracts.RecommendationEvent, 16)

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(ch)
		return ch
	}
	h.subs[ch] = struct{}{}
	done := h.done
	h.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-done:
			return
		}

		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
	}()

	return ch
}

// Publish broadcasts ev to subscribers without blocking on slow receivers.
func (h *Hub) Publish(ev contracts.RecommendationEvent) { //nolint:gocritic // Public API keeps event publishing value-based.
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Close closes all subscriber channels and prevents future publishing.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	close(h.done)
	for ch := range h.subs {
		close(ch)
	}
	h.subs = make(map[chan contracts.RecommendationEvent]struct{})
}

// StateMachine tracks the last pressure band observed per profile.
type StateMachine struct {
	mu   sync.Mutex
	last map[string]headroom.PressureLevel
}

// NewStateMachine creates a pressure-band state machine.
func NewStateMachine() *StateMachine {
	return &StateMachine{
		last: make(map[string]headroom.PressureLevel),
	}
}

// Observe records profile's current pressure and reports upward threshold crossings.
func (sm *StateMachine) Observe(profile string, pct float64) (bool, contracts.RecommendationLevel) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	level := headroom.PressureLevelFromPct(pct)
	prev := sm.last[profile]
	sm.last[profile] = level

	if level <= prev || level < headroom.PressureWarn {
		return false, ""
	}

	recLevel, ok := recommendationLevel(level)
	if !ok {
		return false, ""
	}
	return true, recLevel
}

func recommendationLevel(level headroom.PressureLevel) (contracts.RecommendationLevel, bool) {
	switch level {
	case headroom.PressureWarn:
		return contracts.RecommendationWarn, true
	case headroom.PressureSoft:
		return contracts.RecommendationSoft, true
	case headroom.PressureHard:
		return contracts.RecommendationHard, true
	default:
		return "", false
	}
}
