<!-- markdownlint-disable MD013 MD040 MD049 -->
<!-- cspell:ignore errorlint predev regen -->

# CLAUDE.md

Guidance for **Claude Code** (and other Claude-based agents) working on this
repo. This file is loaded automatically into Claude Code's context — keep it
tight.

> **Read [`AGENTS.md`](AGENTS.md) first.** It is the cross-tool source of truth
> for repo layout, conventions, commands, frozen files, and workflow. This
> file only adds Claude-specific guidance and shortcuts. Where the two
> overlap, AGENTS.md wins.

---

## 1. Orientation — read these in order

When you start cold on a task, before writing any code:

1. **`git log --oneline -20 origin/main`** — the repo moves fast. PR #16 (daemon, hooks, headroom) landed a ~13.5k-line change. Don't trust pre-#16 docs blindly.
2. [`AGENTS.md`](AGENTS.md) — repo map, hard rules, commands, conventions, the daemon/hooks/headroom big picture.
3. [`docs/architecture.md`](docs/architecture.md) — current component map and the three data-flow diagrams (usage, daemon, hooks+headroom).
4. [`docs/conventions.md`](docs/conventions.md) — only if the task touches style, errors, logging, exit codes, or tests.
5. [`HANDOFF.md`](HANDOFF.md) — historical phase snapshot. Useful for "why is the contract shaped this way," **stale** on daemon / hooks / headroom. Cross-check against the code, not the doc.

Don't re-derive what's already documented. Don't summarize these docs back to the user — act on them.

---

## 2. Project at a glance

- **What:** Go 1.24 CLI + optional background daemon + embedded Next.js 15 dashboard. Single binary. Cross-platform (macOS, Linux, Windows).
- **Purpose:** Switch between Claude Code accounts (`CLAUDE_CONFIG_DIR`), surface per-profile usage, capture local session telemetry from Claude Code hooks, and emit advisory profile recommendations.
- **Module path:** `github.com/arafa-dev/ccx`.
- **Binary:** `cmd/ccx` → `dist/ccx` (via `make build`).
- **Dashboard:** `web/` (Next.js static export) embedded into `internal/dashboard/web-out/` via `go:embed`.
- **State (under `~/.ccx/`):** `profiles.toml`, `state.db` (SQLite), optional `pricing.yaml`, and — when the daemon runs — `daemon.{pid,json,log,lock}`.

### Command groups you should know exist

`profile`, `use`, `init`, `usage`, `suggest`, `dashboard`, `daemon` (start/status/stop/restart/logs), `hooks` (install/status/uninstall/record), `doctor`, `version`.

### HTTP API the dashboard consumes (served by `ccx dashboard` *or* `ccx daemon`)

`/api/health`, `/api/profiles`, `/api/usage`, `/api/usage/live` (SSE), `/api/daemon/status`, `/api/hooks/status`, `/api/sessions`, `/api/headroom`. All on `127.0.0.1`.

---

## 3. The six rules you will be tempted to break

1. **Do not edit `internal/contracts/`, `internal/storage/schema.sql`, `api/openapi.yaml`, or `docs/conventions.md`** from a feature branch. They are contracts. If they need to change, open a separate amendment PR against `main` first. (Schema v2 additions live in `internal/storage/migrate.go` and were added with the contract amendment, not by editing v1.)
2. **Leaf `internal/*` packages do not import siblings.** Integration glue lives in `internal/cli/`, `internal/server/`, and `internal/daemon/` — those are *allowed* to fan in. Don't blur that line.
3. **All HTTP servers bind `127.0.0.1` only.** Both `ccx dashboard` and `ccx daemon` serve on loopback. Not `0.0.0.0`. Not "behind auth." No exceptions.
4. **"Telemetry" here is *local* SQLite, never network.** ccx does not phone home. The hook-telemetry path writes to `~/.ccx/state.db` only. No outbound HTTP at runtime.
5. **No proxying Anthropic traffic.** ccx switches env vars and reads local JSONL/hook payloads. It does not see, relay, or rewrite API calls.
6. **No CGo.** SQLite is `modernc.org/sqlite` (pure Go) for cross-compile reasons. Don't swap it.

---

## 4. The task loop Claude should run

For each discrete unit of work:

```
1. Write or update a failing test that pins the new behavior.
2. Run that test, confirm it fails for the right reason.
3. Make the smallest change that turns it green.
4. Re-run package tests + `golangci-lint run ./internal/<pkg>/...`.
5. Stage exactly the files touched and commit:  type(scope): subject
```

One commit per logical change. One package per PR. Before pushing: `make ci`.

### When the change touches the dashboard end-to-end

```bash
cd web && pnpm build && cd ..
make stage-web          # copies web/out → internal/dashboard/web-out
make build              # bakes a fresh dist/ccx with the new assets
./dist/ccx dashboard --no-open
curl -s http://127.0.0.1:7777/api/health
```

### When the change touches the API contract

```bash
# 1. Edit api/openapi.yaml on main via a contract-amendment PR.
# 2. Then in your feature branch:
cd web && pnpm gen:api && pnpm check:api-types && cd ..
# 3. Update the Go handlers in internal/server/handlers.go to match.
```

