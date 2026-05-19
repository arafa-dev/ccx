# ccx — Design Specification

**Status:** Approved — ready for implementation planning
**Date:** 2026-05-19
**Owner:** @arafa-dev
**Target release:** v0.1.0
**Estimated effort:** 80-150 hours across 8-10 weeks at 10-15 hrs/week
**Target launch:** 2026-07-21 (8-week mark; adjust based on actual velocity)
**Hard floor on time commitment:** if weekly hours drop below 10, dashboard is the first cut to keep the timeline.

---

## 1. One-line pitch

> A power-user workspace manager for Claude Code — switch between accounts in one command, see your real usage across all of them, all from a single Go binary.

## 2. Why this exists

Claude Code power users often run more than one account (Pro + Max, work + personal, multiple Pro accounts). Today they juggle shell aliases, manually swap `~/.claude/` directories, and use `ccusage` separately for analytics. There is no single tool that does both account switching and usage analytics in one polished binary.

**The validated technical premise** (see Appendix A for research): Claude Code respects `CLAUDE_CONFIG_DIR` for account isolation on all three platforms. On macOS the keychain service name is derived from the config-dir path via SHA256, so credential routing happens automatically. On Linux and Windows, `.credentials.json` lives inside `CLAUDE_CONFIG_DIR`. This means a CLI can cleanly switch between credential sets without forcing re-login.

**Positioning vs alternatives:**

| Tool | Account switching | Usage analytics | Cross-platform | Distribution |
|---|---|---|---|---|
| `ccusage` | ❌ | ✅ (best) | ✅ | npm |
| `claude-account-switcher` | ✅ (bash) | ❌ | macOS-focused | shell script |
| `clausona` | ✅ | ❌ | TS | npm |
| `ccs / cc-switch` | ✅ (multi-provider) | partial | TS + Tauri | github releases |
| **`ccx`** | **✅** | **✅** | **macOS/Linux/Windows** | **brew/scoop/apt/go install** |

The moat is **"both, in one binary, properly cross-platform."**

## 3. Scope

### 3.1 In scope for v0.1

CLI commands:
- `ccx profile add <name> --config-dir <path>`
- `ccx profile list` — table with name, path, last-used, today's tokens, today's $
- `ccx profile rm <name>`
- `ccx profile current`
- `ccx use [<name>]` — name given: prints `export ...` for eval; no name: opens TUI picker
- `ccx init <shell>` — emits rc snippet for zsh, bash, fish, pwsh
- `ccx usage [--profile <name>] [--since <dur>] [--json]` — default: 24h, all profiles
- `ccx dashboard [--port <n>] [--no-open]` — boots local server + opens browser
- `ccx doctor` — diagnostic checks
- `ccx version`, `ccx --help`

Capabilities:
- macOS + Linux + Windows support (amd64 + arm64 where applicable)
- Local SQLite cache with incremental scanning
- Embedded Next.js dashboard served via `go:embed`
- Live updates via fsnotify while dashboard is open (short-lived watcher per dashboard session)
- TUI picker via bubbletea
- Distribution via Homebrew tap, Scoop bucket, .deb + .rpm (nfpm), GitHub Releases, `go install`, and `curl|sh` script

### 3.2 Explicitly out of scope for v0.1 (deferred to later releases)

- Standalone long-running daemon (v0.3)
- Routing prompts to the account with budget (v0.4+, and only as advisory — never as proxy, per Anthropic ToS)
- Claude Code hooks/MCP integration (v0.2+)
- Team workspace primitives, profile sync (v0.5+)
- iOS/Android companion apps (no)
- Custom telemetry collection beyond what JSONL provides (v0.3+)

### 3.3 Non-goals

- We do **not** proxy or relay Claude API requests. The user runs `claude` themselves; ccx only manipulates the environment it runs in. This stays clear of Anthropic's Feb 2026 Usage Policy update banning third-party tools from routing requests through Pro/Max OAuth credentials.
- We do **not** read or transmit credential contents anywhere. Credentials stay where Claude Code put them (keychain on macOS, file on Linux/Windows).
- We do **not** require the user to log in to ccx. There are no ccx accounts.

## 4. Product surface (final)

