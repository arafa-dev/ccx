<!-- markdownlint-disable MD013 MD040 MD049 -->
<!-- cspell:ignore predev rescans upserted upserts -->

# AGENTS.md

Operational guide for AI coding agents (Codex, Claude, Cursor, etc.) working on
**ccx** — _Claude Code eXtended_. Read this file before writing code.

> **TL;DR for the impatient agent**
>
> - Stack: Go 1.22 CLI + optional background daemon + embedded Next.js 15 dashboard. Single binary.
> - Touch only your assigned package. `internal/contracts/` and `internal/storage/schema.sql` are shared/frozen.
> - Workflow per task: failing test → run-fail → implement → run-pass → one commit (conventional commits).
> - Before pushing: `make ci` (= `golangci-lint run` + `go test -race -count=1 ./...`).
> - Dashboard changes require staging: `cd web && pnpm build && cd .. && make stage-web && go build ./...`.
> - Bind HTTP only to `127.0.0.1`. Never log credential paths. No proxying. No CGo. No telemetry phone-home.

---

## 1. What ccx is (and isn't)

ccx is a Go CLI + optional local daemon that:

1. Manages a registry of Claude Code "profiles" (each = one `CLAUDE_CONFIG_DIR`).
2. Switches the active profile by **printing shell `export` lines** the user `eval`s.
3. Parses per-profile JSONL session files into SQLite and surfaces usage via CLI + an embedded local dashboard.
4. Optionally runs a background daemon (`ccx daemon`) that watches profile dirs, ingests live, and serves the dashboard.
5. Optionally installs **Claude Code hooks** in a profile's `settings.json` so session lifecycle events flow into ccx's local SQLite (hook telemetry).
6. Offers **advisory** profile-headroom suggestions (`ccx suggest`) ranked from budgets, usage, recent failures, and auth health.

