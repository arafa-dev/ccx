# ccx architecture

## High level

ccx is a single Go binary. It does three things: manage a registry of Claude
Code accounts ("profiles"), parse the JSONL session files those accounts
produce, and present that data via a CLI or an embedded web dashboard.

ccx never proxies API calls. The upstream `claude` CLI handles all Anthropic API
communication. ccx only manipulates the environment that `claude` runs in.

![architecture diagram](assets/architecture.png)

## Components

| Layer | Package | Responsibility |
| --- | --- | --- |
| Shared types | `internal/contracts` | Profile, Event, Usage structs; Scanner / Store / PricingTable interfaces; sentinel errors |
| Persistence | `internal/storage` | SQLite-backed implementation of `Store` |
| Parsing | `internal/scanner` | JSONL streaming parser with incremental cursors and shared-history attribution |
| Profile mgmt | `internal/profile` | TOML registry at `~/.ccx/profiles.toml` |
| Pricing | `internal/pricing` | Embedded model→USD rate table |
| Shell | `internal/shell` | Snippet generators for zsh/bash/fish/pwsh |
| Platform | `internal/platform` | OS detection, default config dir resolution |
| Daemon | `internal/daemon` | Background scanner, dashboard server, pid/status/log runtime files |
| Hooks | `internal/hooks` | Claude Code settings installer and hook telemetry recorder |
| Headroom | `internal/headroom` | Advisory profile ranking from budgets, quota pressure, usage, failures, and health |
| Shared migration | `internal/quotamigrate` | Symlink and migration planner for `~/.ccx/shared-projects/` |
| Run supervisor | `internal/run` | Claude launcher, pre-launch selection, and mid-session resume supervisor |
| CLI | `internal/cli` | cobra command tree |
| Server | `internal/server` | chi-routed HTTP API for the dashboard |
| TUI | `internal/tui` | bubbletea profile picker |
| Doctor | `internal/doctor` | Diagnostic checks |
| Dashboard | `internal/dashboard` + `web/` | Next.js static export, embedded via `go:embed` |

## Usage and quota data flow

```text
[ Claude Code session ] ──writes──► <config dir>/projects/<encoded-cwd>/<uuid>.jsonl
                                         │
                                         │ new v0.2 profiles link this path to:
                                         ▼
                              ~/.ccx/shared-projects/<encoded-cwd>/<uuid>.jsonl
                                         │
                                         ▼
                            internal/scanner + sharedscan
                                         │
                                         ▼
                         internal/storage (SQLite, ~/.ccx/state.db)
                                         │
                  ┌──────────────────────┴──────────────────────┐
                  ▼                                             ▼
       internal/cli (ccx usage --quota)       internal/server (/api/usage, /api/quota)
                                                                │
                                                                ▼
                                                     web/ dashboard quota panel
```

Token and cost rows still come from JSONL files. Plan-aware turn counts come
from local hook telemetry: each completed Claude turn records a `Stop` hook
event, and the quota query counts those stops inside the active 5-hour and
weekly windows for the profile's configured plan tier.

## Daemon, hooks, and recommendations flow

```text
ccx daemon start
      │
      ▼
~/.ccx/daemon.pid + daemon.json + daemon.log
      │
      ├── watches registered profile project dirs and ~/.ccx/shared-projects/
      ├── ingests JSONL usage and hook-backed turn counts
      ├── evaluates quota pressure transitions per profile
      └── serves the local dashboard API on 127.0.0.1

ccx hooks install
      │
      ▼
<profile config dir>/settings.json
      │
      ▼
Claude Code hook payloads ──► ccx hooks record --profile <name>
      │                                   │
      │                                   ▼
      └────────────────────► hook_events + sessions tables in ~/.ccx/state.db

ccx suggest
      │
      ├── scans registered profiles for fresh usage
      ├── reads profile limits from ~/.ccx/profiles.toml
      ├── reads usage, quota windows, recent StopFailure hooks, and profile health from SQLite
      ▼
advisory ranking + suggested ccx use command

GET /api/recommendations/live
      │
      ▼
daemon-sent pressure events for dashboard banners and ccx run --supervise
```

The headroom path is advisory by default. It does not proxy requests or move
credentials. `ccx run` can opt into acting on recommendations before launch, and
`ccx run --supervise` can opt into a mid-session restart after a completed turn.

## Run and shared-history flow

```text
ccx profile add work --config-dir ~/.claude-profiles/work
      │
      └── ensures <config dir>/projects -> ~/.ccx/shared-projects

existing profile
      │
      └── ccx migrate-shared-history --dry-run
          ccx migrate-shared-history

ccx run --supervise -- claude
      │
      ├── launches claude with selected CLAUDE_CONFIG_DIR
      ├── observes local Stop hooks for the active session id
      ├── listens to daemon /api/recommendations/live when available
      └── on a hard-pressure recommendation:
             terminate current claude process
             relaunch healthier profile with claude --resume <session-id>
```

Shared history keeps Claude's JSONL session files visible from every managed
profile, which is what lets `claude --resume <session-id>` continue the same
conversation after a supervised profile swap.

## Why these choices

| Choice | Rationale |
| --- | --- |
| Go | Single binary, easy cross-compile, mature CLI ecosystem |
| `modernc.org/sqlite` (pure Go) | No CGo → clean Windows cross-compilation |
| TOML for registry | Human-editable; recoverable if corrupted |
| `go:embed` for dashboard | One binary install — no separate npm step for users |
| HTTP server on 127.0.0.1 only | Local-only data; zero exposure surface |
| `eval`-style profile switching | Same pattern as nvm, pyenv — proven UX |

## Threat model

ccx is a local tool. Its threat model is small:

- The dashboard binds to `127.0.0.1` only — never `0.0.0.0`
- No outbound network calls (except `claude` itself, which ccx doesn't initiate)
- No telemetry
- ccx never reads credential contents; on macOS the OS Keychain holds them, on
  Linux/Windows the file lives inside `CLAUDE_CONFIG_DIR` and ccx never opens it

See [`SECURITY.md`](../SECURITY.md) for disclosure policy.