```
ccx profile add <name> --config-dir <path>
ccx profile list
ccx profile rm <name>
ccx profile current

ccx use [<name>]                              # eval-style; opens TUI picker if no name
ccx init <shell>                              # zsh | bash | fish | pwsh

ccx usage [--profile <name>] [--since <dur>] [--json]

ccx dashboard [--port <n>] [--no-open]

ccx doctor
ccx version
ccx --help
```

**UX principles:**
- `ccx use` follows the `nvm`/`pyenv` pattern: prints shell commands to stdout, designed to be wrapped in `eval`. We do not try to modify the parent shell directly.
- `ccx init <shell>` emits a wrapper function so users can type `ccx use work` (no `eval` needed) after one-time rc-file setup.
- Plain table output by default; `--json` for machine consumption; no bubbletea except for the picker.
- Dashboard is dark mode by default with a toggle. Designed to look good in screenshots.

## 5. Architecture

### 5.1 Repository layout

```
ccx/
├── cmd/ccx/main.go                # cobra entry point only
├── internal/
│   ├── contracts/                 # Go interfaces + shared types (Phase 0 artifact)
│   ├── cli/                       # cobra commands (one file per command)
│   ├── profile/                   # profile model, TOML registry I/O
│   ├── shell/                     # shell snippet generators per shell
│   ├── scanner/                   # JSONL parser + project dir walker
│   ├── pricing/                   # model→$ rate tables (YAML), cost calc
│   ├── storage/                   # SQLite layer + migrations
│   ├── server/                    # localhost HTTP server (net/http + chi)
│   ├── tui/                       # bubbletea picker
│   ├── doctor/                    # diagnostic checks
│   ├── platform/                  # OS-specific code (config-dir, shell detection)
│   └── dashboard/                 # //go:embed all:web/out
├── api/openapi.yaml               # HTTP server contract
├── web/                           # Next.js 15 dashboard (static export)
│   ├── app/ components/ lib/
│   └── package.json
├── pricing/models.yaml            # embedded via go:embed
├── testdata/jsonl/                # anonymized fixtures
├── docs/
│   ├── architecture.md
│   ├── shell-integration.md
│   ├── conventions.md             # error wrapping, exit codes, log fields
│   └── superpowers/specs/         # this doc lives here
├── .github/workflows/             # ci.yml, release.yml, codeql.yml
├── .goreleaser.yaml
├── Makefile
├── go.mod
└── README.md
```

### 5.2 Library choices

| Choice | Why |
|---|---|
| `spf13/cobra` | De-facto Go CLI standard (k8s, hugo, gh) |
| `modernc.org/sqlite` | Pure-Go SQLite — clean Windows cross-compilation, no CGo |
| `charmbracelet/bubbletea` + `lipgloss` | Standard Go TUI; high quality |
| `fsnotify/fsnotify` | Cross-platform file watching |
| `go-chi/chi` | Stdlib-compatible HTTP router; no DI magic |
| `pelletier/go-toml/v2` | TOML for profile registry |
| stdlib `log/slog` | Built-in structured logging |
| `golang-migrate/migrate` | SQL migrations |
| Next.js 15 (App Router) + Tailwind + Recharts | Dashboard frontend |
| `goreleaser/goreleaser` + nfpm | Multi-platform binaries + deb/rpm |
| `golangci-lint` + `gofumpt` | Linting + formatting |
| `lefthook` | Pre-commit hooks (faster than husky) |
| `cosign` | Release artifact signing |

### 5.3 Build flow

1. `pnpm --filter web build` produces `web/out/`
2. `go build -tags release` embeds `web/out/` via `embed.FS`
3. Goreleaser cross-compiles for darwin/linux/windows × amd64/arm64
4. nfpm produces `.deb` and `.rpm`; brew tap and scoop bucket auto-update on tag push

## 6. Profile switching mechanism

**Profile model:**

```go
type Profile struct {
    Name        string
    ConfigDir   string    // absolute path
    Label       string    // optional human label
    Color       string    // hex, optional, used in dashboard
    CreatedAt   time.Time
    LastUsedAt  time.Time
}
```

Registry stored as `~/.ccx/profiles.toml`. The `ConfigDir` value is the only thing that determines identity — `CLAUDE_CONFIG_DIR=<that path>` is what isolates the account.

**Switching mechanism (POSIX shells):**

```
$ eval "$(ccx use work)"

# ccx emits to stdout:
export CLAUDE_CONFIG_DIR="/Users/arafa/.claude-profiles/work"
export CCX_ACTIVE_PROFILE="work"
```

