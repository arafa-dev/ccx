# ccx v0.2 — Plan-Aware Quota & Auto-Switching Plan Index

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement each plan task-by-task. Each plan uses checkbox (`- [ ]`) syntax for tracking and lays out one TDD task per commit.

**Spec:** [`docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`](../specs/2026-05-24-ccx-plan-aware-quota.md)

**Goal:** Ship `ccx v0.2.0` — add plan-aware quota tracking, pressure-aware suggest, and progressive auto-switching (pre-launch + mid-session) on top of v0.1's profile management, usage analytics, daemon, hooks, and headroom.

**Approach:** Decompose the spec into **5 sequential plans**. P0 is a contracts amendment landed first on `main` (touches the frozen contract files). B1–B3b are feature plans, each in its own worktree, each merging back to `main` before the next worktree rebases. Each plan ends in one PR.

**Tech Stack:** Go 1.22+ (existing). No new runtime deps for B1/B2/B3a. B3b adds no Go deps but introduces filesystem symlinks. Web side: existing Next.js 15 + MSW; B1 adds one new component, B3a adds a live banner component.

---

## How to use this index

Plans must be executed **in order**. Unlike Phase 1 of v0.1 (which used parallel worktrees for independent leaf packages), v0.2's phases each depend on the previous one's merged state. No parallelism.

**Per-phase workflow:**

1. Wait for previous phase to merge to `main`. Rebase if a worktree already exists.
2. Create worktree off latest `main`:
   ```bash
   git worktree add ../ccx-quota-<PHASE> -b feat/quota-<phase> main
   cd ../ccx-quota-<PHASE>
   ```
3. Open the plan file. Execute tasks in order. **One commit per task.**
4. Run `make ci` before opening the PR.
5. Open PR. Title: `feat(quota-<phase>): <one-line summary>`. Body: link to the plan file.
6. Review, then squash-merge (or rebase-merge — keep linear history). Delete the worktree.
7. Update the **Status** column below.

**If a phase needs a contract change mid-flight:** stop, open a separate contract-amendment PR against `main`, merge it, rebase the phase worktree. Same rule that applied in v0.1. Do **not** edit `internal/contracts/`, `internal/storage/schema.sql`, `api/openapi.yaml`, or `docs/conventions.md` from a phase worktree.

---

## Plan registry

| ID | Plan | Goal | Depends on | Status | Worktree |
|---|---|---|---|---|---|
| **P0** | [Contract amendment](./2026-05-24-ccx-quota-P0-contract-amendment.md) | Add `PlanTier`/`WeeklyAnchor`/`Caps5hTurns`/`CapsWeeklyTurns` to `ProfileLimits`. Add `QuotaWindow`, `ProfileQuota`, `RecommendationEvent`, `RecommendationLevel` types. Add `/api/quota` and `/api/recommendations/live` routes + schemas. Extend `HeadroomCandidate`. Regenerate `web/lib/api-types.ts`. | v0.1 main | ✅ Merged in #17 | `contract/quota-amendment` |
| **B1** | [Analytics](./2026-05-24-ccx-quota-B1-analytics.md) | Implement plan-tier defaults, `headroom.Store.QueryTurnsInWindow`, reset-time math, `GET /api/quota` server handler, `ccx usage --quota` CLI flag, dashboard quota panel component. | P0 | ✅ Merged in #18 | `feat/quota-analytics` |
| **B2** | [Pressure-aware suggest](./2026-05-24-ccx-quota-B2-pressure-suggest.md) | Tiered threshold constants, `headroom.Evaluator` extension (folds quota pressure into HeadroomPercent + Score + Reasons + Available gate), `HeadroomCandidate` quota fields, `ccx suggest` rendering update. | B1 | ✅ Merged in #19 | `feat/quota-pressure-suggest` |
| **B3a** | [Pre-launch fallback](./2026-05-24-ccx-quota-B3a-pre-launch-fallback.md) | `ccx run [-- args]` (fork+wait), `ccx init <shell> --with-claude-wrapper`, daemon-level threshold-crossing detector, `GET /api/recommendations/live` SSE handler, dashboard live banner. | B2 | ✅ Merged in #20 | `feat/quota-pre-launch` |
| **B3b** | [Supervisor + shared history](./2026-05-24-ccx-quota-B3b-supervisor-shared-history.md) | Symlink `<config_dir>/projects/` → `~/.ccx/shared-projects/` at profile add. Scanner refactor (walk shared dir, attribute via `sessions.profile_name`). `ccx migrate-shared-history`. `ccx run --supervise` mode: watch hook event stream, kill+relaunch `claude --resume <session-id>` on threshold trip. | B3a | ✅ Merged in #21 | `feat/quota-supervisor` |