**ccx never proxies API calls** and **does not phone home**. "Telemetry" here means *local* per-session lifecycle data captured from Claude Code hooks into the local SQLite — not network analytics. No outbound traffic from the binary at runtime (except `claude` itself, which ccx doesn't initiate). The headroom path is advisory: it prints a `ccx use <name>` command; it does not switch accounts, move credentials, or resume sessions.

Architecture overview: [`docs/architecture.md`](docs/architecture.md).

---

## 2. Repo layout (memorize this)

```
cmd/ccx/                   main.go — thin entry point, calls internal/cli.Execute
internal/
  contracts/               SHARED. Profile, Event, Usage, HookEvent, SessionTelemetry, ProfileHealth,
                           DaemonStatus, HeadroomRecommendation, ProfileLimits + Scanner/Store/PricingTable
                           interfaces. FROZEN — amendments via separate PR against main.
  storage/                 SQLite-backed Store impl. v1 schema in schema.sql (frozen); v2 migration adds
                           hook_events / sessions / profile_health / profile-limit columns (see migrate.go).
  scanner/                 JSONL streaming parser with incremental cursors
  profile/                 ~/.ccx/profiles.toml registry + ProfileLimits (budgets, priority, cooldown)
  pricing/                 Embedded models.yaml + user override at ~/.ccx/pricing.yaml
  shell/                   zsh/bash/fish/pwsh snippet generators for `ccx use` and `ccx init`
  platform/                OS detection, default config dir resolution, ProcessIdentity (for daemon locks)
  daemon/                  Background daemon runtime: lock file, pid/status/log, fsnotify watcher,
                           coalesced rescans, foreground + detached modes. Serves the dashboard API.
  hooks/                   Claude Code hook installer (writes profile settings.json with backups) and
                           the `ccx hooks record` payload recorder that drives session telemetry.
  headroom/                Evaluator that ranks profiles for `ccx suggest` and /api/headroom.
  cli/                     cobra command tree (wires Phase 1 packages)
  server/                  chi-routed HTTP API for the dashboard (always 127.0.0.1)
  tui/                     bubbletea profile picker
  doctor/                  Diagnostic checks for `ccx doctor`
  dashboard/               go:embed wrapper for web/out → web-out/
web/                       Next.js 15 static-export dashboard (pnpm workspace)
  app/                     App-router pages
  components/              UI — includes recommendation-panel, recent-sessions, profile-cards, etc.
  lib/api-types.ts         Generated from api/openapi.yaml — regenerate via `pnpm gen:api`
  mocks/                   MSW handlers for `pnpm dev`
api/openapi.yaml           HTTP API contract. FROZEN. Now includes /api/daemon/status, /api/hooks/status,
                           /api/sessions, /api/headroom alongside /api/{health,profiles,usage,usage/live}.
docs/
  architecture.md          Component diagram, usage flow, and daemon/hooks/headroom flow
  conventions.md           SOURCE OF TRUTH for engineering rules (read this)
  shell-integration.md     How `ccx use` actually works
  troubleshooting.md       User-facing FAQ (covers daemon, hooks, suggest)
  superpowers/             Spec + 13 phase plans (historical reference)
HANDOFF.md                 Phase status snapshot — somewhat stale; trust git log + this file for current state
pricing/models.yaml        Embedded pricing table baseline
integration_test/          Build-tagged end-to-end tests (`go test -tags integration`)
.golangci.yml              Lint config; CI gates on it
lefthook.yml               pre-commit: gofumpt + golangci-lint
```

Files an agent **must never edit from a feature worktree** (open a separate
contract-amendment PR against `main` instead):

- `internal/contracts/` — every package depends on it.
- `internal/storage/schema.sql` — v1 source of truth; new schema goes through a versioned migration in `internal/storage/migrate.go` after the contract is amended.
- `api/openapi.yaml` — both Go server and TS client follow it.
- `docs/conventions.md` — cross-package rules.

---

## 3. CLI command surface (post-PR #16)

Top-level groups under `ccx`:

| Group | Subcommands / purpose |
| --- | --- |
| `profile` | `add`, `list`, `rm`, `current`, `set` (limits/cooldown/suggest toggle) |
| `use [name]` | Print shell exports to activate a profile (interactive picker if `[name]` omitted) |
| `init <shell>` | Emit shell rc snippet for `zsh\|bash\|fish\|pwsh` |
| `usage` | Per-profile / per-project usage rollup (`--json` for machine output) |
| `suggest` | Advisory profile recommendation (`--json`, `--include-unavailable`) |
| `dashboard` | Start the embedded dashboard server (`--port`, `--no-open`, `--daemon` to reuse the running daemon) |
| `daemon` | `start --foreground` / `start` / `status` / `stop` / `restart` / `logs` |
| `hooks` | `install`, `status`, `uninstall`, `record` (the last is invoked by Claude Code, not humans) |
| `doctor` | Diagnostic checks |
| `version` | Build info |

Runtime files (under `~/.ccx/`):

- `profiles.toml` — TOML registry (human-editable).
- `state.db` — SQLite (events, hook_events, sessions, profile_health, scan_cursors).
- `pricing.yaml` — optional user override.
- `daemon.pid`, `daemon.json`, `daemon.log`, `daemon.lock` — daemon runtime files (only when the daemon is running).

---

## 4. HTTP API (served by `ccx dashboard` or `ccx daemon`)

Always bound to `127.0.0.1`. No auth.

| Route | Purpose |
| --- | --- |
| `GET /api/health` | `{ok, version}` |
| `GET /api/profiles` | Profiles + today's totals |
| `GET /api/usage?profile=&project=&since=` | Aggregated usage rows |
| `GET /api/usage/live` | SSE stream, emits on fsnotify changes |
| `GET /api/daemon/status` | Daemon runtime status (works in foreground mode too) |
| `GET /api/hooks/status?profile=` | Per-profile hook install status |
| `GET /api/sessions?profile=&status=&since=&limit=` | Recent session telemetry (newest first) |
| `GET /api/headroom` | Advisory recommendation + ranked candidates |

If you add or change a route: update `api/openapi.yaml` **and** regenerate `web/lib/api-types.ts` via `cd web && pnpm gen:api`. CI's `check:api-types` will fail otherwise.

---

## 5. Commands cheat sheet

### Go side

```bash
make help                  # list all targets
make test                  # go test -race -count=1 ./...
make lint                  # golangci-lint run
make ci                    # lint + test  ← run this before push
make build                 # stages web/out then builds dist/ccx
make integration-test      # go test -tags integration ./integration_test/...
make stage-web             # copy web/out → internal/dashboard/web-out/ for go:embed
make fmt                   # gofumpt -w .
make clean                 # remove dist, web/out, web/.next
```

### Web side (from `web/`)

```bash
pnpm install
pnpm dev                   # http://localhost:3001 — MSW serves mock data
pnpm build                 # produces ./out for Go embed
pnpm typecheck             # tsc --noEmit
pnpm test                  # vitest
pnpm e2e                   # playwright (run `pnpm e2e:install` once)
pnpm gen:api               # regenerate lib/api-types.ts from ../api/openapi.yaml
pnpm check:api-types       # CI fails if committed api-types.ts is stale
```

### Manual end-to-end smoke

```bash
# Build full binary including dashboard
cd web && pnpm install && pnpm build && cd ..
make stage-web && make build

# CLI basics
./dist/ccx version
./dist/ccx profile add personal --config-dir ~/.claude
./dist/ccx usage

# Foreground dashboard
./dist/ccx dashboard --no-open                  # then curl http://127.0.0.1:7777/api/health

# Daemon mode
./dist/ccx daemon start
./dist/ccx daemon status --json
./dist/ccx daemon logs
./dist/ccx daemon stop

# Hooks (writes to <profile config dir>/settings.json with a backup file)
./dist/ccx hooks install --profile personal
./dist/ccx hooks status --json
./dist/ccx hooks uninstall --profile personal

# Advisory suggest
./dist/ccx suggest --json
```

---

## 6. Engineering conventions (must follow)

Full source of truth: [`docs/conventions.md`](docs/conventions.md). Highlights:

- **Format:** `gofumpt` (not vanilla `gofmt`). Pre-commit hook enforces.
- **Lint:** `golangci-lint` with config in `.golangci.yml`. CI gates.
- **Errors:** always wrap with context: `fmt.Errorf("loading profile %q: %w", name, err)`.
  Shared sentinels live in `internal/contracts/errors.go`. Detect with `errors.Is`.
- **Logging:** `log/slog` (stdlib only). Default INFO, `--verbose` → DEBUG.
  Standard field names: `profile`, `path`, `count`, `duration`, `err`.
- **Context:** every I/O or blocking public function takes `ctx context.Context` as its first parameter.
- **Exit codes:** 0 success · 1 user error · 2 internal · 64 EX_USAGE · 70 EX_SOFTWARE · 73 EX_CANTCREAT · 74 EX_IOERR.
- **Tests:** table-driven for pure functions; `_test` package suffix preferred; fixtures in `testdata/`; CI runs with `-race -count=1`.
- **Package boundaries:** every `internal/*` leaf package imports from `internal/contracts` + stdlib + its declared deps **only**. Wiring lives in `internal/cli/`, `internal/server/`, and `internal/daemon/` — those *are* allowed to import multiple internal packages because they are the integration layer. Leaf packages (scanner, storage, profile, pricing, shell, platform, hooks, headroom, dashboard, tui, doctor) do not import siblings.
- **Commits:** Conventional Commits: `type(scope): subject`. Scope = package name. One logical change per commit.

### Documentation comments

Every exported symbol gets a Go doc comment starting with the symbol's name (enforced by `revive`'s `exported` rule). Example:

