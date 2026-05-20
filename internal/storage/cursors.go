package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetCursor returns the last recorded (offset, inode) for the JSONL file at
// filePath under profileName. If no row exists, returns (0, 0, nil) — an
// unknown cursor is not an error; it just means "start from the beginning."
func (s *Store) GetCursor(ctx context.Context, profileName, filePath string) (offset int64, inode uint64, err error) {
	var (
		storedOffset int64
		storedInode  sql.NullInt64
	)
	err = s.db.QueryRowContext(
		ctx,
		`SELECT offset, inode FROM scan_cursors WHERE profile_name = ? AND file_path = ?`,
		profileName, filePath,
	).Scan(&storedOffset, &storedInode)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("get cursor for %q %q: %w", profileName, filePath, err)
	}
	if storedInode.Valid {
		inode = uint64(storedInode.Int64) //nolint:gosec // intentional SQLite signed integer roundtrip.
	}
	return storedOffset, inode, nil
}

// SetCursor upserts the (offset, inode) for the JSONL file at filePath under
// profileName. Subsequent calls overwrite the previous row.
func (s *Store) SetCursor(ctx context.Context, profileName, filePath string, offset int64, inode uint64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	const q = `
INSERT INTO scan_cursors (profile_name, file_path, offset, inode)
VALUES (?, ?, ?, ?)
ON CONFLICT(profile_name, file_path) DO UPDATE SET
    offset = excluded.offset,
    inode  = excluded.inode
`
	if _, err := s.db.ExecContext(
		ctx, q,
		profileName, filePath, offset, int64(inode), //nolint:gosec // intentional SQLite signed integer roundtrip.
	); err != nil {
		return fmt.Errorf("set cursor for %q %q: %w", profileName, filePath, err)
	}
	return nil
}
