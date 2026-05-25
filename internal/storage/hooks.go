package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

const hookEventSelectColumns = `
    profile_name,
    session_id,
    event_name,
    ts,
    COALESCE(transcript_path, ''),
    COALESCE(cwd, ''),
    COALESCE(model, ''),
    COALESCE(source, ''),
    COALESCE(permission_mode, ''),
    COALESCE(reason, ''),
    COALESCE(error, ''),
    COALESCE(error_details, ''),
    COALESCE(trigger, '')
`

// QueryHookEventsForSession returns hook events for a session at or after
// since, ordered oldest first.
func (s *Store) QueryHookEventsForSession(ctx context.Context, sessionID string, since time.Time) ([]contracts.HookEvent, error) {
	query := `
SELECT
` + hookEventSelectColumns + `
FROM hook_events
WHERE session_id = ? AND ts >= ?
ORDER BY ts ASC
`
	rows, err := s.db.QueryContext(ctx, query, sessionID, unixNano(since))
	if err != nil {
		return nil, fmt.Errorf("querying hook events for session %q: %w", sessionID, err)
	}
	defer func() { _ = rows.Close() }()

	events, err := scanHookEvents(rows)
	if err != nil {
		return nil, fmt.Errorf("querying hook events for session %q: %w", sessionID, err)
	}
	return events, nil
}

// LatestHookEventID returns the latest inserted hook event id for sessionID, or
// zero when the session has no hook events.
func (s *Store) LatestHookEventID(ctx context.Context, sessionID string) (int64, error) {
	const query = `
SELECT COALESCE(MAX(id), 0)
FROM hook_events
WHERE session_id = ?
`
	var id int64
	if err := s.db.QueryRowContext(ctx, query, sessionID).Scan(&id); err != nil {
		return 0, fmt.Errorf("querying latest hook event id for session %q: %w", sessionID, err)
	}
	return id, nil
}

// QueryHookEventsForSessionAfterID returns hook events for a session inserted
// after afterID, ordered by insertion order.
func (s *Store) QueryHookEventsForSessionAfterID(ctx context.Context, sessionID string, afterID int64) ([]contracts.HookEvent, error) {
	query := `
SELECT
` + hookEventSelectColumns + `
FROM hook_events
WHERE session_id = ? AND id > ?
ORDER BY id ASC
`
	rows, err := s.db.QueryContext(ctx, query, sessionID, afterID)
	if err != nil {
		return nil, fmt.Errorf("querying hook events after id for session %q: %w", sessionID, err)
	}
	defer func() { _ = rows.Close() }()

	events, err := scanHookEvents(rows)
	if err != nil {
		return nil, fmt.Errorf("querying hook events after id for session %q: %w", sessionID, err)
	}
	return events, nil
}

func scanHookEvents(rows *sql.Rows) ([]contracts.HookEvent, error) {
	out := make([]contracts.HookEvent, 0)
	for rows.Next() {
		var (
			ev contracts.HookEvent
			ns int64
		)
		if err := rows.Scan(
			&ev.Profile,
			&ev.Session,
			&ev.Event,
			&ns,
			&ev.Transcript,
			&ev.CWD,
			&ev.Model,
			&ev.Source,
			&ev.Permission,
			&ev.Reason,
			&ev.Error,
			&ev.ErrorDetails,
			&ev.Trigger,
		); err != nil {
			return nil, fmt.Errorf("scanning hook event row: %w", err)
		}
		ev.Timestamp = time.Unix(0, ns).UTC()
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating hook event rows: %w", err)
	}
	return out, nil
}
