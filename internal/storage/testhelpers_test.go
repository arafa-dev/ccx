package storage

import (
	"context"
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
