package storage

import (
	"context"
	"fmt"
)

// Migrate applies the embedded schema. Safe to call multiple times because
// every statement uses CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS
// / INSERT OR IGNORE. For v0.1 there is exactly one schema version; future
// versions will run additional statements gated on the schema_version row.
func (s *Store) Migrate(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("applying schema: %w", err)
	}
	return nil
}