```go
// SaveProfile persists the given profile, replacing any existing entry with
// the same name. It is idempotent.
func (s *Store) SaveProfile(ctx context.Context, p contracts.Profile) error {
```

---

## 7. Hard rules (do not violate)

1. **No proxying Anthropic traffic.** ccx never sees or relays API requests. If you find yourself writing HTTP client code that talks to `api.anthropic.com`, stop — that is out of scope and (per Feb 2026 ToS) actively prohibited for Pro/Max OAuth credentials.
2. **All HTTP servers bind to `127.0.0.1` only.** Never `0.0.0.0`. The dashboard *and* the daemon serve on loopback.
3. **No telemetry phone-home.** "Hook telemetry" in this repo means *local* SQLite rows captured from Claude Code hooks. No outbound analytics, ever. No update pings unless explicitly opt-in.
4. **No outbound network calls from the binary** at runtime. `go mod download` at build time is fine; runtime is offline-by-default.
5. **Never log credential contents** or the path of a non-default `.credentials.json`. Only surface those under explicit `--debug`.
6. **No CGo.** Use `modernc.org/sqlite` (pure Go). Adding `mattn/go-sqlite3` or any CGo dep breaks the Windows cross-compile story and will be reverted.
7. **Do not edit frozen files from a feature worktree** (see §2). If a contract needs to change, pause and open a contract-amendment PR against `main`.
8. **Single binary install story.** Don't add runtime dependencies on `node`, `npm`, or any other binary outside `claude` itself.
9. **Hooks edit user files. Always back up.** `ccx hooks install` writes `settings.json.ccx-backup-*` next to the file before modifying. Preserve and extend that behavior. Never blow away unrelated user-defined hooks; only touch ccx-managed entries.

