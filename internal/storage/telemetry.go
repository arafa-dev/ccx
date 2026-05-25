package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// InsertHookEvent stores one hook event. The profileName argument is
// authoritative because hook payloads may be forwarded without a profile
// field; event.Profile is used only as a fallback.
func (s *Store) InsertHookEvent(ctx context.Context, profileName string, event contracts.HookEvent) error { //nolint:gocritic // contracts.Store requires a value parameter.
	profileName, err := normalizeHookTelemetry(profileName, &event)
	if err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := insertHookEvent(ctx, s.db, profileName, &event); err != nil {
		return err
	}
	return nil
}

// RecordHookTelemetry stores the raw hook event and updates the session
// aggregate in a single database transaction.
func (s *Store) RecordHookTelemetry(ctx context.Context, profileName string, event contracts.HookEvent) (retErr error) { //nolint:gocritic // contracts.Store requires a value parameter.
	profileName, err := normalizeHookTelemetry(profileName, &event)
	if err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin hook telemetry record for %q/%q: %w", profileName, event.Session, err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback()
		}
	}()

	if err := insertHookEvent(ctx, tx, profileName, &event); err != nil {
		return err
	}
	if err := upsertSessionTelemetry(ctx, tx, profileName, &event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit hook telemetry record for %q/%q: %w", profileName, event.Session, err)
	}
	return nil
}

type hookEventExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertHookEvent(ctx context.Context, execer hookEventExecer, profileName string, event *contracts.HookEvent) error {
	const q = `
INSERT INTO hook_events (
    profile_name, session_id, event_name, ts, transcript_path, cwd, model,
    source, permission_mode, reason, error, error_details, trigger
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	if _, err := execer.ExecContext(
		ctx, q,
		profileName,
		event.Session,
		event.Event,
		unixNano(event.Timestamp),
		event.Transcript,
		event.CWD,
		event.Model,
		event.Source,
		event.Permission,
		event.Reason,
		event.Error,
		event.ErrorDetails,
		event.Trigger,
	); err != nil {
		return fmt.Errorf("inserting hook event %q for %q: %w", event.Event, profileName, err)
	}
	return nil
}

// UpsertSessionTelemetry folds one hook event into the session aggregate row.
func (s *Store) UpsertSessionTelemetry(ctx context.Context, profileName string, event contracts.HookEvent) (retErr error) { //nolint:gocritic // contracts.Store requires a value parameter.
	profileName, err := normalizeHookTelemetry(profileName, &event)
	if err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session upsert for %q/%q: %w", profileName, event.Session, err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback()
		}
	}()

	if err := upsertSessionTelemetry(ctx, tx, profileName, &event); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session upsert for %q/%q: %w", profileName, event.Session, err)
	}
	return nil
}

func normalizeHookTelemetry(profileName string, event *contracts.HookEvent) (string, error) {
	if profileName == "" {
		profileName = event.Profile
	}
	if profileName == "" {
		return "", errors.New("hook telemetry profile is empty")
	}
	if event.Session == "" {
		return "", errors.New("hook telemetry session is empty")
	}
	if event.Event == "" {
		return "", errors.New("hook telemetry event is empty")
	}
	return profileName, nil
}

func upsertSessionTelemetry(ctx context.Context, tx *sql.Tx, profileName string, event *contracts.HookEvent) error {
	rec, found, err := loadSessionRecord(ctx, tx, profileName, event.Session)
	if err != nil {
		return err
	}
	if !found {
		rec = sessionRecord{
			ProfileName: profileName,
			SessionID:   event.Session,
			Status:      "unknown",
		}
	}

	ts := unixNano(event.Timestamp)
	isNewer := !found || ts > rec.LastSeenAt
	if isNewer {
		rec.LastSeenAt = ts
	}

	mergeEventMetadata(&rec, event, isNewer)
	switch event.Event {
	case "SessionStart":
		if !rec.StartedAt.Valid || (ts != 0 && ts < rec.StartedAt.Int64) {
			rec.StartedAt = sql.NullInt64{Int64: ts, Valid: true}
		}
		applySessionStatus(&rec, "running")
	case "Stop":
		if applySessionStatus(&rec, "completed") {
			rec.FailureError = ""
			rec.FailureDetails = ""
		}
	case "StopFailure":
		applySessionStatus(&rec, "failed")
		if shouldReplaceFailureFacts(&rec, event, ts) {
			rec.FailureError = event.Error
			rec.FailureDetails = event.ErrorDetails
			rec.FailureAt = sql.NullInt64{Int64: ts, Valid: true}
		}
	case "SessionEnd":
		if !rec.EndedAt.Valid || ts > rec.EndedAt.Int64 {
			rec.EndedAt = sql.NullInt64{Int64: ts, Valid: true}
			rec.EndReason = event.Reason
		} else if rec.EndReason == "" {
			rec.EndReason = event.Reason
		}
		applySessionStatus(&rec, "ended")
	case "PreCompact":
	case "PostCompact":
		// The session aggregate has no hook event id, so compaction is counted
		// once per observed PostCompact payload, including replays.
		rec.CompactCount++
	default:
		if rec.Status == "" {
			rec.Status = "unknown"
		}
	}

	if err := saveSessionRecord(ctx, tx, &rec); err != nil {
		return err
	}
	return nil
}

// QuerySessions returns session aggregates ordered by most recent activity.
func (s *Store) QuerySessions(ctx context.Context, q contracts.SessionQuery) ([]contracts.SessionTelemetry, error) { //nolint:gocritic // contracts.Store requires a value parameter.
	var (
		where []string
		args  []any
	)
	if q.Profile != "" {
		where = append(where, "profile_name = ?")
		args = append(args, q.Profile)
	}
	if q.Status != "" {
		where = append(where, "status = ?")
		args = append(args, q.Status)
	}
	if !q.Since.IsZero() {
		where = append(where, "last_seen_at >= ?")
		args = append(args, unixNano(q.Since))
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	query := `
SELECT
    profile_name,
    session_id,
    COALESCE(transcript_path, ''),
    COALESCE(cwd, ''),
    COALESCE(model, ''),
    COALESCE(source, ''),
    COALESCE(permission_mode, ''),
    started_at,
    ended_at,
    last_seen_at,
    status,
    COALESCE(end_reason, ''),
    COALESCE(failure_error, ''),
    COALESCE(failure_details, ''),
    compact_count
FROM sessions
` + whereSQL + `
ORDER BY last_seen_at DESC
`
	if q.Limit > 0 {
		query += "LIMIT ?\n"
		args = append(args, q.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]contracts.SessionTelemetry, 0)
	for rows.Next() {
		session, err := scanSessionTelemetry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating session rows: %w", err)
	}
	return out, nil
}

// ProfileForSession returns the ccx profile name that owns the given Claude
// Code session ID, as recorded in session telemetry.
func (s *Store) ProfileForSession(ctx context.Context, sessionID string) (profile string, ok bool, err error) {
	const q = `
SELECT profile_name
FROM sessions
WHERE session_id = ?
	LIMIT 1
`
	var profileName string
	err = s.db.QueryRowContext(ctx, q, sessionID).Scan(&profileName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("profile for session %q: %w", sessionID, err)
	}
	return profileName, true, nil
}

// QueryRecentFailures returns StopFailure hook events ordered newest first.
func (s *Store) QueryRecentFailures(ctx context.Context, profileName string, since time.Time) ([]contracts.HookEvent, error) {
	const q = `
SELECT
    profile_name,
    session_id,
    event_name,
    ts,
    COALESCE(transcript_path, ''),
    COALESCE(cwd, ''),
    COALESCE(model, ''),
    COALESCE(source, ''),
    COALESCE(permission_mode, ''),
    COALESCE(reason, ''),
    COALESCE(error, ''),
    COALESCE(error_details, ''),
    COALESCE(trigger, '')
FROM hook_events
WHERE profile_name = ? AND event_name = 'StopFailure' AND ts >= ?
ORDER BY ts DESC
`
	rows, err := s.db.QueryContext(ctx, q, profileName, unixNano(since))
	if err != nil {
		return nil, fmt.Errorf("querying recent failures for %q: %w", profileName, err)
	}
	defer func() { _ = rows.Close() }()

	var out []contracts.HookEvent
	for rows.Next() {
		var (
			ev contracts.HookEvent
			ns int64
		)
		if err := rows.Scan(
			&ev.Profile,
			&ev.Session,
			&ev.Event,
			&ns,
			&ev.Transcript,
			&ev.CWD,
			&ev.Model,
			&ev.Source,
			&ev.Permission,
			&ev.Reason,
			&ev.Error,
			&ev.ErrorDetails,
			&ev.Trigger,
		); err != nil {
			return nil, fmt.Errorf("scanning recent failure row: %w", err)
		}
		ev.Timestamp = time.Unix(0, ns).UTC()
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating recent failure rows: %w", err)
	}
	return out, nil
}

// SaveProfileHealth upserts the latest health check for a profile.
func (s *Store) SaveProfileHealth(ctx context.Context, health contracts.ProfileHealth) error { //nolint:gocritic // contracts.Store requires a value parameter.
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	const q = `
INSERT INTO profile_health (profile_name, checked_at, auth_status, auth_detail)
VALUES (?, ?, ?, ?)
ON CONFLICT(profile_name) DO UPDATE SET
    checked_at  = excluded.checked_at,
    auth_status = excluded.auth_status,
    auth_detail = excluded.auth_detail
`
	if _, err := s.db.ExecContext(ctx, q, health.Profile, unixNano(health.CheckedAt), health.AuthStatus, health.AuthDetail); err != nil {
		return fmt.Errorf("saving profile health %q: %w", health.Profile, err)
	}
	return nil
}

// GetProfileHealth returns the latest health check for a profile.
func (s *Store) GetProfileHealth(ctx context.Context, profileName string) (contracts.ProfileHealth, error) {
	const q = `
SELECT profile_name, checked_at, auth_status, COALESCE(auth_detail, '')
FROM profile_health
WHERE profile_name = ?
`
	var (
		health contracts.ProfileHealth
		ns     int64
	)
	err := s.db.QueryRowContext(ctx, q, profileName).Scan(&health.Profile, &ns, &health.AuthStatus, &health.AuthDetail)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.ProfileHealth{}, fmt.Errorf("profile health %q: %w", profileName, contracts.ErrProfileNotFound)
	}
	if err != nil {
		return contracts.ProfileHealth{}, fmt.Errorf("getting profile health %q: %w", profileName, err)
	}
	health.CheckedAt = time.Unix(0, ns).UTC()
	return health, nil
}

type sessionRecord struct {
	ProfileName    string
	SessionID      string
	TranscriptPath string
	CWD            string
	Model          string
	Source         string
	PermissionMode string
	StartedAt      sql.NullInt64
	EndedAt        sql.NullInt64
	LastSeenAt     int64
	Status         string
	EndReason      string
	FailureAt      sql.NullInt64
	FailureError   string
	FailureDetails string
	CompactCount   int
}

func loadSessionRecord(ctx context.Context, tx *sql.Tx, profileName, sessionID string) (sessionRecord, bool, error) {
	const q = `
SELECT
    COALESCE(transcript_path, ''),
    COALESCE(cwd, ''),
    COALESCE(model, ''),
    COALESCE(source, ''),
    COALESCE(permission_mode, ''),
    started_at,
    ended_at,
    last_seen_at,
    status,
    COALESCE(end_reason, ''),
    failure_at,
    COALESCE(failure_error, ''),
    COALESCE(failure_details, ''),
    compact_count
FROM sessions
WHERE profile_name = ? AND session_id = ?
`
	rec := sessionRecord{ProfileName: profileName, SessionID: sessionID}
	err := tx.QueryRowContext(ctx, q, profileName, sessionID).Scan(
		&rec.TranscriptPath,
		&rec.CWD,
		&rec.Model,
		&rec.Source,
		&rec.PermissionMode,
		&rec.StartedAt,
		&rec.EndedAt,
		&rec.LastSeenAt,
		&rec.Status,
		&rec.EndReason,
		&rec.FailureAt,
		&rec.FailureError,
		&rec.FailureDetails,
		&rec.CompactCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return sessionRecord{}, false, nil
	}
	if err != nil {
		return sessionRecord{}, false, fmt.Errorf("loading session %q/%q: %w", profileName, sessionID, err)
	}
	return rec, true, nil
}

func saveSessionRecord(ctx context.Context, tx *sql.Tx, rec *sessionRecord) error {
	const q = `
INSERT INTO sessions (
    profile_name, session_id, transcript_path, cwd, model, source,
    permission_mode, started_at, ended_at, last_seen_at, status, end_reason,
    failure_at, failure_error, failure_details, compact_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(profile_name, session_id) DO UPDATE SET
    transcript_path = excluded.transcript_path,
    cwd             = excluded.cwd,
    model           = excluded.model,
    source          = excluded.source,
    permission_mode = excluded.permission_mode,
    started_at      = excluded.started_at,
    ended_at        = excluded.ended_at,
    last_seen_at    = excluded.last_seen_at,
    status          = excluded.status,
    end_reason      = excluded.end_reason,
    failure_at      = excluded.failure_at,
    failure_error   = excluded.failure_error,
    failure_details = excluded.failure_details,
    compact_count   = excluded.compact_count
`
	if _, err := tx.ExecContext(
		ctx, q,
		rec.ProfileName,
		rec.SessionID,
		rec.TranscriptPath,
		rec.CWD,
		rec.Model,
		rec.Source,
		rec.PermissionMode,
		nullableInt64(rec.StartedAt),
		nullableInt64(rec.EndedAt),
		rec.LastSeenAt,
		rec.Status,
		rec.EndReason,
		nullableInt64(rec.FailureAt),
		rec.FailureError,
		rec.FailureDetails,
		rec.CompactCount,
	); err != nil {
		return fmt.Errorf("saving session %q/%q: %w", rec.ProfileName, rec.SessionID, err)
	}
	return nil
}

func mergeEventMetadata(rec *sessionRecord, event *contracts.HookEvent, overwrite bool) {
	if event.Transcript != "" && (overwrite || rec.TranscriptPath == "") {
		rec.TranscriptPath = event.Transcript
	}
	if event.CWD != "" && (overwrite || rec.CWD == "") {
		rec.CWD = event.CWD
	}
	if event.Model != "" && (overwrite || rec.Model == "") {
		rec.Model = event.Model
	}
	if event.Source != "" && (overwrite || rec.Source == "") {
		rec.Source = event.Source
	}
	if event.Permission != "" && (overwrite || rec.PermissionMode == "") {
		rec.PermissionMode = event.Permission
	}
}

func applySessionStatus(rec *sessionRecord, next string) bool {
	if sessionStatusRank(next) < sessionStatusRank(rec.Status) {
		return false
	}
	rec.Status = next
	return true
}

func sessionStatusRank(status string) int {
	switch status {
	case "failed":
		return 5
	case "ended":
		return 4
	case "completed":
		return 3
	case "running":
		return 2
	case "unknown", "":
		return 1
	default:
		return 0
	}
}

func shouldReplaceFailureFacts(rec *sessionRecord, event *contracts.HookEvent, ts int64) bool {
	if !rec.FailureAt.Valid {
		return true
	}
	if ts > rec.FailureAt.Int64 {
		return true
	}
	if ts < rec.FailureAt.Int64 {
		return false
	}
	// Equal-timestamp StopFailure events use lexicographic tuple max so the
	// same set of hook payloads produces the same aggregate in any order.
	if event.Error != rec.FailureError {
		return event.Error > rec.FailureError
	}
	return event.ErrorDetails > rec.FailureDetails
}

func scanSessionTelemetry(rows *sql.Rows) (contracts.SessionTelemetry, error) {
	var (
		session          contracts.SessionTelemetry
		started, ended   sql.NullInt64
		lastSeenUnixNano int64
	)
	if err := rows.Scan(
		&session.Profile,
		&session.Session,
		&session.Transcript,
		&session.CWD,
		&session.Model,
		&session.Source,
		&session.Permission,
		&started,
		&ended,
		&lastSeenUnixNano,
		&session.Status,
		&session.EndReason,
		&session.FailureError,
		&session.FailureDetails,
		&session.CompactCount,
	); err != nil {
		return contracts.SessionTelemetry{}, fmt.Errorf("scanning session row: %w", err)
	}
	if started.Valid {
		session.StartedAt = time.Unix(0, started.Int64).UTC()
	}
	if ended.Valid {
		session.EndedAt = time.Unix(0, ended.Int64).UTC()
	}
	session.LastSeenAt = time.Unix(0, lastSeenUnixNano).UTC()
	return session, nil
}

func nullableInt64(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func unixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}