**Switching mechanism (PowerShell):**

```
PS> ccx use work | Invoke-Expression

# ccx emits to stdout:
$env:CLAUDE_CONFIG_DIR = "C:\Users\arafa\.claude-profiles\work"
$env:CCX_ACTIVE_PROFILE = "work"
```

The downstream `claude` CLI then:
- Reads config from `CLAUDE_CONFIG_DIR`
- On macOS: derives the keychain service name from that path via SHA256 — automatically picks up the right credential
- On Linux/Windows: reads `.credentials.json` from that dir — naturally isolated

**Shell init (`ccx init <shell>`):** Emits a wrapper function so the user does not need to type `eval` repeatedly. Example for zsh/bash:

```bash
ccx() {
  if [[ "$1" == "use" ]]; then
    eval "$(command ccx use "${@:2}")"
  else
    command ccx "$@"
  fi
}
```

The user pastes this into their rc file once. After that, `ccx use work` "just works."

**First-time profile creation flow:**

```
$ ccx profile add work --config-dir ~/.claude-profiles/work

→ Validates path; offers to create if missing
→ If creating: makes empty dir + minimal config skeleton
→ Writes to ~/.ccx/profiles.toml
→ Prints: "Profile 'work' added. Run `ccx use work` then `claude /login` to authenticate."
```

**Critical UX detail:** On first switch to a new profile, no credentials exist yet. ccx must tell the user to run `claude /login`. This is the one moment the abstraction leaks; we surface it explicitly.

**Active-profile detection (`ccx profile current`):**

1. Read `$CCX_ACTIVE_PROFILE` from env. If set, look up in registry, show name + path + last-used + today's $.
2. Else, if `$CLAUDE_CONFIG_DIR` is set, show "unmanaged config: <path>".
3. Else, show "default profile (~/.claude or ~/.config/claude)".

**Doctor checks (`ccx doctor`)** report each as ✅/⚠/❌:
- `claude` on PATH? Version?
- Default config dir exists?
- ccx registry readable?
- Each registered profile: config dir exists? credentials present?
- Shell init detected in user's rc file?
- macOS only: keychain entry findable for each profile?

**Edge cases handled:**
- User deletes a profile's config dir manually → doctor flags, list shows ⚠
- Two profiles point at the same `config_dir` → rejected at `add`
- User runs `claude` without `eval` first → default account behavior, expected
- Pre-existing `~/.claude` → `ccx profile add default --config-dir ~/.claude` adopts cleanly

## 7. Usage parsing & storage

### 7.1 Scanner

For each profile, walk `<config_dir>/projects/*/<session-uuid>.jsonl`. Each line is one event. Parser is defensive — the JSONL format is reverse-engineered, not officially specced.

```go
type Event struct {
    UUID         string
    SessionID    string
    Timestamp    time.Time
    Type         string
    Project      string
    Model        string
    Usage        *Usage
}

type Usage struct {
    InputTokens, OutputTokens             int
    CacheReadTokens, CacheCreateTokens    int
}
```

**Defensive parsing rules:**
- Unknown event types → log at debug, skip
- Unknown fields → ignored (default Go JSON behavior)
- Malformed lines → log warning with `file:line`, skip
- Never panic on bad input
- Fuzz-tested (`go test -fuzz=`)

### 7.2 SQLite schema

```sql
CREATE TABLE profiles (
    name           TEXT PRIMARY KEY,
    config_dir     TEXT UNIQUE NOT NULL,
    label          TEXT,
    color          TEXT,
    created_at     INTEGER,
    last_used_at   INTEGER
);

CREATE TABLE events (
    profile_name        TEXT NOT NULL,
    session_id          TEXT NOT NULL,
    event_uuid          TEXT NOT NULL,
    ts                  INTEGER NOT NULL,
    project             TEXT,
    model               TEXT,
    input_tokens        INTEGER,
    output_tokens       INTEGER,
    cache_read_tokens   INTEGER,
    cache_create_tokens INTEGER,
    PRIMARY KEY (profile_name, event_uuid)
);

CREATE INDEX events_profile_ts ON events(profile_name, ts);
CREATE INDEX events_project    ON events(project);

CREATE TABLE scan_cursors (
    profile_name TEXT NOT NULL,
    file_path    TEXT NOT NULL,
    offset       INTEGER NOT NULL,
    inode        INTEGER,
    PRIMARY KEY (profile_name, file_path)
);

CREATE TABLE schema_version (version INTEGER NOT NULL);
```

