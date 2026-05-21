# ccx — Handoff Document

**Generated:** 2026-05-20
**Purpose:** Bring a fresh AI context (or human teammate) up to speed in <5 minutes so the next phase of work can
start without re-deriving anything.

---

## TL;DR

**ccx** is a Go CLI + embedded Next.js dashboard for managing multiple Claude Code accounts. It switches between
accounts (via `CLAUDE_CONFIG_DIR` export) and tracks per-profile usage from JSONL session files. Single binary,
cross-platform (macOS + Linux + Windows), distributed via brew/scoop/apt/curl|sh.

**Status:** Spec + 12 implementation plans are written. Phase 0 (contracts/schema/CI) is executed and on `main`.
Phase 1 (9 parallel package implementations) is **ready to dispatch** but has not started.

**Repo:** [github.com/arafa-dev/ccx](https://github.com/arafa-dev/ccx) — currently at commit `84e3e44`.

---

## Where to start reading (in order)

1. **Spec** — [`docs/superpowers/specs/2026-05-19-ccx-design.md`](docs/superpowers/specs/2026-05-19-ccx-design.md)
2. **Plan index** — [`docs/superpowers/plans/2026-05-19-ccx-plan-index.md`](docs/superpowers/plans/2026-05-19-ccx-plan-index.md)
3. **This file** — answers "where are we / what's next"

Everything else is detail.

---

## Phase 0 — DONE

Executed by Codex. On `main`. Includes:

- `internal/contracts/` — shared types and interfaces
- `internal/storage/schema.sql` — locked SQLite schema
- `api/openapi.yaml` — HTTP API contract
- `docs/conventions.md` — error wrapping, logging, exit codes, lint rules
- `Makefile`, `.gitignore`, `.golangci.yml`, `lefthook.yml`
- `LICENSE`, `SECURITY.md`, `CONTRIBUTING.md`
- `.github/workflows/ci.yml` (lint + test on macOS/Linux/Windows)
- `.github/dependabot.yml`, PR template, CODEOWNERS
- Empty stub packages (`internal/{profile,scanner,...}/doc.go`)

**Codex made 3 small refinements vs the original Phase 0 plan** (all on `main` already):

1. `Store.InsertEvents` now takes `profileName` explicitly: `InsertEvents(ctx, profileName, events)`
2. `UsageRow` carries JSON tags matching `api/openapi.yaml`
3. Shell constants got doc comments

These changes are reflected in A3's amendment block and in P2's wiring code.

---

## Phase 1 — READY TO DISPATCH (9 parallel plans)

Each plan is a self-contained, bite-sized-task implementation guide with exact code in every step. Each runs in its
own git worktree.

| Plan | Plan path | Worktree | Package |
|---|---|---|---|
| A1 | [`...phase-1-A1-profile.md`](docs/superpowers/plans/2026-05-19-ccx-phase-1-A1-profile.md) | `feat/profile` | `internal/profile/` |
| A2 | [`...phase-1-A2-scanner.md`](docs/superpowers/plans/2026-05-19-ccx-phase-1-A2-scanner.md) | `feat/scanner` | `internal/scanner/` |
| A3 | [`...phase-1-A3-storage.md`](docs/superpowers/plans/2026-05-19-ccx-phase-1-A3-storage.md) | `feat/storage` | `internal/storage/` ⚠️ has amendment block |
| A4 | [`...phase-1-A4-pricing.md`](docs/superpowers/plans/2026-05-19-ccx-phase-1-A4-pricing.md) | `feat/pricing` | `internal/pricing/` |
| A5 | [`...phase-1-A5-shell.md`](docs/superpowers/plans/2026-05-19-ccx-phase-1-A5-shell.md) | `feat/shell` | `internal/shell/` |
| A6 | [`...phase-1-A6-platform.md`](docs/superpowers/plans/2026-05-19-ccx-phase-1-A6-platform.md) | `feat/platform` | `internal/platform/` |
| A7 | [`...phase-1-A7-web.md`](docs/superpowers/plans/2026-05-19-ccx-phase-1-A7-web.md) | `feat/web` | `web/` (Next.js) |
| A8 | [`...phase-1-A8-distribution.md`](docs/superpowers/plans/2026-05-19-ccx-phase-1-A8-distribution.md) | `feat/distribution` | `.goreleaser.yaml`, install.sh, taps |
| A9 | [`...phase-1-A9-docs.md`](docs/superpowers/plans/2026-05-19-ccx-phase-1-A9-docs.md) | `feat/docs` | `README.md`, `docs/`, GIFs |

### Recommended dispatch pattern (Codex)

```bash
# Create all 9 worktrees in one go
for p in profile scanner storage pricing shell platform web distribution docs; do
  git worktree add ../ccx-$p -b feat/$p main
done
```

Then dispatch one Codex agent per worktree with a prompt template like:

> Read `docs/superpowers/plans/2026-05-19-ccx-phase-1-A{N}-{name}.md` and execute every task in order.
> Bite-sized step pattern: failing test → run-fail → impl → run-pass → commit. One commit per task. Do not modify
> files outside your assigned package. Open a PR against `main` when all tasks are done.

### Critical rules for Phase 1 agents

1. **Never edit `internal/contracts/`, `api/openapi.yaml`, `internal/storage/schema.sql`, or `docs/conventions.md`**
   from a Phase 1 worktree. If a contract needs to change, pause that worktree, open a separate contract-amendment
   PR against `main`, merge it, then rebase the worktree.
2. **Never import from a sibling `internal/*` package.** Only `internal/contracts` and stdlib + the dependencies
   declared in your plan.
3. **One commit per task.** Conventional commits format: `type(scope): subject`.

### After Phase 1 merges → Phase 2 (Integration)

- Plan: [`docs/superpowers/plans/2026-05-19-ccx-phase-2-integration.md`](docs/superpowers/plans/2026-05-19-ccx-phase-2-integration.md)
- One Codex agent, ~1-2 weeks
- Wires `internal/cli`, `internal/server`, `internal/tui`, `internal/doctor`, `internal/dashboard` + adds
  `cmd/ccx/main.go` + end-to-end integration tests

### After Phase 2 → Phase 3 (Polish & launch)

- Plan: [`docs/superpowers/plans/2026-05-19-ccx-phase-3-polish-launch.md`](docs/superpowers/plans/2026-05-19-ccx-phase-3-polish-launch.md)
- Mostly human: friend-test, fix top friction, record GIFs, tag v0.1.0, Show HN

---

## Known issues / amendments (must know)

| Issue | Where | Status |
|---|---|---|
| A3's plan body uses `InsertEventsForProfile`; the real interface is `InsertEvents(ctx, profileName, events)` | A3 plan top has an `⚠️ Amendment` block | Patched in `84e3e44` |
| P2's wiring originally called `profile.NewManager(filepath.Join(home, "profiles.toml"))` but A1 takes `NewManager(home)` (root dir, not file path) | P2 plan | Patched in `84e3e44` |
| P2's wiring originally called `scanner.New(...StoreCursorAdapter(...))` but A2 defines `NewScanner(CursorStore)` and ships no `StoreCursorAdapter` | P2 plan now includes a `storeCursorAdapter` defined inside P2 itself | Patched in `84e3e44` |
| A7 (web, 3,048 lines) not deep-reviewed | A7 plan | Worth a skim before dispatching its Codex agent |
| A8 distribution config not validated against goreleaser schema | A8 plan | Agent should run `goreleaser check` early to catch any drift |

---

## Key decisions made (don't re-litigate)

These were settled during brainstorming. If the next session asks "should we do X instead?", here's why we picked
what we picked:

| Decision | Why |
|---|---|
| **Approach B** (CLI + dashboard in v0.1, 8-10 weeks) chosen over Approach A (CLI only, 4-5 weeks) | User chose to include dashboard, scoop, apt/deb, TUI picker. Trade-off: longer timeline, applies to FAANG ~6 weeks later than originally planned, but stronger CV piece. |
| **Build with AI → study before applying** | User has no Go experience. Will study the codebase after v0.1 ships, before starting FAANG interview loops. |
| **Codex multi-agent in parallel worktrees** | User's chosen execution model. The architecture is decomposed for this (contracts-first, then 9 isolated packages, then integration). |
| **Project name `ccx`** (not `claudectl`) | Anthropic has forced renames before (Clawdbot → OpenClaw). `ccx` has no "claude" string → safer. |
| **Drop v2 "routing prompts" pitch** | Feb 2026 Anthropic ToS bans third-party tools from routing requests through Pro/Max OAuth credentials. Reframed as v0.4 "advisory" feature only — never proxies. |
| **`modernc.org/sqlite`** (pure Go) over `mattn/go-sqlite3` (CGo) | Clean Windows cross-compilation; no CGo toolchain needed in CI. |
| **TOML for profile registry** | Human-editable; recoverable if corrupted. |
| **Embed Next.js via `go:embed`** | Single-binary install story is core to differentiation vs ccs (TS+Tauri) and ccusage (npm). |
| **Bind dashboard to `127.0.0.1` only** | Local data, no exposure surface. |

---

## How to verify state when you resume

```bash
cd /Users/arafa/Developer/ccx
git status                                # working tree should be clean
git log --oneline | head -5               # latest commit should be 84e3e44 or later
ls docs/superpowers/plans/                # should list 13 .md files
ls internal/                              # should list 12 package dirs

# Verify Phase 0 builds and tests still pass:
go build ./...
go test -race -count=1 ./...
golangci-lint run

# Verify the active interface (the one A3 was patched to match):
grep "InsertEvents" internal/contracts/interfaces.go
# Expected: InsertEvents(ctx context.Context, profileName string, events []Event) error
```

---

## Important context for the next session

- The user's working directory is `/Users/arafa/Developer/ccx` (macOS). `~/.claude` already exists and is their
  personal account.
- The user is running multi-agent execution via **Codex CLI**, not Claude Code, for Phase 1 onward. Plans are written
  to be portable (no Claude-Code-specific assumptions).
- Branch protection is enabled on `main` (requires PR + 4 CI checks), but the user has admin override. They've used
  direct pushes for plan docs; **Phase 1 code work should go through PRs** to keep the activity graph healthy for CV
  signal.
- The user previously hit a usage limit while I was dispatching parallel subagents to write A1–A9. A1–A8 made it to
  disk; A9 was written by me directly afterward. P2, P3, the plan index, and all patches were also written by me
  directly. Treat A1–A8 with appropriate skepticism — they were subagent-authored.
- ~~Anything about the user's role/preferences worth knowing~~ — see `MEMORY.md` if any was saved (this conversation
  didn't add anything).

---

## Suggested next prompt for fresh context

> Read `HANDOFF.md` and `docs/superpowers/plans/2026-05-19-ccx-plan-index.md`. Then help me dispatch Codex agents
> for Phase 1. Start with A4 (pricing) as a small first test of the dispatch workflow before fanning out to the
> larger plans.

(Why A4 first: small plan, no cross-package dependencies beyond contracts, easy to verify the agent did the right
thing before committing to dispatching all 9.)

---

## Files committed but not yet pushed

None. `origin/main` is up to date with local `main` as of this handoff.
