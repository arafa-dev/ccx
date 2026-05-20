package storage

import (
	_ "embed"

	// Register the modernc.org/sqlite driver under the name "sqlite".
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// SchemaSQL returns the embedded SQLite schema as a string. Exposed for tests
// and tooling; callers wanting to apply it should use (*Store).Migrate.
func SchemaSQL() string {
	return schemaSQL
}
