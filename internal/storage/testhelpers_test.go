package storage

import (
	"context"
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
