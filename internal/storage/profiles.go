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
INSERT INTO profiles (
    name, config_dir, label, color, created_at, last_used_at,
    daily_token_budget, weekly_token_budget, monthly_usd_budget, priority,
    suggest_enabled, rate_limit_cooldown
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
    config_dir          = excluded.config_dir,
    label               = excluded.label,
    color               = excluded.color,
    created_at          = excluded.created_at,
    last_used_at        = excluded.last_used_at,
    daily_token_budget  = excluded.daily_token_budget,
    weekly_token_budget = excluded.weekly_token_budget,
    monthly_usd_budget  = excluded.monthly_usd_budget,
    priority            = excluded.priority,
    suggest_enabled     = excluded.suggest_enabled,
    rate_limit_cooldown = excluded.rate_limit_cooldown
`
	_, err := s.db.ExecContext(
		ctx, q,
		p.Name,
		p.ConfigDir,
		p.Label,
		p.Color,
		p.CreatedAt.UnixNano(),
		p.LastUsedAt.UnixNano(),
		nullableInt(p.Limits.DailyTokenBudget),
		nullableInt(p.Limits.WeeklyTokenBudget),
		nullableFloat(p.Limits.MonthlyUSDBudget),
		nullableInt(p.Limits.Priority),
		nullableBoolPtr(p.Limits.SuggestEnabled),
		nullableString(p.Limits.RateLimitCooldown),
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
SELECT
    name,
    config_dir,
    label,
    color,
    created_at,
    last_used_at,
    daily_token_budget,
    weekly_token_budget,
    monthly_usd_budget,
    priority,
    suggest_enabled,
    rate_limit_cooldown
FROM profiles
WHERE name = ?
`
	var (
		p                  contracts.Profile
		label, color       sql.NullString
		createdNs, usedNs  int64
		daily, weekly, pri sql.NullInt64
		monthly            sql.NullFloat64
		suggest            sql.NullInt64
		rateLimitCooldown  sql.NullString
	)
	err := s.db.QueryRowContext(ctx, q, name).Scan(
		&p.Name,
		&p.ConfigDir,
		&label,
		&color,
		&createdNs,
		&usedNs,
		&daily,
		&weekly,
		&monthly,
		&pri,
		&suggest,
		&rateLimitCooldown,
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
	applyProfileLimits(&p, daily, weekly, monthly, pri, suggest, rateLimitCooldown)
	return p, nil
}

// ListProfiles returns every profile, sorted ascending by name.
func (s *Store) ListProfiles(ctx context.Context) ([]contracts.Profile, error) {
	const q = `
SELECT
    name,
    config_dir,
    label,
    color,
    created_at,
    last_used_at,
    daily_token_budget,
    weekly_token_budget,
    monthly_usd_budget,
    priority,
    suggest_enabled,
    rate_limit_cooldown
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
			p                  contracts.Profile
			label, color       sql.NullString
			createdNs, usedNs  int64
			daily, weekly, pri sql.NullInt64
			monthly            sql.NullFloat64
			suggest            sql.NullInt64
			rateLimitCooldown  sql.NullString
		)
		if err := rows.Scan(
			&p.Name,
			&p.ConfigDir,
			&label,
			&color,
			&createdNs,
			&usedNs,
			&daily,
			&weekly,
			&monthly,
			&pri,
			&suggest,
			&rateLimitCooldown,
		); err != nil {
			return nil, fmt.Errorf("scanning profile row: %w", err)
		}
		p.Label = label.String
		p.Color = color.String
		p.CreatedAt = time.Unix(0, createdNs).UTC()
		p.LastUsedAt = time.Unix(0, usedNs).UTC()
		applyProfileLimits(&p, daily, weekly, monthly, pri, suggest, rateLimitCooldown)
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

func applyProfileLimits(
	p *contracts.Profile,
	daily sql.NullInt64,
	weekly sql.NullInt64,
	monthly sql.NullFloat64,
	priority sql.NullInt64,
	suggest sql.NullInt64,
	rateLimitCooldown sql.NullString,
) {
	if daily.Valid {
		p.Limits.DailyTokenBudget = int(daily.Int64)
	}
	if weekly.Valid {
		p.Limits.WeeklyTokenBudget = int(weekly.Int64)
	}
	if monthly.Valid {
		p.Limits.MonthlyUSDBudget = monthly.Float64
	}
	if priority.Valid {
		p.Limits.Priority = int(priority.Int64)
	}
	if suggest.Valid {
		enabled := suggest.Int64 != 0
		p.Limits.SuggestEnabled = &enabled
	}
	if rateLimitCooldown.Valid {
		p.Limits.RateLimitCooldown = rateLimitCooldown.String
	}
}

func nullableInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableFloat(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullableBoolPtr(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return 1
	}
	return 0
}
