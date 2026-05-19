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

// ListProfiles returns every profile, sorted ascending by name.
func (s *Store) ListProfiles(ctx context.Context) ([]contracts.Profile, error) {
	const q = `
SELECT name, config_dir, label, color, created_at, last_used_at
FROM profiles
ORDER BY name ASC
`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listing profiles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []contracts.Profile
	for rows.Next() {
		var (
			p                 contracts.Profile
			label, color      sql.NullString
			createdNs, usedNs int64
		)
		if err := rows.Scan(&p.Name, &p.ConfigDir, &label, &color, &createdNs, &usedNs); err != nil {
			return nil, fmt.Errorf("scanning profile row: %w", err)
		}
		p.Label = label.String
		p.Color = color.String
		p.CreatedAt = time.Unix(0, createdNs).UTC()
		p.LastUsedAt = time.Unix(0, usedNs).UTC()
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating profile rows: %w", err)
	}
	return out, nil
}

// DeleteProfile removes the named profile. The FOREIGN KEY ... ON DELETE
// CASCADE on events and scan_cursors removes the associated rows automatically.
// Returns contracts.ErrProfileNotFound (wrapped) if no row matched.
func (s *Store) DeleteProfile(ctx context.Context, name string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	res, err := s.db.ExecContext(ctx, `DELETE FROM profiles WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("deleting profile %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected for delete %q: %w", name, err)
	}
	if n == 0 {
		return fmt.Errorf("deleting profile %q: %w", name, contracts.ErrProfileNotFound)
	}
	return nil
}
