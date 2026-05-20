package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// QueryUsage returns one UsageRow per (profile_name, project, model, day)
// group that overlaps the query's TimeRange. An empty Profile/Project field
// in the query is treated as "any". EstimatedUSD is left at zero; pricing is
// the caller's responsibility (see internal/pricing).
func (s *Store) QueryUsage(ctx context.Context, q contracts.UsageQuery) ([]contracts.UsageRow, error) { //nolint:gocritic // contracts.Store requires a value parameter.
	var (
		where []string
		args  []any
	)

	if q.Profile != "" {
		where = append(where, "profile_name = ?")
		args = append(args, q.Profile)
	}
	if q.Project != "" {
		where = append(where, "project = ?")
		args = append(args, q.Project)
	}
	if !q.Range.Start.IsZero() {
		where = append(where, "ts >= ?")
		args = append(args, q.Range.Start.UnixNano())
	}
	if !q.Range.End.IsZero() {
		where = append(where, "ts <= ?")
		args = append(args, q.Range.End.UnixNano())
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	// Group by (profile, project, model, day). Day is computed by truncating
	// the ts (ns) to the start of UTC day. 86400 seconds x 1e9 ns/s.
	query := `
SELECT
    profile_name,
    COALESCE(project, '')                    AS project,
    COALESCE(model, '')                      AS model,
    (ts / 86400000000000) * 86400000000000   AS day_ns,
    SUM(input_tokens)                        AS in_tokens,
    SUM(output_tokens)                       AS out_tokens,
    SUM(cache_read_tokens)                   AS cr_tokens,
    SUM(cache_create_tokens)                 AS cc_tokens,
    COUNT(DISTINCT session_id)               AS sessions
FROM events
` + whereSQL + `
GROUP BY profile_name, project, model, day_ns
ORDER BY profile_name ASC, day_ns ASC, project ASC, model ASC
`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying usage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []contracts.UsageRow
	for rows.Next() {
		var (
			r       contracts.UsageRow
			project sql.NullString
			model   sql.NullString
			dayNs   int64
		)
		if err := rows.Scan(
			&r.Profile,
			&project,
			&model,
			&dayNs,
			&r.Usage.InputTokens,
			&r.Usage.OutputTokens,
			&r.Usage.CacheReadTokens,
			&r.Usage.CacheCreateTokens,
			&r.SessionCount,
		); err != nil {
			return nil, fmt.Errorf("scanning usage row: %w", err)
		}
		r.Project = project.String
		r.Model = model.String
		r.Day = time.Unix(0, dayNs).UTC()
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating usage rows: %w", err)
	}
	return out, nil
}
