package storage

import (
	"context"
	"database/sql"
	"fmt"
)

const currentSchemaVersion = 2

var profileLimitColumns = []struct {
	name string
	def  string
}{
	{name: "daily_token_budget", def: "daily_token_budget INTEGER"},
	{name: "weekly_token_budget", def: "weekly_token_budget INTEGER"},
	{name: "monthly_usd_budget", def: "monthly_usd_budget REAL"},
	{name: "priority", def: "priority INTEGER"},
	{name: "suggest_enabled", def: "suggest_enabled INTEGER"},
	{name: "rate_limit_cooldown", def: "rate_limit_cooldown TEXT"},
}

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
    failure_at      INTEGER,
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

	if err := ensureProfileLimitColumns(ctx, tx); err != nil {
		return fmt.Errorf("ensuring profile limit columns: %w", err)
	}
	if err := ensureSessionFailureAtColumn(ctx, tx); err != nil {
		return fmt.Errorf("ensuring session failure timestamp column: %w", err)
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

func ensureSessionFailureAtColumn(ctx context.Context, tx *sql.Tx) error {
	exists, err := tableColumnExists(ctx, tx, "sessions", "failure_at")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE sessions ADD COLUMN failure_at INTEGER"); err != nil {
		return fmt.Errorf("adding sessions.failure_at: %w", err)
	}
	return nil
}

func ensureProfileLimitColumns(ctx context.Context, tx *sql.Tx) error {
	for _, column := range profileLimitColumns {
		exists, err := tableColumnExists(ctx, tx, "profiles", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := tx.ExecContext(ctx, "ALTER TABLE profiles ADD COLUMN "+column.def); err != nil {
			return fmt.Errorf("adding profiles.%s: %w", column.name, err)
		}
	}
	return nil
}

func tableColumnExists(ctx context.Context, tx *sql.Tx, tableName, columnName string) (bool, error) {
	query, err := tableInfoQuery(tableName)
	if err != nil {
		return false, err
	}
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func tableInfoQuery(tableName string) (string, error) {
	switch tableName {
	case "profiles":
		return "PRAGMA table_info(profiles)", nil
	case "sessions":
		return "PRAGMA table_info(sessions)", nil
	default:
		return "", fmt.Errorf("unsupported table %q", tableName)
	}
}
