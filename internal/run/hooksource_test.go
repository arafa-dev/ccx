package run_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/run"
)

func TestDBHookSourceCurrentSessionIDReturnsNewestSession(t *testing.T) {
	store := &fakeHookEventStore{
		sessions: []contracts.SessionTelemetry{
			{Profile: "work", Session: "sid-new", LastSeenAt: time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)},
			{Profile: "work", Session: "sid-old", LastSeenAt: time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC)},
		},
	}
	hooks := &run.DBHookSource{Store: store}

	got, err := hooks.CurrentSessionID(context.Background(), "work")
	if err != nil {
		t.Fatalf("CurrentSessionID: %v", err)
	}
	if got != "sid-new" {
		t.Fatalf("CurrentSessionID = %q, want sid-new", got)
	}
	if store.sessionQuery.Profile != "work" || store.sessionQuery.Limit != 1 {
		t.Fatalf("session query = %+v, want profile work limit 1", store.sessionQuery)
	}
}

func TestDBHookSourceCurrentSessionIDUsesLaunchBoundary(t *testing.T) {
	launch := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	store := &fakeHookEventStore{
		sessions: []contracts.SessionTelemetry{
			{Profile: "work", Session: "sid-before", LastSeenAt: launch.Add(-time.Second)},
			{Profile: "work", Session: "sid-current", LastSeenAt: launch.Add(time.Second)},
		},
	}
	hooks := &run.DBHookSource{Store: store}
	hooks.MarkLaunch("work", launch)

	got, err := hooks.CurrentSessionID(context.Background(), "work")
	if err != nil {
		t.Fatalf("CurrentSessionID: %v", err)
	}
	if got != "sid-current" {
		t.Fatalf("CurrentSessionID = %q, want sid-current", got)
	}
	if store.sessionQuery.Since != launch || store.sessionQuery.Limit != 2 {
		t.Fatalf("session query = %+v, want launch boundary and ambiguity limit", store.sessionQuery)
	}
}

func TestDBHookSourceCurrentSessionIDErrorsOnAmbiguousLaunchSessions(t *testing.T) {
	launch := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	store := &fakeHookEventStore{
		sessions: []contracts.SessionTelemetry{
			{Profile: "work", Session: "sid-a", LastSeenAt: launch.Add(time.Second)},
			{Profile: "work", Session: "sid-b", LastSeenAt: launch.Add(2 * time.Second)},
		},
	}
	hooks := &run.DBHookSource{Store: store}
	hooks.MarkLaunch("work", launch)

	_, err := hooks.CurrentSessionID(context.Background(), "work")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("CurrentSessionID error = %v, want ambiguous session error", err)
	}
}

func TestDBHookSourceWaitForStopReturnsWhenStopAppears(t *testing.T) {
	firstPoll := make(chan struct{})
	store := &fakeHookEventStore{firstHookQuery: firstPoll}
	hooks := &run.DBHookSource{
		Store:        store,
		PollInterval: 5 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- hooks.WaitForStop(ctx, "sid-1")
	}()

	<-firstPoll
	store.addHookEvent(contracts.HookEvent{
		Session:   "sid-1",
		Event:     "Stop",
		Timestamp: time.Now().UTC(),
	})

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("WaitForStop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForStop did not return after Stop event")
	}
}

func TestDBHookSourceWaitForStopUsesHookArrivalOrder(t *testing.T) {
	firstPoll := make(chan struct{})
	store := &fakeHookEventStore{firstHookQuery: firstPoll}
	hooks := &run.DBHookSource{
		Store:        store,
		PollInterval: 5 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- hooks.WaitForStop(ctx, "sid-1")
	}()

	<-firstPoll
	store.addHookEvent(contracts.HookEvent{
		Session:   "sid-1",
		Event:     "Stop",
		Timestamp: time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC),
	})

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("WaitForStop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForStop did not use hook insertion order")
	}
}

