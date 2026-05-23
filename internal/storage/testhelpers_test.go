package storage

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

// TableExists reports whether a table with the given name exists in the
// SQLite schema. Test-only helper.
func (s *Store) TableExists(ctx context.Context, t *testing.T, name string) bool {
	t.Helper()
	var found string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&found)
	if err != nil {
		return false
	}
	return found == name
}

// IndexExists reports whether an index with the given name exists in the
// SQLite schema. Test-only helper.
func (s *Store) IndexExists(ctx context.Context, t *testing.T, name string) bool {
	t.Helper()
	var found string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, name,
	).Scan(&found)
	if err != nil {
		return false
	}
	return found == name
}

// ExecSQL executes raw SQL against the store. Test-only helper.
func (s *Store) ExecSQL(ctx context.Context, t *testing.T, sql string) {
	t.Helper()
	if _, err := s.db.ExecContext(ctx, sql); err != nil {
		t.Fatalf("ExecSQL: %v", err)
	}
}

// DropProfileLimitColumns removes limit columns from profiles when present.
// Test-only helper used to simulate DBs created by an older v2 migration.
func (s *Store) DropProfileLimitColumns(ctx context.Context, t *testing.T) {
	t.Helper()
	for _, column := range []string{
		"daily_token_budget",
		"weekly_token_budget",
		"monthly_usd_budget",
		"priority",
		"suggest_enabled",
		"rate_limit_cooldown",
	} {
		if !s.columnExists(ctx, t, "profiles", column) {
			continue
		}
		if _, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE profiles DROP COLUMN %s", column)); err != nil {
			t.Fatalf("dropping profiles.%s: %v", column, err)
		}
	}
}

func (s *Store) columnExists(ctx context.Context, t *testing.T, table, column string) bool {
	t.Helper()
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
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
			t.Fatalf("scanning table_info(%s): %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating table_info(%s): %v", table, err)
	}
	return false
}

// SchemaVersion returns the single row from the schema_version table.
// Test-only helper.
func (s *Store) SchemaVersion(ctx context.Context, t *testing.T) int {
	t.Helper()
	var v int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT version FROM schema_version LIMIT 1`,
	).Scan(&v); err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	return v
}

// CountEvents returns the number of rows in events for the given profile.
// Test-only helper.
func (s *Store) CountEvents(ctx context.Context, t *testing.T, profileName string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM events WHERE profile_name = ?`, profileName,
	).Scan(&n); err != nil {
		t.Fatalf("CountEvents: %v", err)
	}
	return n
}

// PragmaString reads a string-valued SQLite pragma. Test-only helper.
func (s *Store) PragmaString(ctx context.Context, t *testing.T, name string) string {
	t.Helper()
	var got string
	if err := s.db.QueryRowContext(ctx, fmt.Sprintf("PRAGMA %s", name)).Scan(&got); err != nil {
		t.Fatalf("PragmaString(%s): %v", name, err)
	}
	return got
}

// PragmaInt reads an integer-valued SQLite pragma. Test-only helper.
func (s *Store) PragmaInt(ctx context.Context, t *testing.T, name string) int {
	t.Helper()
	var got int
	if err := s.db.QueryRowContext(ctx, fmt.Sprintf("PRAGMA %s", name)).Scan(&got); err != nil {
		t.Fatalf("PragmaInt(%s): %v", name, err)
	}
	return got
}
