# B3b Task 5 Step 0: Shared Scanner Refactor Micro-Spec

Date: 2026-05-25; status: draft for human review before scanner code

## Context

B3b makes every managed profile's `<CLAUDE_CONFIG_DIR>/projects/` point at
one ccx-owned directory: `<CCX_HOME>/shared-projects/`.

This is required so `ccx run --supervise` can stop Claude Code between turns,
switch `CLAUDE_CONFIG_DIR`, and relaunch `claude --resume <session-id>` without
orphaning the conversation JSONL.

The existing scanner API walks one profile at a time:

```go
Scan(ctx context.Context, profile contracts.Profile) (<-chan contracts.Event, <-chan error)
```

That behavior must stay for unmigrated profiles. A shared symlink layout needs
a new one-walk path so the same JSONL files are not scanned once per profile.

## Decision 1: New Scanner API

Add these types to `internal/scanner`, not `internal/contracts`:

```go
type AttributedEvent struct {
    Event   contracts.Event
    Profile string
}

type SessionLookup interface {
    ProfileForSession(ctx context.Context, sessionID string) (profile string, ok bool, err error)
}
```

Add this method beside the existing `Scan`:

```go
func (s *Scanner) ScanShared(
    ctx context.Context,
    projectsRoot string,
    lookup SessionLookup,
) (<-chan AttributedEvent, <-chan error)
```

`projectsRoot` is the real shared projects directory, normally
`quotamigrate.SharedProjectsPath(ccxHome)`.

`ScanShared` walks:

```text
<projectsRoot>/<encoded-project>/<session>.jsonl
```

It parses with the same JSONL parser and inode-aware cursor behavior as `Scan`.
The only semantic difference is attribution: each parsed event's
`SessionID` is resolved through `SessionLookup`.

`contracts.Event` remains unchanged because `internal/contracts/` is frozen.

## Decision 2: SessionLookup Storage Backing

Implement the storage lookup outside frozen contracts:

```go
func (s *Store) ProfileForSession(
    ctx context.Context,
    sessionID string,
) (string, bool, error)
```

Query:

```sql
SELECT profile_name
FROM sessions
WHERE session_id = ?
LIMIT 1
```

`sql.ErrNoRows` maps to `("", false, nil)`. Other errors are wrapped.

The `sessions` table is the right source because hook telemetry records
`SessionStart` with the profile that launched the Claude Code session.

## Decision 3: Cursor Key Strategy

Use the existing `CursorStore` interface and the existing
`scan_cursors(profile_name, file_path)` schema.

For shared scans, use this scanner package constant:

```go
const SharedCursorProfile = "__shared__"
```

Profile names are validated as `^[a-z0-9-]+$`, so this sentinel cannot collide
with a user profile name.

Rationale:

- No frozen contract change.
- No `internal/storage/schema.sql` change.
- No migration needed for cursors.
- Existing per-profile cursors stay valid for legacy scans.

When a profile is migrated to shared history, the first shared scan will use
fresh sentinel cursors and may re-read files once. `InsertEvents` already uses
`ON CONFLICT(profile_name, event_uuid) DO NOTHING`, so duplicate rows for the
same owner are harmless.

## Decision 4: Shared vs Legacy Runtime Detection

The ingest layer should not make `Scan` decide this. The scanner should expose
both APIs; callers choose.

Runtime detection lives in the integration layers that already know `CCX_HOME`
and the profile list:

- `internal/cli/usage.go`
- `internal/cli/suggest.go`
- `internal/daemon/ingest.go`

Detection algorithm:

1. Compute `sharedRoot := quotamigrate.SharedProjectsPath(ccxHome)`.
2. `sharedRoot` must exist and be a directory.
3. For each profile, inspect `<profile.ConfigDir>/projects` with `os.Lstat`.
4. A profile is in the shared partition when:
   - `projects` is a symlink, and
   - `filepath.EvalSymlinks(projects)` equals
     `filepath.EvalSymlinks(sharedRoot)`.
5. Profiles not matching that rule remain in the legacy partition.

Ingest behavior:

- If shared partition is non-empty, call `ScanShared` once for `sharedRoot`.
- Also run legacy `Scan` for every profile in the legacy partition.
- If shared partition is empty, keep the old per-profile loop.

This handles mixed states during migration: some profiles may already be
symlinked while others still have private `projects/` directories.

Unexpected symlinks are not rewritten by ingest. They remain outside the shared
partition and get a debug log; follow-up repair is the job of
`ccx migrate-shared-history`.

## Decision 5: Attribution Miss Policy

If `SessionLookup.ProfileForSession` returns `ok=false`, `ScanShared` skips the
event and logs at debug level:

```text
scanner: shared event skipped; session owner unknown
```

Include `session_id` and `path` fields. Do not emit an error, and do not advance that file's shared cursor until every event in it has an owner.

Rationale:

- Missing attribution usually means hooks were not installed before the JSONL
  was written.
- Failing the whole scan would make one old session block all current usage.
- Cursor retry is enough; durable buffering is out of scope for v0.2.

If lookup returns an actual error, report it on the `errs` channel and stop the
scan. Storage errors are different from "unknown session".

## Decision 6: Events Table Migration Story

Do not rewrite existing `events` rows.

Existing rows already carry `profile_name`; they remain the historical truth
for pre-B3b scans.

After shared scanning lands:

- Correctly attributed rows insert under the owning profile.
- Duplicate `(profile_name, event_uuid)` rows are ignored by existing storage.
- Rows inserted by older transitional builds under the wrong profile are not
  auto-corrected in v0.2; repairing that would require a separate data
  migration and conflict policy.

This keeps B3b focused on forward correctness and avoids touching frozen schema.

## Implementation Notes For The Next Step

1. Add scanner tests first:
   - shared root with two sessions maps to two different profiles;
   - unknown session is skipped without scan error;
   - sentinel cursor prevents a second shared scan from re-emitting events.
2. Reuse as much of `listJSONL`, `processFile`, and `readFile` as possible.
3. Add storage tests for `ProfileForSession`.
4. Wire shared detection in CLI and daemon ingest after scanner/storage tests
   are green.
5. Keep `Scan` behavior unchanged for legacy profile directories.

No scanner code should be written until this note is reviewed.