func TestDBHookSourceWaitForStopReturnsForStopAfterLaunchBeforeHardEvent(t *testing.T) {
	launch := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	store := &fakeHookEventStore{
		sessions: []contracts.SessionTelemetry{
			{Profile: "work", Session: "sid-current", LastSeenAt: launch.Add(time.Second)},
		},
	}
	hooks := &run.DBHookSource{
		Store:        store,
		PollInterval: 50 * time.Millisecond,
	}
	hooks.MarkLaunch("work", launch)

	sessionID, err := hooks.CurrentSessionID(context.Background(), "work")
	if err != nil {
		t.Fatalf("CurrentSessionID: %v", err)
	}
	store.addHookEvent(contracts.HookEvent{
		Session:   sessionID,
		Event:     "Stop",
		Timestamp: launch.Add(2 * time.Second),
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := hooks.WaitForStop(ctx, sessionID); err != nil {
		t.Fatalf("WaitForStop: %v", err)
	}
}

func TestDBHookSourceWaitForStopReturnsContextCanceledPromptly(t *testing.T) {
	store := &fakeHookEventStore{}
	hooks := &run.DBHookSource{
		Store:        store,
		PollInterval: 50 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		result <- hooks.WaitForStop(ctx, "sid-1")
	}()
	time.Sleep(5 * time.Millisecond)
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitForStop error = %v, want context.Canceled", err)
		}
	case <-time.After(25 * time.Millisecond):
		t.Fatal("WaitForStop did not return promptly after cancellation")
	}
}

func TestOpenSSEStreamsRecommendationEvents(t *testing.T) {
	want := contracts.RecommendationEvent{
		Profile:        "work",
		Level:          contracts.RecommendationHard,
		Reason:         "cap reached",
		Suggested:      "personal",
		Quota5hPct:     101,
		QuotaWeeklyPct: 20,
		Timestamp:      time.Date(2026, 5, 25, 11, 0, 0, 0, time.UTC),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/recommendations/live" {
			t.Fatalf("path = %q, want /api/recommendations/live", r.URL.Path)
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("marshal recommendation: %v", err)
		}
		_, _ = fmt.Fprintf(w, "event: recommendation\ndata: %s\n\n", data)
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := run.OpenSSE(ctx, server.URL+"/api/recommendations/live")
	if err != nil {
		t.Fatalf("OpenSSE: %v", err)
	}

	select {
	case got := <-events:
		if got.Profile != want.Profile || got.Level != want.Level || got.Suggested != want.Suggested ||
			!got.Timestamp.Equal(want.Timestamp) {
			t.Fatalf("event = %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SSE recommendation")
	}
}

type fakeHookEventStore struct {
	mu             sync.Mutex
	sessions       []contracts.SessionTelemetry
	hookEvents     []contracts.HookEvent
	hookIDs        []int64
	sessionQuery   contracts.SessionQuery
	firstHookQuery chan struct{}
	hookQueries    int
	nextHookID     int64
}

func (s *fakeHookEventStore) QuerySessions(_ context.Context, q contracts.SessionQuery) ([]contracts.SessionTelemetry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionQuery = q
	filtered := make([]contracts.SessionTelemetry, 0, len(s.sessions))
	for _, session := range s.sessions {
		if q.Profile != "" && session.Profile != q.Profile {
			continue
		}
		if !q.Since.IsZero() && session.LastSeenAt.Before(q.Since) {
			continue
		}
		filtered = append(filtered, session)
	}
	if len(filtered) == 0 {
		return []contracts.SessionTelemetry{}, nil
	}
	if q.Limit > 0 && q.Limit < len(filtered) {
		filtered = filtered[:q.Limit]
	}
	return append([]contracts.SessionTelemetry(nil), filtered...), nil
}

func (s *fakeHookEventStore) QueryHookEventsForSession(_ context.Context, sessionID string, since time.Time) ([]contracts.HookEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hookQueries++
	if s.hookQueries == 1 && s.firstHookQuery != nil {
		close(s.firstHookQuery)
	}
	out := make([]contracts.HookEvent, 0)
	for _, ev := range s.hookEvents {
		if ev.Session == sessionID && !ev.Timestamp.Before(since) {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (s *fakeHookEventStore) LatestHookEventID(_ context.Context, sessionID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest int64
	for i, ev := range s.hookEvents {
		if ev.Session == sessionID && s.hookIDs[i] > latest {
			latest = s.hookIDs[i]
		}
	}
	return latest, nil
}

func (s *fakeHookEventStore) QueryHookEventsForSessionAfterID(_ context.Context, sessionID string, afterID int64) ([]contracts.HookEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hookQueries++
	if s.hookQueries == 1 && s.firstHookQuery != nil {
		close(s.firstHookQuery)
	}
	out := make([]contracts.HookEvent, 0)
	for i, ev := range s.hookEvents {
		if ev.Session == sessionID && s.hookIDs[i] > afterID {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (s *fakeHookEventStore) addHookEvent(ev contracts.HookEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextHookID++
	s.hookIDs = append(s.hookIDs, s.nextHookID)
	s.hookEvents = append(s.hookEvents, ev)
}
