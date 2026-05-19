# ccx Implementation Plan Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement each plan task-by-task.

**Spec:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md)

**Goal:** Build and launch `ccx` v0.1.0 — a Go CLI + embedded Next.js dashboard for managing multiple Claude Code accounts, with cross-platform distribution.

**Approach:** Decompose the spec into 12 independently-executable plans. One sets up shared contracts (Phase 0). Nine implement isolated packages in parallel git worktrees (Phase 1). One integrates them into a working binary (Phase 2). One polishes and launches (Phase 3).

**Tech Stack:** Go 1.22+, Next.js 15, SQLite (modernc.org/sqlite), bubbletea, cobra, chi, fsnotify, goreleaser, GitHub Actions.

---

## How to use this index

Each row below is a plan. Plans must be executed in the order dictated by their **depends on** column. Phase 1 plans (A1–A9) can run in parallel — each in its own git worktree off `main`. PRs merge to `main` one at a time.

**Required workflow per Phase 1 worktree:**

1. Create worktree: `git worktree add ../ccx-<package> -b feat/<package>`
2. Dispatch agent against the relevant plan file
3. Agent works to completion (all tests pass in that worktree)
4. Open PR against `main`
5. Review, then merge
6. Other worktrees rebase off updated `main`

**Contract amendment process:** If a Phase 1 agent discovers that a contract needs to change, the worktree pauses, a single "contract amendment" PR is opened against `main` (editing `internal/contracts/`), reviewed, merged, then other worktrees rebase. Direct edits to `internal/contracts/` from a Phase 1 worktree are forbidden.

---

## Plan registry

| ID | Plan | Goal | Depends on | Status | Worktree |
|---|---|---|---|---|---|
| **P0** | [Phase 0 — Contracts](./2026-05-19-ccx-phase-0-contracts.md) | Lock down types, interfaces, schema, openapi, conventions, CI skeleton | — | **Ready** | `main` |
| **A1** | Phase 1 — `internal/profile/` | Profile model, TOML registry I/O, validation, CRUD | P0 | *Pending plan* | `feat/profile` |
| **A2** | Phase 1 — `internal/scanner/` | JSONL parser, dir walker, fuzz tests | P0 | *Pending plan* | `feat/scanner` |
| **A3** | Phase 1 — `internal/storage/` | SQLite layer, migrations, scan-cursor logic | P0 | *Pending plan* | `feat/storage` |
| **A4** | Phase 1 — `internal/pricing/` | YAML loader, cost calc by model + date | P0 | *Pending plan* | `feat/pricing` |
| **A5** | Phase 1 — `internal/shell/` | POSIX + PowerShell + fish snippet generators | P0 | *Pending plan* | `feat/shell` |
| **A6** | Phase 1 — `internal/platform/` | OS detection, default config dir resolution | P0 | *Pending plan* | `feat/platform` |
| **A7** | Phase 1 — `web/` (Next.js) | Dashboard frontend, mocks API via MSW | P0 (openapi.yaml only) | *Pending plan* | `feat/web` |
| **A8** | Phase 1 — Distribution | `.goreleaser.yaml`, Homebrew + Scoop taps, install.sh | P0 | *Pending plan* | `feat/distribution` |
| **A9** | Phase 1 — Docs | README, architecture diagram, vhs GIF tape, contributing | P0 | *Pending plan* | `feat/docs` |
| **P2** | Phase 2 — Integration | cli, server, tui, doctor, dashboard glue, e2e tests | A1–A9 merged | *Pending plan* | `main` |
| **P3** | Phase 3 — Polish & launch | Friend-test, fix friction, record GIFs, tag v0.1.0, HN launch | P2 | *Pending plan* | `main` |

---

## Why some plans are deferred

Plans A1–A9, P2, and P3 are not written yet because:

1. **Contract drift risk.** Their task content (types, interfaces, file names) depends on exact decisions locked down in Phase 0. Writing them speculatively means rewriting them after Phase 0 lands.
2. **Token cost.** Each plan is several thousand lines of detailed task content with no placeholders. Writing all 12 up front is wasteful when 11 are downstream of P0.
3. **Just-in-time planning is more accurate.** Each plan is written against the *actual* state of `main` at the time, not an assumed state.

**Trigger to write the next plan:** When P0 is merged to `main`, request the next plan (e.g., "write A1") and the agent will write it against the real Phase 0 output.

---

## Done definition for v0.1.0

The project ships v0.1.0 when:

- All 12 plans completed
- All tests pass on macOS, Linux, Windows (amd64 + arm64 where applicable)
- `brew install arafa-dev/tap/ccx` works end-to-end on a fresh Mac
- `scoop install arafa-dev/ccx` works end-to-end on a fresh Windows
- `apt install ./ccx_*.deb` works on a fresh Ubuntu
- `ccx dashboard` opens a working browser UI showing real data within 5 seconds of install
- README has a hero GIF, install commands above the fold, comparison table, roadmap
- v0.1.0 tag pushed, GitHub Release published with signed binaries
- Show HN post live with link

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Phase 1 agent over-engineers | Each plan opens with "YAGNI / no abstractions beyond what tests need" |
| Contract change requested mid-Phase-1 | Single serial contract-amendment PR; other worktrees rebase |
| Frontend (web/) and backend (server/) drift | `api/openapi.yaml` is the source of truth; backend implements it, frontend mocks it via MSW |
| Phase 1 takes longer than 3-4 weeks | Phase 2 + 3 have slack baked in; dashboard is the first cut if timeline slips |
| Agent commits secrets or broken code | Pre-commit hooks (lefthook + gofumpt + golangci-lint) installed in Phase 0 |
| User burnout | Each phase has a clear, shippable artifact — Phase 0 alone is a green CI badge worth celebrating |
