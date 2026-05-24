package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const turnEndingHookEventFilter = `
(
    event_name = 'Stop'
    OR (
        event_name = 'StopFailure'
        AND COALESCE(error, '') NOT IN ('authentication_failed', 'oauth_org_not_allowed')
    )
)
`

// QueryTurnsInWindow counts turn-ending hook events for a profile where
// since < event timestamp <= until.
func (s *Store) QueryTurnsInWindow(ctx context.Context, profileName string, since, until time.Time) (int, error) {
	const q = `
SELECT COUNT(*)
FROM hook_events
WHERE profile_name = ?
  AND ts > ?
  AND ts <= ?
  AND ` + turnEndingHookEventFilter

	var count int
	if err := s.db.QueryRowContext(ctx, q, profileName, since.UnixNano(), until.UnixNano()).Scan(&count); err != nil {
		return 0, fmt.Errorf("querying turns in window for %q: %w", profileName, err)
	}
	return count, nil
}

// QueryOldestTurnInWindow returns the oldest turn-ending hook event timestamp
// for a profile where since < event timestamp <= until.
func (s *Store) QueryOldestTurnInWindow(ctx context.Context, profileName string, since, until time.Time) (time.Time, error) {
	const q = `
SELECT MIN(ts)
FROM hook_events
WHERE profile_name = ?
  AND ts > ?
  AND ts <= ?
  AND ` + turnEndingHookEventFilter

	var nsec sql.NullInt64
	if err := s.db.QueryRowContext(ctx, q, profileName, since.UnixNano(), until.UnixNano()).Scan(&nsec); err != nil {
		return time.Time{}, fmt.Errorf("querying oldest turn in window for %q: %w", profileName, err)
	}
	if !nsec.Valid {
		return time.Time{}, nil
	}
	return time.Unix(0, nsec.Int64).UTC(), nil
}