---

## Plan status

All 5 plans are merged. P0 was the only plan that touched frozen files, and it landed first as a separate amendment PR.

**B3b additionally required:** the three verification items in [Spec §11](../specs/2026-05-24-ccx-plan-aware-quota.md#11-verification-required-before-implementation) — Claude Code symlink behavior, `--resume` flag, and hook-payload header coverage. They were validated in [the Task 0 findings note](../notes/2026-05-25-b3b-task0-findings.md) before implementation.

---

## Done definition for v0.2.0

The release ships when:

- All 5 plans (P0, B1, B2, B3a, B3b) merged to `main` with green CI.
- `ccx version` reports `0.2.0`.
- `make ci` green on darwin/linux/windows (amd64 + arm64).
- Manual smoke on a fresh `~/.ccx/`:
  1. `ccx profile add demo --config-dir ~/.claude-profiles/demo --plan-tier max5`
  2. `ccx hooks install --profile demo`
  3. Run a real `claude` session under that profile (one prompt is enough).
  4. `ccx usage --quota` shows the turn count.
  5. `ccx dashboard --no-open` + `curl http://127.0.0.1:7777/api/quota` returns the quota row.
  6. `ccx run --profile demo` launches `claude` successfully (B3a).
  7. With `ccx daemon` running and `ccx run --supervise demo`, force a pressure trip (set `Caps5hTurns: 1` so one turn = hard cap), observe the supervisor logs a swap event after the next `Stop` (B3b).
- README gains a "Plan-aware quota" subsection above "Usage analytics".
- `docs/troubleshooting.md` gains entries: "quota panel is empty", "supervisor didn't swap when I expected", "I changed plan tier and the cap didn't update".
- `docs/architecture.md` updated with the v0.2 data-flow additions: turn counting from `hook_events`, the `/api/recommendations/live` stream, and (for B3b) the shared-history symlink layout.
- v0.2.0 git tag pushed; GitHub Release published with signed binaries via the existing `.goreleaser.yaml`.

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Contract amendment merge race with other in-flight work on `main` | P0 has a tiny diff (~300 LOC, all additive). Land it on a quiet day; other agents rebase. |
| Implementation agent skips Phase B3b's Task 0 verification | The Task 0 step is gated by exit criteria — if any of the three checks fails, the agent must update the plan with the actual finding before proceeding. |
| Threshold constants tuned wrong → noisy warnings or missed switches | Constants live in one file (`internal/headroom/thresholds.go`). Easy to retune in a follow-up PR. No contract change. |
| Symlinking `projects/` breaks an existing user setup | `ccx profile add` does the symlink only for *new* profiles. Existing v0.1 profiles must opt in via `ccx migrate-shared-history`, which has a `--dry-run` flag and prints exactly what it will move. |
| `ccx run` shadows the `claude` binary unexpectedly | We do **not** install a `claude` alias by default. The user must opt in via `ccx init <shell> --with-claude-wrapper`. The plain `ccx run` is the always-explicit form. |
| Daemon SSE recommendation events spam | State machine per profile (`below_warn` → `warn` → `soft` → `hard`); only transitions emit. Smoke test: hammer one profile to 95% pressure, verify exactly one `soft` event fires. |
| Anthropic changes plan caps again mid-cycle | Caps are user-overridable per profile and `internal/headroom/` defaults are a single file. A retune is a 10-line PR. |
| Implementation agent picks wrong unit for turn counting (e.g., counts user-message events from JSONL instead of `Stop` hook events) | Spec §6.1 and §6.2 are explicit. The B1 plan's first task is a test that asserts `count(*) FROM hook_events WHERE event_name='Stop'` is what flows into `/api/quota`. |
| User runs `ccx run` without registered profiles | `ccx run` returns a clear error: "no profiles registered; run `ccx profile add` first". Exit code 1. Test for this. |
| Conversation history loss on first-ever B3b swap (no shared history yet) | B3b's first task ensures `ccx migrate-shared-history` runs before any swap can occur, gated by a runtime check in `ccx run --supervise`. |

---

## Cross-references

- v0.1 plan index: [`2026-05-19-ccx-plan-index.md`](./2026-05-19-ccx-plan-index.md) — for the historical Phase 0/1/2/3 work this builds on.
- v0.1 design spec: [`../specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — for context on what was built.
- v0.2 design spec: [`../specs/2026-05-24-ccx-plan-aware-quota.md`](../specs/2026-05-24-ccx-plan-aware-quota.md) — the rationale behind everything in this index.
- Project agent guides: [`AGENTS.md`](../../../AGENTS.md) and [`CLAUDE.md`](../../../CLAUDE.md) — read first.