---

## 8. Workflow for a single task (bite-sized loop)

```
1. Write a failing test that pins the new behavior.
2. Run `go test ./internal/<pkg>/...` — confirm it fails for the right reason.
3. Implement the smallest change that makes it pass.
4. Run the tests again — green.
5. Run `make lint` (or `golangci-lint run ./internal/<pkg>/...`).
6. `git add -p` (be deliberate) and commit with `type(scope): subject`.
```

Do **not** batch multiple tasks into one commit. Do **not** mix package changes — one package per PR.

---

## 9. The data flows (so you debug in the right place)

### Usage flow

```
[claude session] → ~/.claude*/projects/<encoded-cwd>/<uuid>.jsonl
                          │
                          ▼
              internal/scanner  (incremental, uses cursors)
                          │
                          ▼
              internal/storage  (SQLite at ~/.ccx/state.db, events table)
                          │
            ┌─────────────┴─────────────┐
            ▼                           ▼
   internal/cli (`ccx usage`)   internal/server (/api/usage, SSE /api/usage/live)
                                        │
                                        ▼
                                 web/ (Next.js, embedded via go:embed)
```

### Daemon flow

```
ccx daemon start
      │
      ▼
~/.ccx/daemon.{pid,json,log,lock}   ← lock + status, single-writer
      │
      ├── fsnotify on every registered profile's project dir → coalesced ingest
      │
      └── serves the dashboard HTTP API on 127.0.0.1
```

### Hooks + headroom flow

```
ccx hooks install --profile X
      │
      ▼
<X config dir>/settings.json   (backup file written first)
      │
      ▼
Claude Code session events → invokes `ccx hooks record --profile X`
      │
      ▼
internal/hooks/record.go → storage.InsertHookEvent + UpsertSessionTelemetry
                                       │
                                       ▼
                hook_events + sessions + profile_health tables

ccx suggest  (or GET /api/headroom)
      │
      ├── ingest scan for fresh usage
      ├── read ProfileLimits from profiles.toml (budgets, priority, cooldown, suggest toggle)
      ├── read recent StopFailure hooks + profile_health
      ▼
internal/headroom/evaluator.go → ranked candidates + suggested `ccx use <name>`
```

---

## 10. Web dashboard quick rules

- Static export (`next.config.mjs` sets `output: 'export'`). No SSR at runtime.
- API client types in `web/lib/api-types.ts` are **generated**, not hand-written. Regenerate after any `api/openapi.yaml` change with `pnpm gen:api`. CI fails if stale.
- Dev uses MSW to mock the API (`pnpm dev` runs `predev` that initializes the worker). Never commit `public/mockServiceWorker.js` — it's gitignored.
- Components for new surfaces live alongside existing ones: `recommendation-panel.tsx` (headroom), `recent-sessions.tsx` (sessions), `profile-cards.tsx`, `dashboard.tsx`.
- Lighthouse baseline: Performance ≥ 90, Accessibility ≥ 95. Re-run after UI changes.
- Production builds need network access for `next/font/google`. Air-gapped CI must vendor fonts first.

---

## 11. Storage / schema rules