### When the change touches the daemon

The daemon's start path involves a lock handoff between parent and child (env vars `CCX_DAEMON_LOCK_TOKEN`, `CCX_DAEMON_LOCK_PARENT_PID`, `CCX_DAEMON_LOCK_HELD_BY_PARENT`). Read `internal/daemon/runtime.go` and `internal/daemon/lock.go` before changing anything in there. The lock identity check (`platform.ProcessIdentity`) is what makes stale-pid recovery safe; do not weaken it.

### When the change touches hooks

Hooks edit user files. Every install/uninstall must:

- Write a `settings.json.ccx-backup-<timestamp>` *before* mutating.
- Respect `disableAllHooks: true` (status = `disabled`; install is a no-op unless `--force`).
- Only modify ccx-managed entries; never blow away unrelated user hooks.

---

## 5. Tool-use guidance specific to this repo

- **Always check `git log` against `origin/main` before writing code.** The repo had a 13.5k-line PR (#16) recently; in-flight branches can lag behind quickly. If the user mentions "the new PR" or "telemetry / daemon / headroom," that's #16.
- **Use `Read` over `cat`**, `Edit` over `sed`, `Write` only for new files.
- **Use `Grep` / `Glob`** for code search; the repo spans Go + TS + YAML + SQL.
- **Use `Bash` for:** `make` targets, `go test ./...`, `pnpm` commands, `git` operations, `goreleaser check`.
- **Don't create scratch markdown files** (`PLAN.md`, `NOTES.md`, etc.) in the repo root. Use TaskCreate for in-conversation tracking. `HANDOFF.md` and `docs/superpowers/plans/` are the only authorized planning artifacts (and `HANDOFF.md` is currently stale — don't extend it casually).
- **When the user asks you to "test the dashboard,"** that means: build the binary with staged web assets, run `ccx dashboard --no-open` (or `ccx daemon start`), `curl http://127.0.0.1:7777/api/health` plus the route under test, and inspect the response. Not just `pnpm dev`.

---

## 6. Common gotchas observed in this codebase

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `go build` complains about `internal/dashboard/web-out` being empty | You skipped `make stage-web` | `cd web && pnpm build && cd .. && make stage-web` |
| Lint fails on "exported X should have comment" | `revive` exported rule | Add a doc comment starting with the symbol name |
| Lint fails on `errorlint` | Used `==` to compare errors | Use `errors.Is` and `%w` in wraps |
| Tests pass locally but CI fails with `gofumpt` diff | Default `gofmt` ≠ `gofumpt` | `make fmt` (or `gofumpt -w .`) |
| `pnpm dev` 404s on API calls | MSW worker not initialized | `pnpm predev` runs automatically; if you bypassed it, `pnpm msw init public/ --no-save` |
| `pnpm build` fails offline | `next/font/google` needs network | Either give CI network or vendor the font |
| `web/lib/api-types.ts` mismatches `api/openapi.yaml` | Forgot regen | `cd web && pnpm gen:api` |
| Dashboard 404s on a known API route under embedded server | Route only mounted in mocks | Check `internal/server/server.go` `routes()` first |
| `ccx daemon start` exits immediately with "lock held" | Stale lock from a previous run, but with a live-looking pid | `ccx daemon status` to inspect; `ccx daemon stop` or, last resort, remove `~/.ccx/daemon.{lock,pid,json}` and retry |
| Sessions panel in the dashboard is empty | Hooks not installed, or `settings.json` has `disableAllHooks: true` | `ccx hooks status`; if `partial` or `missing`, `ccx hooks install --profile <name>` |
| `ccx suggest` returns no recommendation | Cooldown after a `rate_limit` StopFailure, or `suggest_enabled=false`, or auth health = `authentication_failed` | `ccx suggest --json` to see per-candidate reasons; covered in `docs/troubleshooting.md` |

---

## 7. Communicating with the user

- They are developing ccx on macOS. They are new to Go but experienced overall.
- Multi-agent execution is happening: Codex CLI ran most of Phase 1, ChatGPT/Codex authored PR #16, and Claude Code is one of multiple agents that touches this repo. Be careful about assuming your in-flight changes are the only ones — always `git fetch && git log origin/main` before assuming the world.
- Default to short, decisive replies. State what you did and what's next. Don't summarize diffs; the user can read them.
- When you can't verify a behavior end-to-end (e.g., the dashboard rendered correctly in a browser, the daemon survived an OS sleep), **say so explicitly** rather than declare success on type-check / test-pass alone.

---

## 8. When in doubt

- The behavior is already specified somewhere. Check, in order: `internal/contracts/`, `api/openapi.yaml`, the handler in `internal/server/handlers.go` or the relevant `internal/<pkg>/` source, `docs/conventions.md`, `docs/architecture.md`, `docs/troubleshooting.md`.
- If those don't answer it, ask the user — don't invent contract semantics, exit-code mappings, schema fields, or daemon lock invariants.
- If the answer is "we haven't decided yet," propose a minimal answer, mark it as such, and offer the user a chance to redirect before you commit.
