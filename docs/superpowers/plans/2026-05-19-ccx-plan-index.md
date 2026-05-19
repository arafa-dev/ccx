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
| **P0** | [Phase 0 — Contracts](./2026-05-19-ccx-phase-0-contracts.md) | Lock down types, interfaces, schema, openapi, conventions, CI skeleton | — | ✅ **Executed by Codex** | `main` |
| **A1** | [Phase 1 — `internal/profile/`](./2026-05-19-ccx-phase-1-A1-profile.md) | Profile model, TOML registry I/O, validation, CRUD | P0 | Ready | `feat/profile` |
| **A2** | [Phase 1 — `internal/scanner/`](./2026-05-19-ccx-phase-1-A2-scanner.md) | JSONL parser, dir walker, fuzz tests | P0 | Ready | `feat/scanner` |
| **A3** | [Phase 1 — `internal/storage/`](./2026-05-19-ccx-phase-1-A3-storage.md) | SQLite layer, migrations, scan-cursor logic | P0 | Ready | `feat/storage` |
| **A4** | [Phase 1 — `internal/pricing/`](./2026-05-19-ccx-phase-1-A4-pricing.md) | YAML loader, cost calc by model + date | P0 | Ready | `feat/pricing` |
| **A5** | [Phase 1 — `internal/shell/`](./2026-05-19-ccx-phase-1-A5-shell.md) | POSIX + PowerShell + fish snippet generators | P0 | Ready | `feat/shell` |
| **A6** | [Phase 1 — `internal/platform/`](./2026-05-19-ccx-phase-1-A6-platform.md) | OS detection, default config dir resolution | P0 | Ready | `feat/platform` |
| **A7** | [Phase 1 — `web/` (Next.js)](./2026-05-19-ccx-phase-1-A7-web.md) | Dashboard frontend, mocks API via MSW | P0 (openapi.yaml only) | Ready | `feat/web` |
| **A8** | [Phase 1 — Distribution](./2026-05-19-ccx-phase-1-A8-distribution.md) | `.goreleaser.yaml`, Homebrew + Scoop taps, install.sh | P0 | Ready | `feat/distribution` |
| **A9** | [Phase 1 — Docs](./2026-05-19-ccx-phase-1-A9-docs.md) | README, architecture diagram, vhs GIF tape, contributing | P0 | Ready | `feat/docs` |
| **P2** | [Phase 2 — Integration](./2026-05-19-ccx-phase-2-integration.md) | cli, server, tui, doctor, dashboard glue, e2e tests | A1–A9 merged | Ready | `main` (or `feat/integration`) |
| **P3** | [Phase 3 — Polish & launch](./2026-05-19-ccx-phase-3-polish-launch.md) | Friend-test, fix friction, record GIFs, tag v0.1.0, HN launch | P2 | Ready | `main` |

---

## Plan status

All 12 plans are written and the project is ready for Codex agent handoff. Phase 0 has been executed by Codex; the live contracts in `internal/contracts/` are the source of truth for any plan content that disagrees (a couple of small drifts: `Store.InsertEvents` now takes `profileName` explicitly; `UsageRow` carries JSON tags). Worktrees can be created and Phase 1 agents dispatched in parallel against `main`.

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