- v1 schema source of truth: `internal/storage/schema.sql`. **Frozen.**
- v2 migration (added in PR #16) layers on top in `internal/storage/migrate.go`:
  - `hook_events` (one row per hook payload)
  - `sessions` (one row per session, upserted from hook events)
  - `profile_health` (latest auth health check per profile)
  - new columns on `profiles` for `ProfileLimits` (`daily_token_budget`, `weekly_token_budget`, `monthly_usd_budget`, `priority`, `suggest_enabled`, `rate_limit_cooldown`)
- `InsertEvents(ctx, profileName, events)` — note the explicit `profileName` param. Events from JSONL don't carry profile context; the caller supplies it.
- Cursors are `(profile_name, file_path) → (offset, inode)`. Inode is what lets the scanner detect log rotation; don't drop it.
- Cost (`EstimatedUSD`) is **not** stored. It is computed at query time by the caller after consulting `contracts.PricingTable.Cost`.
- Concurrency: `internal/storage` is the single writer. The daemon coalesces ingest scans behind a write mutex; don't add a second writer.

---

## 12. Daemon rules

- Lock file (`daemon.lock`) ensures a single live daemon per `~/.ccx/`. Inherits identity via `platform.ProcessIdentity` so a stale pid file does not block startup forever.
- When forking into the background, the parent holds the lock until the child confirms readiness (env vars `CCX_DAEMON_LOCK_TOKEN`, `CCX_DAEMON_LOCK_PARENT_PID`, `CCX_DAEMON_LOCK_HELD_BY_PARENT`). Don't rewrite this without understanding why both sides are required.
- Foreground (`--foreground`) and detached modes share the same `Run()`. The only difference is who holds stdio.
- Watcher batches fsnotify events to avoid hammering the scanner on bulk writes.
- `ccx daemon status` works even when no daemon is running — it returns `{running: false}` derived from the status file.

---

## 13. Hooks rules

- ccx-managed hooks are *additive*. They live alongside any pre-existing user hooks in `settings.json` under specific keys.
- Every install/uninstall writes `settings.json.ccx-backup-<timestamp>` before mutating.
- `disableAllHooks: true` in the user's settings is **respected**. Status returns `disabled`. Install becomes a no-op (unless `--force`); the user must remove the flag themselves.
- `ccx hooks record --profile <name>` reads a JSON payload from stdin and writes one hook event + upserts session telemetry. This is the command Claude Code invokes via the installed hook; humans should not run it directly.
- Failure facts (`StopFailure`) are replaced atomically per session — the latest failure wins, older ones are not appended.

---

## 14. Pricing rules

- Embedded `pricing/models.yaml` is the baseline.
- User override at `~/.ccx/pricing.yaml` layers on top by model name.
- Always label currency displays as "Estimated USD" — Anthropic rates change.
- New model? Add it to `pricing/models.yaml` with an `effective_from` date.
- Do not hard-code rates in Go. The pricing package reads the YAML.

---

## 15. Security checklist before opening a PR

- [ ] No new outbound HTTP call from the binary at runtime.
- [ ] Dashboard *and* daemon server still bind 127.0.0.1 only (`net.Listen("tcp", "127.0.0.1:…")`).
- [ ] No `slog` line logs `.credentials.json` content or path unless gated on `--debug`.
- [ ] No new CGo dependency in `go.mod`.
- [ ] No new background goroutine that survives `ctx.Done()`.
- [ ] If you added or changed an API endpoint, you updated `api/openapi.yaml` and regenerated `web/lib/api-types.ts`.
- [ ] If you touched hook installation, the backup-before-write behavior is preserved.
- [ ] If you touched the daemon lock, you tested both startup races (parent-held) and stale-pid recovery.

---

## 16. When the user asks "just make it work"

Resist. The reasons this codebase is split into 14+ packages with frozen contracts are recorded in `HANDOFF.md` and `docs/superpowers/specs/`. If a shortcut means crossing a package boundary, importing a sibling `internal/*` leaf, or editing a frozen file, the right answer is to:

1. Stop, surface the constraint, and propose a contract-amendment PR.
2. Or scope the work down to fit inside the current package.

A clean PR that lands in one package beats a sprawling PR that has to be unwound.

---

## 17. Pointers to deeper reading

- [`README.md`](README.md) — user-facing pitch and install.
- [`docs/architecture.md`](docs/architecture.md) — component diagram + usage / daemon / hooks / headroom flow diagrams.
- [`docs/conventions.md`](docs/conventions.md) — non-negotiable style + workflow rules.
- [`docs/shell-integration.md`](docs/shell-integration.md) — how `ccx use` actually switches accounts.
- [`docs/troubleshooting.md`](docs/troubleshooting.md) — user-facing FAQ; covers daemon, hooks, suggest failure modes.
- [`docs/superpowers/specs/`](docs/superpowers/specs/) — original design spec.
- [`docs/superpowers/plans/`](docs/superpowers/plans/) — historical execution plans (Phases 0–3).
- [`HANDOFF.md`](HANDOFF.md) — phase snapshot (stale on daemon/hooks/headroom; trust git log + this file).
- [`api/openapi.yaml`](api/openapi.yaml) — HTTP API contract.
- [`internal/contracts/`](internal/contracts/) — Go interfaces every other package implements.