**Incremental scanning:** Remember byte offset + inode per JSONL file. On re-scan, only read from offset forward. If inode changed (file rotated/replaced), reset to 0.

### 7.3 Pricing

Embedded YAML, queryable by model name + date:

```yaml
- model: claude-opus-4-7
  effective_from: 2026-01-15
  input_per_mtok:        15.00
  output_per_mtok:       75.00
  cache_read_per_mtok:    1.50
  cache_create_per_mtok: 18.75
- model: claude-sonnet-4-6
  effective_from: 2026-01-15
  input_per_mtok:         3.00
  output_per_mtok:       15.00
  cache_read_per_mtok:    0.30
  cache_create_per_mtok:  3.75
```

Cost calc: `(tokens / 1_000_000) * applicable_rate_at_ts`. README and CLI clearly mark costs as "estimated." Users can override by editing `~/.ccx/pricing.yaml`.

### 7.4 Output formats

`ccx usage` default (24h, all profiles):

```
Usage for last 24h, all profiles

PROFILE   TOKENS (in/out/cache)        EST. COST   TOP PROJECT
work      1.2M / 240k / 4.1M           $18.42      acme-api
personal  220k / 84k / 980k             $4.18       hobby-site
side      0 / 0 / 0                     $0.00       —

Total: $22.60
```

`--json` output structured for piping.

### 7.5 Performance targets

- `ccx usage` cold (first run, no SQLite cache): **<500ms** for typical user (~100MB JSONL across profiles)
- `ccx usage` warm (SQLite primed, incremental scan): **<50ms**
- Scanner throughput: **>100MB/s** on a modern laptop (M1/M2 baseline)
- Targets benchmarked in `BENCHMARKS.md`. Regressions surface in CI as a soft warning (not a hard merge gate; performance work is real but cost-of-CI-flakiness is also real).

## 8. Dashboard (`ccx dashboard`)

### 8.1 Architecture

```
ccx dashboard  →  127.0.0.1:<port>  →  serves embedded web/out + JSON API

Go server endpoints:
    GET /api/profiles
    GET /api/usage?profile=&since=
    GET /api/usage/live (SSE; emits on JSONL file change via fsnotify)
    GET /api/health
    GET / and /*  (serves embedded Next.js static export via embed.FS)
```

### 8.2 Security posture

- Bind to **127.0.0.1 only** — never 0.0.0.0
- Strict CSP headers + `X-Content-Type-Options: nosniff` + `Referrer-Policy: no-referrer`
- No auth (overkill for localhost-only) but documented as such in `docs/security.md`
- No outbound network calls from the server process (offline-by-default)

### 8.3 Live updates

While the dashboard process is running, a short-lived fsnotify watcher monitors each profile's `projects/` dir. On change, parse the new lines, insert into SQLite, and broadcast over SSE. When the user closes the dashboard, the process exits and the watcher stops. No daemon required in v0.1.

### 8.4 Frontend (Next.js 15, App Router, static export)

Single-page layout, scrolls:
1. **Header bar** — ccx logo, profile picker (view-only filter), live-status dot
2. **Profile cards row** — today's spend, today's tokens, 7-day sparkline per profile
3. **Time-series chart** — stacked area chart, daily tokens segmented by profile (Recharts)
4. **Top projects table** — sortable by tokens / cost / sessions
5. **Recent sessions** — most recent 20, with project + duration + cost
6. **Footer** — version, GitHub link, last-refreshed timestamp

Visual style:
- Dark mode default + toggle
- JetBrains Mono for numbers, Inter for body
- Each profile has a stable accent color from its registry `color` field
- "Above the fold" view (profiles row + chart) fits a 14" laptop without scroll — this is what gets screenshotted

### 8.5 Build integration

- `make web` → `pnpm install && pnpm --filter web build` → `web/out/`
- Go build picks up `web/out/` via `//go:embed all:web/out`
- If `web/out/` missing at build time, build fails with a message pointing to `make web`
- Dev mode: `make dev` runs `next dev` on :3001 + Go server on :7777 with CORS to localhost:3001

### 8.6 Failure modes

