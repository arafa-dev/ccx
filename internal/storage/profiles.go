package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// SaveProfile inserts or replaces a profile by name. Timestamps are stored
// as Unix nanoseconds for monotonic comparison and to avoid timezone drift.
func (s *Store) SaveProfile(ctx context.Context, p contracts.Profile) error { //nolint:gocritic // contracts.Store requires a value parameter.
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	const q = `
INSERT INTO profiles (name, config_dir, label, color, created_at, last_used_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
    config_dir   = excluded.config_dir,
    label        = excluded.label,
    color        = excluded.color,
    created_at   = excluded.created_at,
    last_used_at = excluded.last_used_at
`
	_, err := s.db.ExecContext(
		ctx, q,
		p.Name,
		p.ConfigDir,
		p.Label,
		p.Color,
		p.CreatedAt.UnixNano(),
		p.LastUsedAt.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("saving profile %q: %w", p.Name, err)
	}
	return nil
}

// GetProfile returns the profile with the given name. Returns
// contracts.ErrProfileNotFound (wrapped) if no row exists.
func (s *Store) GetProfile(ctx context.Context, name string) (contracts.Profile, error) {
	const q = `
SELECT name, config_dir, label, color, created_at, last_used_at
FROM profiles
WHERE name = ?
`
	var (
		p                 contracts.Profile
		label, color      sql.NullString
		createdNs, usedNs int64
	)
	err := s.db.QueryRowContext(ctx, q, name).Scan(
		&p.Name, &p.ConfigDir, &label, &color, &createdNs, &usedNs,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.Profile{}, fmt.Errorf("looking up profile %q: %w", name, contracts.ErrProfileNotFound)
	}
	if err != nil {
		return contracts.Profile{}, fmt.Errorf("looking up profile %q: %w", name, err)
	}
	p.Label = label.String
	p.Color = color.String
	p.CreatedAt = time.Unix(0, createdNs).UTC()
	p.LastUsedAt = time.Unix(0, usedNs).UTC()
	return p, nil
}
