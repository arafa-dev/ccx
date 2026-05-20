package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// Store is the SQLite-backed implementation of contracts.Store. It is safe
// for concurrent use: reads run in parallel against WAL-mode SQLite, while
// writes are serialized through writeMu (SQLite is single-writer).
type Store struct {
	db      *sql.DB
	writeMu sync.Mutex

	closeOnce sync.Once
	closeErr  error
}

// NewStore opens (or creates) a SQLite database at the given path. Use the
// literal string ":memory:" for an in-memory database (useful for tests).
// The returned Store has not yet been migrated; callers should run
// (*Store).Migrate before issuing CRUD calls.
func NewStore(ctx context.Context, dbPath string) (*Store, error) {
	dsn := buildDSN(dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite %q: %w", dbPath, err)
	}

	// Single open connection for :memory: so all callers see the same DB.
	// File-backed DBs benefit from a small pool but are still single-writer.
	if dbPath == ":memory:" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(8)
	}
	db.SetMaxIdleConns(2)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging sqlite %q: %w", dbPath, err)
	}

	return &Store{db: db}, nil
}

// Close releases the underlying database handle. Safe to call multiple times;
// returns the same error on subsequent calls.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()

		s.closeErr = s.db.Close()
	})
	return s.closeErr
}

// buildDSN constructs the modernc.org/sqlite connection string with the
// pragmas we require: WAL journaling, foreign keys ON, synchronous NORMAL.
func buildDSN(dbPath string) string {
	// In-memory databases must not be URL-encoded; they need the literal
	// ":memory:" form. The modernc.org/sqlite driver accepts the raw form
	// with query parameters appended.
	const pragmas = "_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"

	if dbPath == ":memory:" {
		return ":memory:?" + pragmas
	}
	return "file:" + dbPath + "?" + pragmas
}