- Port 7777 in use → fall back through [7778-7787]
- No profiles registered → onboarding screen with copy-paste `ccx profile add` commands
- Empty data → "No sessions yet. Run `claude` to start tracking."
- Server crash → browser shows "ccx isn't running" page (served from cached HTML if available)

### 8.7 Trade-offs accepted

- Binary size: embedded frontend adds ~3-5MB. Acceptable for screenshot value.
- Two languages = two test surfaces. Frontend gets Vitest unit tests + one Playwright happy-path E2E.
- Largest single source of complexity in v0.1. **If timeline slips, dashboard is the first thing cut** — CLI and distribution are not negotiable.

## 9. Testing, CI, quality gates

### 9.1 Test pyramid

| Layer | What | How |
|---|---|---|
| Unit | Pure functions: parser, pricing, shell snippets, registry validation | Go table-driven tests |
| Golden file | Parser output, shell snippets, CLI command output | `testdata/golden/*.txt` + `-update` flag |
| Fuzz | JSONL parser must never panic on arbitrary bytes | `go test -fuzz=` |
| Integration | `ccx profile add → use → claude --version reads right config` | Subprocess tests in `tests/integration/` |
| Cross-platform | All above × {macOS, Linux, Windows} | GitHub Actions matrix |
| Frontend | Dashboard renders, fetches API, charts populate | Vitest + one Playwright E2E |
| Benchmark | `ccx usage` cold/warm targets, parser throughput | `go test -bench=` + `BENCHMARKS.md` |

**Coverage target:** 70%+ on `internal/`. Not a merge gate, but visible in CI (Codecov badge).

### 9.2 Linting and formatting

- `golangci-lint` strict (`govet`, `staticcheck`, `errcheck`, `gosec`, `gocritic`, `revive`)
- `gofumpt` for Go formatting
- Frontend: `eslint`, `prettier`, `typescript --strict`
- All run pre-commit via `lefthook`

### 9.3 CI workflows

```
ci.yml — on every PR:
    matrix: macos-14, macos-14-arm, ubuntu-22.04, ubuntu-22.04-arm, windows-2022
    lint → test → build → upload-artifact
    integration tests gated behind `[integration]` build tag (main + nightly)

release.yml — on tag push (v*):
    goreleaser release --clean
    publishes to: GitHub Releases, Homebrew tap, Scoop bucket, .deb/.rpm
    cosign-signs binaries

codeql.yml — weekly + PR (Go + JS)

dependabot.yml — weekly (Go modules + npm + GH Actions)
```

### 9.4 Merge gates (block PR merge)

1. Lint clean
2. Tests pass on all OS
3. No new high/critical findings from gosec / CodeQL
4. Frontend type-check clean
5. Cross-compile sanity check passes

### 9.5 Error and logging conventions

- Wrap with `fmt.Errorf("loading profile %q: %w", name, err)` — always include context
- Sentinel errors: `ErrProfileNotFound`, `ErrInvalidConfigDir`, `ErrAlreadyExists`
- Exit codes: 0 success, 1 user error, 2 internal error, 64-78 per `sysexits.h`
- User-facing errors: red `Error:` prefix via `lipgloss`; full stack at `--debug`
- Structured logging via `log/slog`; `--log-format=json` for JSON output; `--verbose` raises to debug

## 10. Distribution & launch

### 10.1 Goreleaser config

- Cross-compile darwin/linux/windows × amd64/arm64 (6 binaries per release)
- `-trimpath -ldflags="-s -w -X main.version={{.Version}}"`
- nfpm builds `.deb` + `.rpm` with conf files in `/etc/ccx`
- Homebrew tap: `arafa-dev/homebrew-ccx`
- Scoop bucket: `arafa-dev/scoop-ccx`
- GitHub Releases: tarballs + SHA256 checksums + cosign signatures
- `install.sh` one-liner served from GitHub Pages (or jsr.io if simpler): `curl -fsSL https://ccx.sh/install | sh`

### 10.2 README structure

```
1. Hero GIF (vhs-rendered terminal: `ccx use work` + `ccx usage` in 4-6 sec)
2. One-line pitch
3. Install commands above the fold (brew, scoop, apt, curl|sh, go install)
4. 60-second quick-start
5. Dashboard screenshot (full-width, dark mode)
6. Why ccx? (authentic story — switching pain, ccusage doesn't switch)
7. How it works (small architecture diagram)
8. Comparison table vs ccusage, claude-account-switcher, ccs
9. Configuration
10. Roadmap (v0.2 daemon, v0.3 hooks integration, v0.4 advisory routing)
11. Contributing + License (MIT)
```

