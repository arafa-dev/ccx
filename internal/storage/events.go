package storage

import (
	"context"
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// InsertEvents writes a batch of events under a single transaction. Duplicate
// (profile_name, event_uuid) pairs are silently skipped via ON CONFLICT DO
// NOTHING, which makes re-scanning safe and idempotent.
func (s *Store) InsertEvents(ctx context.Context, profileName string, events []contracts.Event) (retErr error) {
	if len(events) == 0 {
		return nil
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for %q: %w", profileName, err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback()
		}
	}()

	const q = `
INSERT INTO events (
    profile_name, session_id, event_uuid, ts, project, model,
    input_tokens, output_tokens, cache_read_tokens, cache_create_tokens
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(profile_name, event_uuid) DO NOTHING
`
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("prepare insert for %q: %w", profileName, err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range events {
		ev := events[i]
		var in, out, cr, cc int
		if ev.Usage != nil {
			in = ev.Usage.InputTokens
			out = ev.Usage.OutputTokens
			cr = ev.Usage.CacheReadTokens
			cc = ev.Usage.CacheCreateTokens
		}
		if _, execErr := stmt.ExecContext(
			ctx,
			profileName,
			ev.SessionID,
			ev.UUID,
			ev.Timestamp.UnixNano(),
			ev.Project,
			ev.Model,
			in,
			out,
			cr,
			cc,
		); execErr != nil {
			return fmt.Errorf("inserting event %q for %q: %w", ev.UUID, profileName, execErr)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit insert for %q: %w", profileName, err)
	}
	return nil
}
