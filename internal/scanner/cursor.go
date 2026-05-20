package scanner

import (
	"context"
	"sync"
)

// Cursor is the per-file position checkpoint used for incremental scanning.
// Offset is the next byte to read; Inode is the underlying file inode at the
// time the offset was recorded. If the inode changes on a subsequent scan,
// the file is assumed to have been rotated or replaced and the scan restarts
// from offset 0.
type Cursor struct {
	Offset int64
	Inode  uint64
}

// CursorStore persists per-file scan checkpoints. In Phase 2 it is backed by
// internal/storage; unit tests use NewMemoryCursorStore.
type CursorStore interface {
	// Get returns the saved cursor for (profile, file). If absent, returns
	// the zero-value Cursor and a nil error.
	Get(ctx context.Context, profile, file string) (Cursor, error)
	// Set persists the cursor for (profile, file).
	Set(ctx context.Context, profile, file string, c Cursor) error
}

// memoryCursorStore is an in-memory CursorStore for tests.
type memoryCursorStore struct {
	mu   sync.Mutex
	data map[string]Cursor
}

// NewMemoryCursorStore returns a CursorStore backed by a sync-protected map.
// Suitable for unit tests and short-lived processes; not durable.
func NewMemoryCursorStore() CursorStore {
	return &memoryCursorStore{data: map[string]Cursor{}}
}

func (m *memoryCursorStore) key(profile, file string) string {
	return profile + "\x00" + file
}

func (m *memoryCursorStore) Get(_ context.Context, profile, file string) (Cursor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.data[m.key(profile, file)], nil
}

func (m *memoryCursorStore) Set(_ context.Context, profile, file string, c Cursor) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[m.key(profile, file)] = c
	return nil
}