### 10.3 Pre-launch checklist (week before HN)

- 3-5 friends install on their machines; watch them on a call. Fix top 3 friction points.
- Demo GIF re-recorded with `vhs`; demo video (60 sec, OBS) for tweet
- 3 issues tagged `good-first-issue` ready
- `CHANGELOG.md` filled out for v0.1.0
- Hand-craft HN title + first comment
- Tweet thread drafted

### 10.4 Launch day sequencing

1. **Soft launch (day -7):** Push v0.1.0-rc1 to GitHub. 3-5 friends try it.
2. **Fix friction (day -7 → -1):** Address blockers from soft launch.
3. **Tag v0.1.0** launch-day morning.
4. **Show HN** at 9:00 AM Pacific.
5. **Tweet thread + GIF** immediately after HN goes live.
6. **r/ClaudeAI, Anthropic Discord, DEV.to** within 4 hours.
7. **First 24h:** Reply to every comment and issue within 1 hour.

### 10.5 Realistic outcome expectations

Given crowded space (ccusage is well-known, several account switchers exist):
- **Stars target week 1:** 100-300. 500+ possible if HN front-pages, not baseline.
- **Even 100 stars + a few sustained issues is enough CV signal.** Set expectations honestly.

## 11. Parallel-agent implementation strategy

This project will be built using multiple AI agents working concurrently in separate git worktrees. The architecture above is designed for this: every `internal/` subdir has a single responsibility and zero cross-imports except through `internal/contracts/`. The implementation proceeds in four phases.

### 11.1 Phase 0 — Contracts (single agent, ~3-5 hrs, MUST complete before Phase 1)

This phase is the "design treaties" between agents. Outputs that lock down before any parallel work:

1. `go.mod` — module path, Go 1.22+ pinned
2. `internal/contracts/` — Go interfaces and shared types every other package imports:
   - `Profile`, `Event`, `Usage` structs
   - `Scanner`, `Store`, `PricingTable`, `ShellEmitter` interfaces
3. `internal/storage/schema.sql` — final SQLite schema
4. `api/openapi.yaml` — HTTP server contract (allows frontend to mock the API)
5. `docs/conventions.md` — error wrapping, log fields, exit codes, lint config
6. `Makefile` — skeleton targets (`make build`, `make web`, `make test`, `make dev`)
7. `.github/workflows/ci.yml` — skeleton (lint + test on PR)
8. Empty `internal/<package>/` directories with package docs so `go build ./...` works

**Exit criteria:** `go build ./...` and `go test ./...` succeed on empty implementations. PR merged to `main`.

### 11.2 Phase 1 — Parallel package implementation (multiple agents, ~3-4 weeks)

Each agent works in its own git worktree off `main`. Packages have zero cross-imports (only `contracts/`). PRs are reviewed and merged one at a time; conflicts limited to top-level files.

| Worktree | Package | Depends on | Deliverables |
|---|---|---|---|
| A1 | `internal/profile/` | contracts | Registry I/O, validation, CRUD + tests |
| A2 | `internal/scanner/` | contracts | JSONL parser, dir walker, fuzz tests |
| A3 | `internal/storage/` | contracts, schema.sql | sqlite layer, migrations, cursor logic |
| A4 | `internal/pricing/` | contracts | YAML loader, cost calc + tests |
| A5 | `internal/shell/` | contracts | POSIX + PowerShell + fish snippet generators |
| A6 | `internal/platform/` | contracts | OS detection, default config dir resolution |
| A7 | `web/` (Next.js) | openapi.yaml | Dashboard frontend, mocks API via MSW |
| A8 | `.goreleaser.yaml` + Homebrew/Scoop taps | none | Distribution pipeline |
| A9 | `docs/` + README + vhs GIF tape | none | All documentation work |

**Rules of engagement:**
- Any agent's PR must include passing tests for its own package
- An agent never modifies files outside its assigned directory (except top-level shared like `go.sum`)
- If a contract needs to change, raise an issue and pause that worktree. A single "contract-amendment" PR is opened by one agent (or you), reviewed, and merged. All Phase 1 worktrees then rebase off the new `main`. Direct edits to `contracts/` from a feature worktree are forbidden.
- Each Phase 1 PR template asks: "Did this work require any contract change? If yes, link the amendment PR."

