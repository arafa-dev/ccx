package storage

import (
	"context"
	"database/sql"
	"fmt"
)

const currentSchemaVersion = 2

const migrationV2SQL = `
CREATE TABLE IF NOT EXISTS hook_events (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_name    TEXT NOT NULL,
    session_id      TEXT NOT NULL,
    event_name      TEXT NOT NULL,
    ts              INTEGER NOT NULL,
    transcript_path TEXT,
    cwd             TEXT,
    model           TEXT,
    source          TEXT,
    permission_mode TEXT,
    reason          TEXT,
    error           TEXT,
    error_details   TEXT,
    trigger         TEXT,
    FOREIGN KEY (profile_name) REFERENCES profiles(name) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS hook_events_profile_ts ON hook_events(profile_name, ts);
CREATE INDEX IF NOT EXISTS hook_events_session ON hook_events(profile_name, session_id);

CREATE TABLE IF NOT EXISTS sessions (
    profile_name    TEXT NOT NULL,
    session_id      TEXT NOT NULL,
    transcript_path TEXT,
    cwd             TEXT,
    model           TEXT,
    source          TEXT,
    permission_mode TEXT,
    started_at      INTEGER,
    ended_at        INTEGER,
    last_seen_at    INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown',
    end_reason      TEXT,
    failure_error   TEXT,
    failure_details TEXT,
    compact_count   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (profile_name, session_id),
    FOREIGN KEY (profile_name) REFERENCES profiles(name) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS sessions_profile_seen ON sessions(profile_name, last_seen_at);
CREATE INDEX IF NOT EXISTS sessions_status ON sessions(status);

CREATE TABLE IF NOT EXISTS profile_health (
    profile_name TEXT PRIMARY KEY,
    checked_at   INTEGER NOT NULL,
    auth_status  TEXT NOT NULL,
    auth_detail  TEXT,
    FOREIGN KEY (profile_name) REFERENCES profiles(name) ON DELETE CASCADE
);
`

// Migrate applies versioned schema migrations. Version 1 remains the embedded
// schema.sql contract; later versions are layered on top without dropping
// existing tables or data.
func (s *Store) Migrate(ctx context.Context) (retErr error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback()
		}
	}()

	version, err := schemaVersion(ctx, tx)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, currentSchemaVersion)
	}

	if version == 0 {
		if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
			return fmt.Errorf("applying schema v1: %w", err)
		}
		version, err = schemaVersion(ctx, tx)
		if err != nil {
			return fmt.Errorf("reading schema version after v1: %w", err)
		}
	}

	if version < 2 {
		if _, err := tx.ExecContext(ctx, migrationV2SQL); err != nil {
			return fmt.Errorf("applying schema v2: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM schema_version; INSERT INTO schema_version (version) VALUES (2);`); err != nil {
			return fmt.Errorf("recording schema v2: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func schemaVersion(ctx context.Context, tx *sql.Tx) (int, error) {
	var tableCount int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_version'`,
	).Scan(&tableCount); err != nil {
		return 0, err
	}
	if tableCount == 0 {
		return 0, nil
	}

	var version sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		return 0, err
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}
