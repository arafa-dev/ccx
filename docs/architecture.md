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
| Parsing | `internal/scanner` | JSONL streaming parser with incremental cursors |
| Profile mgmt | `internal/profile` | TOML registry at `~/.ccx/profiles.toml` |
| Pricing | `internal/pricing` | Embedded model→USD rate table |
| Shell | `internal/shell` | Snippet generators for zsh/bash/fish/pwsh |
| Platform | `internal/platform` | OS detection, default config dir resolution |
| CLI | `internal/cli` | cobra command tree |
| Server | `internal/server` | chi-routed HTTP API for the dashboard |
| TUI | `internal/tui` | bubbletea profile picker |
| Doctor | `internal/doctor` | Diagnostic checks |
| Dashboard | `internal/dashboard` + `web/` | Next.js static export, embedded via `go:embed` |

## Data flow

```text
[ Claude Code session ] ──writes──► ~/.claude*/projects/<encoded-cwd>/<uuid>.jsonl
                                              │
                                              ▼
                                  internal/scanner (fsnotify in dashboard mode)
                                              │
                                              ▼
                                  internal/storage (SQLite, ~/.ccx/state.db)
                                              │
                              ┌───────────────┴───────────────┐
                              ▼                               ▼
                  internal/cli (ccx usage)        internal/server (/api/usage)
                                                              │
                                                              ▼
                                                       web/ dashboard (browser, dark mode)
```

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