### 11.3 Phase 2 — Integration (one synthesizer agent, ~1-2 weeks)

Once Phase 1 packages are merged, one agent does the wiring. This is the only phase that touches multiple packages at once.

| Step | What | Notes |
|---|---|---|
| I1 | `internal/cli/` cobra commands | Wires every package through the cli layer |
| I2 | `internal/server/` chi routes | Implements openapi.yaml against `Store` |
| I3 | `internal/tui/` bubbletea picker | Uses profile + storage |
| I4 | `internal/doctor/` | Reads from every package |
| I5 | `internal/dashboard/` `embed.FS` glue | Trivial |
| I6 | `cmd/ccx/main.go` | Final entry point |
| I7 | End-to-end integration tests | First time the whole system runs together |
| I8 | Expand CI matrix to full OS list | macOS, Linux, Windows × amd64/arm64 |

### 11.4 Phase 3 — Polish & launch (single agent, ~1 week)

| Step | What |
|---|---|
| P1 | Friend-test on 5 machines, fix top friction points |
| P2 | Re-record demo GIF + screenshots |
| P3 | Final README pass |
| P4 | Tag v0.1.0 |
| P5 | HN + Twitter + Discord launch |

### 11.5 Risks of parallel approach + mitigations

| Risk | Mitigation |
|---|---|
| **Contract drift** — if Phase 0 contracts are wrong, every Phase 1 agent rebuilds | Spend at least 3 hours on Phase 0; review every contract file before any Phase 1 agent starts |
| **Hidden coupling** — two agents discover unspecced cross-package needs | Cross-package questions go through a contract-amendment PR, never direct edits |
| **Drift in shared style** — different agents pick different patterns | `docs/conventions.md` locks: error wrapping, log fields, naming, lint config |
| **Merge conflicts in top-level files** (go.mod, README) | Synthesizer agent owns these; Phase 1 agents only edit their dirs |
| **Phase 1 agent over-engineers** for hypothetical future needs | Each agent's prompt explicitly references YAGNI, scoped task, and "no abstractions beyond what tests need" |

## 12. Roadmap (post-v0.1)

- **v0.2 (2026-08):** standalone daemon mode (`ccx daemon start`), persistent file watching, faster dashboard refresh
- **v0.3 (2026-09):** Claude Code hooks integration for per-session telemetry (exit codes, durations)
- **v0.4 (2026-10):** advisory routing — `ccx suggest` recommends the profile with most headroom, plus rate-limit detection
- **v0.5+:** team workspace primitives, profile registry sync via git, optional cloud-hosted analytics (opt-in)

## 13. CV bullets this becomes

```
ccx — Multi-account workspace manager for Claude Code     github.com/arafa-dev/ccx
• Cross-platform Go CLI (macOS/Linux/Windows × amd64/arm64) with embedded Next.js
  dashboard served from a single binary via go:embed
• [X] GitHub stars, [Y] weekly downloads, [Z] contributors
• Designed JSONL streaming parser handling 100MB+ session files in <500ms via
  inode-aware incremental scanning and SQLite-backed aggregation
• Shipped multi-channel distribution (Homebrew tap, Scoop bucket, .deb/.rpm via
  goreleaser+nfpm, signed releases via cosign) — install-to-first-use in <60s
```

---

## Appendix A — Technical viability research (2026-05-19)

Sources verified via WebSearch and official Anthropic docs:

**Verified:** `CLAUDE_CONFIG_DIR` is officially documented in Anthropic's authentication docs. On Linux and Windows, it relocates both config and `.credentials.json`. Quote: *"If you've set the CLAUDE_CONFIG_DIR environment variable on Linux or Windows, the .credentials.json file lives under that directory instead."*

**Likely (reverse-engineered):** On macOS, Claude Code derives the keychain service name from the config-dir path via SHA256, so switching `CLAUDE_CONFIG_DIR` automatically routes to a different keychain entry. Multiple community tools (claude-account-switcher, clausona, claude-code-profiles, ccs) confirm this works in practice. Not formally documented.

