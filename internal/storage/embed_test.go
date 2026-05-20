package storage_test

import (
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/storage"
)

func TestSchemaSQLIsEmbedded(t *testing.T) {
	got := storage.SchemaSQL()
	if got == "" {
		t.Fatal("SchemaSQL() returned empty string; schema.sql not embedded")
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS profiles",
		"CREATE TABLE IF NOT EXISTS events",
		"CREATE TABLE IF NOT EXISTS scan_cursors",
		"CREATE TABLE IF NOT EXISTS schema_version",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("embedded schema is missing %q", want)
		}
	}
}
