package run

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

const defaultHookPollInterval = 2 * time.Second

// QueryHookEventsStore is the narrow storage interface DBHookSource needs.
type QueryHookEventsStore interface {
	QuerySessions(ctx context.Context, q contracts.SessionQuery) ([]contracts.SessionTelemetry, error)
	QueryHookEventsForSession(ctx context.Context, sessionID string, since time.Time) ([]contracts.HookEvent, error)
	LatestHookEventID(ctx context.Context, sessionID string) (int64, error)
	QueryHookEventsForSessionAfterID(ctx context.Context, sessionID string, afterID int64) ([]contracts.HookEvent, error)
}

// DBHookSource implements HookSource by polling state.db. It is used when the
// supervisor runs without daemon-backed event delivery and as the Stop-event
// source for SSE-backed supervision.
type DBHookSource struct {
	Store        QueryHookEventsStore
	PollInterval time.Duration

	mu          sync.Mutex
	launchTime  map[string]time.Time
	sessionTime map[string]time.Time
}

// MarkLaunch records the supervisor launch boundary for profile so
// CurrentSessionID can avoid older unrelated sessions.
func (h *DBHookSource) MarkLaunch(profile string, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.launchTime == nil {
		h.launchTime = make(map[string]time.Time)
	}
	h.launchTime[profile] = at.UTC()
}

// CurrentSessionID returns the newest known session id for profile.
func (h *DBHookSource) CurrentSessionID(ctx context.Context, profile string) (string, error) {
	if h.Store == nil {
		return "", errors.New("DBHookSource store is nil")
	}
	query := contracts.SessionQuery{Profile: profile, Limit: 1}
	if since, ok := h.launchBoundary(profile); ok {
		query.Since = since
		query.Limit = 2
	}
	rows, err := h.Store.QuerySessions(ctx, query)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	if query.Limit == 2 && len(rows) > 1 {
		return "", fmt.Errorf("ambiguous current session for profile %q after supervisor launch", profile)
	}
	if !query.Since.IsZero() {
		h.markSessionBoundary(rows[0].Session, query.Since)
	}
	return rows[0].Session, nil
}

// WaitForStop polls hook_events until a Stop or StopFailure event lands for
// sessionID, or until ctx is canceled.
func (h *DBHookSource) WaitForStop(ctx context.Context, sessionID string) error {
	if h.Store == nil {
		return errors.New("DBHookSource store is nil")
	}
	if since, ok := h.sessionBoundary(sessionID); ok {
		return h.waitForStop(ctx, func(ctx context.Context) ([]contracts.HookEvent, error) {
			return h.Store.QueryHookEventsForSession(ctx, sessionID, since)
		})
	}
	afterID, err := h.Store.LatestHookEventID(ctx, sessionID)
	if err != nil {
		return err
	}
	return h.waitForStop(ctx, func(ctx context.Context) ([]contracts.HookEvent, error) {
		return h.Store.QueryHookEventsForSessionAfterID(ctx, sessionID, afterID)
	})
}

func (h *DBHookSource) waitForStop(ctx context.Context, query func(context.Context) ([]contracts.HookEvent, error)) error {
	ticker := time.NewTicker(h.pollInterval())
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		events, err := query(ctx)
		if err != nil {
			return err
		}
		for i := range events {
			if events[i].Event == "Stop" || events[i].Event == "StopFailure" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (h *DBHookSource) markSessionBoundary(sessionID string, since time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessionTime == nil {
		h.sessionTime = make(map[string]time.Time)
	}
	h.sessionTime[sessionID] = since
}

func (h *DBHookSource) sessionBoundary(sessionID string) (time.Time, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessionTime == nil {
		return time.Time{}, false
	}
	at, ok := h.sessionTime[sessionID]
	return at, ok
}

func (h *DBHookSource) launchBoundary(profile string) (time.Time, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.launchTime == nil {
		return time.Time{}, false
	}
	at, ok := h.launchTime[profile]
	return at, ok
}

func (h *DBHookSource) pollInterval() time.Duration {
	if h.PollInterval > 0 {
		return h.PollInterval
	}
	return defaultHookPollInterval
}