**Verified credential storage:**
- macOS: encrypted Keychain (service typically `"Claude Code-credentials"`)
- Linux: `~/.claude/.credentials.json`, mode `0600`
- Windows: `%USERPROFILE%\.claude\.credentials.json`, ACL-restricted

**Likely (community-maintained references):** JSONL session format at `~/.claude/projects/<url-encoded-cwd>/<session-uuid>.jsonl`. Append-only, one JSON object per line. Stable-ish fields: `type`, `uuid`, `parentUuid`, `timestamp`, `sessionId`, `cwd`, `gitBranch`, `version`. Usage in `message.usage.{input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens}`. **Format is reverse-engineered and not formally versioned — expect breakage on Claude Code updates.** Mitigation: pin testing to current Claude Code versions, follow ccusage for format updates.

**Existing tools landscape:** Crowded space (`ccusage`, `claude-account-switcher`, `clausona`, `claude-code-profiles`, `ccs/cc-switch`, `aimux`) but no Go entrant. Differentiation must be more than "switching in Go" — the unified switching + analytics positioning is the moat.

**Trademark:** "CLAUDE" is a registered Anthropic trademark. Projects with "claude" in the name currently coexist (`ccusage`, `claude-account-switcher`, `claude-code-router`) but Anthropic has forced renames before (Clawdbot → OpenClaw). The name **`ccx`** (no "Claude" string) is the safest path.

**Critical ToS update (Feb 20 / April 4, 2026):** Anthropic bans third-party tools from *routing requests through* Pro/Max OAuth credentials. **ccx is compliant as long as it only switches between the user's own credentials for the user's own claude CLI invocations and never proxies or relays API traffic.** The v0.4 "advisory routing" feature must be strictly advisory (telling the user which profile to switch to), never proxy.

**Plugin/MCP integration:** Plugins run *inside* a Claude Code session; ccx selects *which* session. Plugin model is not a natural fit for v0.1, but Claude Code's hooks API receives `CLAUDE_PROJECT_DIR` — useful for v0.3 per-profile telemetry.

**Sources:**
- Claude Code Authentication docs — `code.claude.com/docs/en/authentication`
- Claude Code env-vars docs — `code.claude.com/docs/en/env-vars`
- GitHub issue #3833 — CLAUDE_CONFIG_DIR behavior
- GitHub issue #9403 — macOS keychain service name bug
- GitHub issue #44687 — multi-account support request
- ccusage on npm
- claude-account-switcher (ukogan/GitHub)
- ccs (kaitranntt/GitHub)
- Anthropic plugins announcement (claude.com/blog/claude-code-plugins)
- The Register, 2026-02-20 — Anthropic clarifies ban on third-party tool access
- CLAUDE trademark filing (US Reg #7645254)

---

## Appendix B — Decisions log

| Decision | Why |
|---|---|
| Pick Approach B (8-10 weeks, full dashboard in v0.1) over A (CLI-only) | User chose to add dashboard + scoop + apt + TUI back into v0.1 after initial agreement on lean MVP |
| Windows in v0.1 | Implied by adding Scoop packaging |
| Go over TypeScript/Rust | User picked it for FAANG signal; pure-Go SQLite (`modernc.org/sqlite`) makes cross-compile easy |
| Vibe-coded via AI agents | User has no Go experience; will study before applying to FAANG |
| Project name "ccx" not "claudectl" | Trademark safety; Anthropic has forced renames before |
| Drop "routing prompts" from v1 pitch | Anthropic's Feb 2026 ToS bans proxy/relay of OAuth credentials. Reframed as v0.4 advisory feature only. |
| Embed Next.js via `go:embed` over separate dashboard download | Single-binary install story is core to differentiation |
| `modernc.org/sqlite` (pure Go) over `mattn/go-sqlite3` (CGo) | Clean Windows cross-compilation; no CGo toolchain needed in CI |
| TOML for profile registry over JSON/YAML | Human-editable; `~/.ccx/profiles.toml` is a hand-fixable file |
| No bubbletea except for picker | Plain tables read faster in CI logs and screenshots; bubbletea complexity reserved for the one truly interactive flow |
| Dark mode default for dashboard | Every screenshot people share will be dark; matches the audience aesthetic |
| Bind dashboard server to 127.0.0.1 only | Local-only data; no exposure surface |
| "Estimated cost" labeling everywhere | Anthropic rates change; we never want to be cited for incorrect billing claims |
