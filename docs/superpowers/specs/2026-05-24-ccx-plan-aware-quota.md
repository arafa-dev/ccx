# ccx — Plan-Aware Quota & Auto-Switching Design Specification

**Status:** Approved for implementation planning
**Date:** 2026-05-24
**Owner:** @arafa-dev
**Target release:** v0.2.0
**Estimated effort:** 80-160 hours across 5-8 weeks at 10-20 hrs/week
**Hard floor:** if weekly hours drop below 10, ship Phase B1 alone and defer the rest. The analytics surface alone is already a meaningful product upgrade.

---

## 1. One-line pitch

> Track *what you're actually paying for* on Pro / Max plans, score profiles by remaining headroom, and (eventually) seamlessly hand the conversation off when one account hits its cap.

## 2. Why this exists

ccx today (v0.1) tracks usage as **tokens** and **estimated USD cost** — the API-pricing view. But most ccx users are not paying per token: they pay a flat $20 / $100 / $200 a month for Claude Pro, Max 5x, or Max 20x, and Anthropic meters their usage on a totally different axis — **prompt count inside a rolling 5-hour window**, plus a **weekly cap**. (See Appendix A for current numbers.)

The result: ccx's existing dashboard tells subscription users a number they don't care about (USD), and is silent on the number they actually care about (turns left until I get rate-limited).

This feature closes that gap, in three layers:

1. **Surface the plan-relevant metrics** (5h turns used / cap, weekly turns used / cap) per profile, in the CLI and dashboard.
2. **Fold pressure into the existing `ccx suggest` recommendation** so the recommender prefers profiles with headroom, not just profiles with auth ok.
3. **Auto-switch at the right boundary** — at launch, between turns, and (in the most ambitious form) mid-session, so a 429 doesn't interrupt the user's flow.

## 3. Scope

### 3.1 In scope for v0.2

CLI changes:
- `ccx profile add` and `ccx profile set` gain `--plan-tier {pro|max5|max20|api}`, `--weekly-anchor`, `--caps-5h-turns`, `--caps-weekly-turns`.
- `ccx usage` gains a `--quota` flag that prints the per-profile 5h / weekly turn windows alongside the token rollup.
- `ccx suggest` output gains quota pressure as a ranking signal and as a Reason string.
- New command `ccx run [--profile <name>] [--quiet|--verbose] [-- <args>]` that wraps `claude`, fork+wait, with pre-launch profile selection.
- `ccx init <shell>` gains `--with-claude-wrapper` to optionally emit a `claude` alias that invokes `ccx run`.
- `ccx run --supervise` (B3b only) is the supervisor mode that watches hook events and relaunches under a different profile on threshold trip.

HTTP API additions (served by `ccx dashboard` and `ccx daemon`):
- `GET /api/quota` — per-profile turn windows.
- `GET /api/recommendations/live` — SSE stream of threshold-crossing events.
- `/api/headroom` `HeadroomCandidate` gains optional `quota_5h` / `quota_weekly` fields.

Dashboard:
- New per-profile quota bars (5h + weekly) with green/yellow/red coloring.
- Live banner when the active profile crosses the warn / soft / hard threshold.

### 3.2 Explicitly out of scope for v0.2

- Querying Anthropic's API for "remaining quota" — they don't expose it, and ccx's no-proxy rule blocks the alternative path (header capture).
- Authoritative cap numbers — we ship community-sourced defaults, the user can override (see §13.4).
- Cross-machine session resumption — Phase B3b restarts `claude` on the *same* machine; we don't try to migrate state across hosts.
- Team-level quota pooling.
- iOS / Android.

### 3.3 Non-goals

- **No proxying.** Reinforces the v0.1 non-goal. ccx still never sees an API request.
- **No phone-home.** Quota math is local to `~/.ccx/state.db`.
- **No automatic credential refresh.** If a profile is auth-failed, ccx still surfaces it via `profile_health` and asks the user to fix; quota logic doesn't paper over auth issues.

## 4. Glossary

| Term | Meaning |
|---|---|
| **Turn** | One assistant response to one user prompt. Detected by Claude Code's `Stop` hook event. Failed turns (`StopFailure`) also count toward the cap **except** when `error` is `authentication_failed` or `oauth_org_not_allowed` (those never reached Anthropic, so they don't burn quota). All other `StopFailure` reasons — `rate_limit`, `billing_error`, `invalid_request`, `model_not_found`, `server_error`, `max_output_tokens`, `unknown` — do count. |
| **5h window** | The rolling 5-hour quota window Anthropic applies to Pro / Max plans. Starts at the first turn after a previous window closes. **We model it as rolling** (count turns whose `ts > now - 5h`), per current docs. |
| **Weekly window** | The 7-day cap Anthropic applies to Max plans (and, as of 2026, Pro in limited form). Anchor is configurable: `"rolling"` (default — trailing 7d) or a weekday name (`"monday"` etc.) for users on calendar-anchored billing weeks. |
| **Plan tier** | One of `pro`, `max5`, `max20`, `api`. `api` opts out of quota tracking entirely (token cost view is enough). |
| **Pressure** | A profile's quota-window usage as a percentage of its cap, taken as `max(pct_5h, pct_weekly)`. |
| **Threshold** | A pressure cutoff that changes behavior. v0.2 ships three: `warn=75%`, `soft=90%`, `hard=100%`. |
| **Headroom** | Inverse of pressure (`100 - pressure`). Already the unit `ccx suggest` uses; we extend its derivation. |
| **Supervisor** | A `ccx run` parent process that watches the live `claude` child via hook events and can kill+relaunch under a different profile between turns. |
| **Shared history** | The architecture, introduced in Phase B3b, where every profile's `<config_dir>/projects/` symlinks to one shared directory so a swap doesn't lose conversation state. |

## 5. Architecture

### 5.1 Where the data comes from

```
                                      ┌──────────────────────────────┐
                                      │   Claude Code hook system    │
                                      │  (PR #16 installed in v0.1)  │
                                      └──────────┬───────────────────┘
                                                 │
       SessionStart / Stop / StopFailure / SessionEnd / Pre+PostCompact
                                                 │
                                                 ▼
                                ┌───────────────────────────────────┐
                                │   `ccx hooks record --profile X`  │
                                │  (writes one row per hook event)  │
                                └───────────┬───────────────────────┘
                                            │
                                            ▼
                          ~/.ccx/state.db (hook_events, sessions, profile_health)

                                            │
                                            ▼
                ┌─────────────────────────────────────────────────────────┐
                │  NEW IN v0.2:  turn = hook_events.event_name='Stop'     │
                │                       OR (event_name='StopFailure'     │
                │                           AND reason != 'authentication_failed') │
                │                                                         │
                │  rolling 5h count =                                      │
                │     SELECT count(*) FROM hook_events                     │
                │     WHERE profile_name = ?                               │
                │       AND ts BETWEEN ? AND ?                             │
                │       AND <turn predicate above>                         │
                └─────────────────────────────────────────────────────────┘
```

**Key insight:** turn counting requires *no new tables*. The hook telemetry path landed in v0.1 (PR #16) already records every assistant Stop, with `event_name`, `ts`, `reason`, and `error` columns. v0.2 only adds queries against those existing rows.

### 5.2 Two new HTTP endpoints

```
GET /api/quota                    →  v0.2  per-profile turn windows + caps + reset times
GET /api/recommendations/live    →  v0.2  SSE stream of threshold-crossing events

(Existing endpoints unchanged in shape; /api/headroom gains optional fields.)
```

### 5.3 New CLI surface

```
ccx run                  v0.2 (B3a)  pre-launch profile picker + child wrapper
ccx run --supervise      v0.2 (B3b)  same, plus mid-session swap on threshold trip
ccx init --with-claude-wrapper       v0.2 (B3a)  rc-file snippet that aliases claude→ccx run
```

### 5.4 Phase-by-phase data flow

```
P0   contract amendment      → contracts + openapi + regenerate api-types
B1   /api/quota + panel       ↓
B2   pressure in evaluator    │  each phase depends on the previous
B3a  ccx run + SSE recs       │
B3b  supervisor + shared hx   ↓
```

Each phase is independently shippable and independently useful.

## 6. Design decisions (the load-bearing ones)

### 6.1 Count turns, not tokens

**Decision:** plan-quota tracking is denominated in turns (assistant `Stop` events), not tokens.

**Rationale:**
- Anthropic's published subscription caps are prompt/turn-denominated; tokens are the API-pricing axis.
- Using token counts to estimate turns is lossy and would change the reported "remaining quota" every time the model gets chattier or terser.
- The existing `events` table (token-denominated, for cost) remains the source of truth for the USD view. We keep both, side by side.

**Trade-off:** a user who burns many tokens on a single long turn still counts as 1 turn against quota. That matches how Anthropic meters them.

### 6.2 Hook events are the canonical source

**Decision:** turn counting reads from `hook_events`, not from JSONL.

**Rationale:**
- The `Stop` and `StopFailure` events fire *exactly once per turn*, with a clean timestamp. JSONL has multiple events per turn (one per streamed message), making turn-boundary inference fragile.
- Hook events are written live by `ccx hooks record`, so quota counts update in real time without requiring a JSONL re-scan.
- Already battle-tested via the `/api/sessions` and headroom failure-gate paths.

**Trade-off:** profiles without ccx hooks installed have no quota data. Mitigation: dashboard surfaces hook-installed status per profile (already exists); the empty quota panel says "install hooks to see quota usage".

### 6.3 No `anthropic-ratelimit-*` header capture

**Decision:** we count locally; we do not opportunistically read Anthropic rate-limit headers from any payload.

**Rationale:**
- ccx's no-proxy rule means we never see API responses.
- The hook payload (verified against `internal/hooks/record.go`) exposes `reason`, `error`, `error_details`, `trigger` — no header data.
- A future Claude Code release *could* surface those headers in hook payloads. If it does, we extend the spec then. For v0.2 we ship without.

**Trade-off:** ccx's view of remaining quota is an estimate that depends on the user declaring the right `caps_5h_turns` / `caps_weekly_turns` for their plan. We ship sensible defaults but the truth source is the user's plan-tier declaration. Anthropic's view may differ if other tools (claude.ai web, Claude Code on another machine) also charged against the same plan.

### 6.4 Tiered thresholds (75 / 90 / 100)

**Decision:** three behavioral thresholds, hard-coded as constants in `internal/headroom/`:

| Pressure | Behavior |
|---|---|
| `< 75%` | No signal. |
| `75% ≤ p < 90%` | **warn:** Reason string surfaced in `ccx suggest` output; dashboard bar turns yellow; `/api/recommendations/live` emits a `warn` event the first time this pressure level is crossed in a session. No score impact. |
| `90% ≤ p < 100%` | **soft:** linear score penalty in evaluator (`penalty = (pct − 90) × 2`, capped at 20); dashboard bar turns orange/red; live event emitted. |
| `p ≥ 100%` | **hard:** profile is excluded from recommendations (`Available = false`) with `CooldownUntil` = next window reset; dashboard bar is fully red and locked; live event emitted. |

**Rationale:**
- Single threshold isn't enough — users want a warning before they hit hard cap.
- Per-profile thresholds add knobs we don't need yet. They can be added as ProfileLimits fields in a future amendment if real users ask.
- Linear soft penalty (rather than a step function) makes the recommender's preference smooth as pressure increases — at 95% pressure a profile is still picked over one at 99%, all else equal.

**Trade-off:** the threshold values are taste calls. If they're wrong, easy to retune (just constants).

### 6.5 Auto-switch granularity (the big one)

**Decision:** v0.2 ships *three* progressively more capable forms of auto-switching, gated by phase:

| Phase | Form | What it does |
|---|---|---|
| **B2** | **Pressure-aware advisory** | `ccx suggest` and `/api/headroom` rank by pressure. User still types `ccx use <name>` themselves. Soft. |
| **B3a** | **Pre-launch hard fallback** | `ccx run` picks the best-headroom profile at launch. Session starts under the right account. Once running, no swap. |
| **B3b** | **Mid-session swap** | `ccx run --supervise` watches hook events, kills `claude` between turns when threshold trips, relaunches under sibling profile with `claude --resume <session-id>`. Conversation continues seamlessly *if* history is shared (§6.6). |

**Rationale:**
- Forms are independent: a user can stop at B3a and still get most of the value.
- Each form fails safely: if the supervisor crashes, the user still has B3a's pick; if B3a errors, the user can still `ccx suggest && ccx use`; if all of this is offline, the existing v0.1 manual switch path works.

**Trade-off:** B3b is materially more complex (process supervision, symlinks, scanner refactor — see §6.6).

### 6.6 Shared history via symlink (B3b only)

**Decision:** for B3b, every managed profile's `<CLAUDE_CONFIG_DIR>/projects/` becomes a symlink to a single `~/.ccx/shared-projects/` directory. Profile credentials stay isolated (per profile's `<CLAUDE_CONFIG_DIR>/.credentials.json`); only conversation history is shared.

**Rationale:**
- Claude Code's `Stop` hook fires on a turn boundary; between turns we can safely kill and restart `claude`.
- Restarting `claude` under a different `CLAUDE_CONFIG_DIR` would normally orphan the conversation, because each profile's old `projects/` subtree is invisible to the new profile.
- Symlinking all `projects/` dirs to one shared location lets `claude --resume <session-id>` find the same JSONL regardless of which profile is active.
- Anthropic's own claude.ai keeps conversations distinct from accounts; this is closer to that mental model.

**Trade-off:**
- We touch each managed profile's config dir on `ccx profile add` (creating the symlink). Migrating existing v0.1 profiles requires a one-shot `ccx migrate-shared-history` command.
- If Claude Code ever stores per-account session metadata *outside* `projects/` (e.g., a separate model preferences file), the swap could lose that. **Verification required before B3b lands** (see §11.1).

### 6.7 New SSE stream rather than extending `/api/usage/live`

**Decision:** `GET /api/recommendations/live` is a *new* SSE endpoint, not a new event type on `/api/usage/live`.

**Rationale:**
- `/api/usage/live` is documented as emitting `usage` events. Mixing in `recommendation` events breaks consumer assumptions.
- Different consumers care about different streams. The CLI's `ccx run --supervise` cares about recommendations but not usage row updates. The dashboard subscribes to both.
- Clean separation makes the recommendation stream easier to reason about and test.

**Trade-off:** the dashboard opens two SSE connections instead of one. Negligible on loopback.

## 7. Data model & contract changes

### 7.1 `internal/contracts/types.go` (frozen — P0 amendment PR)

`ProfileLimits` gains four optional fields:

```go
PlanTier        string  // "pro" | "max5" | "max20" | "api" | ""
WeeklyAnchor    string  // "rolling" (default) | "monday".."sunday"
Caps5hTurns     int     // 0 → use shipped default for PlanTier
CapsWeeklyTurns int     // 0 → use shipped default for PlanTier
```

Four new types are added:

```go
QuotaWindow         { Used, Cap int; Pct float64; ResetsAt time.Time }
ProfileQuota        { Profile, PlanTier string; Window5h, WindowWeekly QuotaWindow }
RecommendationLevel string                     // "warn" | "soft" | "hard"
RecommendationEvent { Profile string; Level RecommendationLevel; Reason, Suggested string; Quota5hPct, QuotaWeeklyPct float64; Timestamp time.Time }
```

**No changes** to existing types' wire format. All additions are pure additions.

### 7.2 `api/openapi.yaml` (frozen — same P0 amendment PR)

New routes:
- `GET /api/quota` → `[]ProfileQuota`
- `GET /api/recommendations/live` → SSE stream of `RecommendationEvent`

New schemas: `QuotaWindow`, `ProfileQuota`, `RecommendationEvent`.

Existing schema extension: `HeadroomCandidate` gains optional `quota_5h` and `quota_weekly` fields.

### 7.3 `internal/storage/schema.sql` and `migrate.go`

**No schema changes required.** Phase B1's turn count is a query against the existing `hook_events` table.

If a future iteration of this feature needs a materialized turn-count table for performance, that's a v3 migration. We don't pre-build.

### 7.4 `internal/headroom/` constants

Two non-contract constants ship as Go-level definitions (revisable any time):

```go
const (
    thresholdWarn = 0.75
    thresholdSoft = 0.90
    thresholdHard = 1.00

    defaultPro5h        = 45    // turns per 5h
    defaultPro7d        = 0     // pro has no weekly cap as of May 2026
    defaultMax5_5h      = 225
    defaultMax5_7d      = 0     // (TBD, see Appendix A)
    defaultMax20_5h     = 900
    defaultMax20_7d     = 0     // (TBD, see Appendix A)
)
```

**These numbers are best-effort. The user can override per-profile via the new ProfileLimits fields.** See Appendix A for sourcing and the May 2026 doubling.

## 8. UX

### 8.1 `ccx profile add` / `set`

```bash
ccx profile add work --config-dir ~/.claude-profiles/work \
                     --plan-tier max20 \
                     --weekly-anchor monday

ccx profile set work --plan-tier max5             # change tier
ccx profile set work --caps-5h-turns 400          # override the shipped default
```

### 8.2 `ccx usage --quota`

```
PROFILE   PLAN     5H WINDOW           WEEKLY WINDOW       TOKENS 24H    USD 30D
work      max20    142/900 (16%)       1203/—              4.2M          $—
personal  pro      45/45 (100%) ⛔     —                   1.1M          $—
api-dev   api      —                   —                   12.7M         $84.32
```

`⛔` = at hard cap. `pro` plan has no weekly cap (as of May 2026), so weekly is dashed.

### 8.3 `ccx suggest`

Existing output gains:

```
Recommended profile: work
Score: 89.5  Headroom: 84.0%  Auth: ok
Reasons:
  - 5h turns 142/900 (16%)
  - weekly turns 1203/4500 (27%)
  - auth ok
  - priority +0

PROFILE   AVAILABLE  SCORE   HEADROOM   AUTH   REASONS
work      true       89.5    84.0%      ok     5h turns 142/900 (16%)
personal  false      —       0.0%       ok     5h turns 45/45 (100%): hard cap
```

### 8.4 `ccx run`

```bash
$ claude            # if user installed --with-claude-wrapper
ccx: launched claude with profile=work (5h: 16%, weekly: 27%, score: 89.5)
[normal claude UI follows]
```

`ccx run --supervise` adds:
- One stderr line per swap event when threshold trips.
- Graceful relaunch between turns.

### 8.5 Dashboard

Insert a new panel above the existing TimeSeriesChart:

```
┌─ Plan Quota ─────────────────────────────────────────────────────┐
│ work      max20    5h:  ███░░░░░░░░░░░░░░  142/900 (16%)         │
│                    7d:  █████░░░░░░░░░░░░  1203/4500 (27%)       │
│                                                                  │
│ personal  pro      5h:  ████████████████░  45/45  (100%)  ⛔     │
└──────────────────────────────────────────────────────────────────┘
```

Bar coloring: green `<75`, yellow `<90`, orange `<100`, red `≥100`.

Live recommendation banner appears above the panel when an event arrives:

```
┌──────────────────────────────────────────────────────────────────┐
│ ⚠ Active profile `personal` has crossed the soft threshold       │
│   (5h: 92%). Consider switching to `work` (5h: 16%).             │
│   [ Switch to work ]                                              │
└──────────────────────────────────────────────────────────────────┘
```

## 9. Phasing

| Phase | Plan file | Scope | Estimated LOC |
|---|---|---|---|
| **P0** | `2026-05-24-ccx-quota-P0-contract-amendment.md` | ProfileLimits extensions, new types, openapi.yaml routes & schemas, api-types.ts regen | ~300 (across 3 files) |
| **B1** | `2026-05-24-ccx-quota-B1-analytics.md` | `headroom.Store.QueryTurnsInWindow`, `internal/headroom` reset-time math, `/api/quota` handler, `ccx usage --quota`, dashboard quota panel | ~700 |
| **B2** | `2026-05-24-ccx-quota-B2-pressure-suggest.md` | Tiered threshold constants, evaluator extension, Reason strings, `HeadroomCandidate` quota fields, `ccx suggest` rendering update | ~400 |
| **B3a** | `2026-05-24-ccx-quota-B3a-pre-launch-fallback.md` | `ccx run`, `ccx init --with-claude-wrapper`, daemon SSE recommendations emitter, dashboard live banner | ~900 |
| **B3b** | `2026-05-24-ccx-quota-B3b-supervisor-shared-history.md` | `ccx run --supervise`, projects symlinking, scanner refactor for shared scan, `ccx migrate-shared-history` | ~1100 |

Each phase is its own worktree, its own PR. Worktree workflow per the existing repo convention.

## 10. Risks & mitigations

| Risk | Phase | Mitigation |
|---|---|---|
| Shipped default caps drift from Anthropic's real numbers | P0/B1 | Caps are runtime overrides via ProfileLimits; defaults are just convenience. Document this clearly in `ccx profile set --help`. |
| User has no hooks installed → no quota data | B1 | Dashboard explicitly says "install hooks to see quota"; `/api/quota` returns `used: 0, cap: <default>, pct: 0` (not an error). |
| Two ccx instances both writing during a swap | B3a/B3b | Existing daemon lock already enforces single-writer for storage. The supervisor is a separate process from the daemon but only does read queries against state.db; no conflict. |
| Claude Code doesn't honor symlinks on `projects/` | B3b | **Prerequisite verification step in B3b plan.** Test with a manual symlink before committing to the architecture. |
| `claude --resume <session-id>` doesn't exist or isn't stable | B3b | **Prerequisite verification step.** If absent, B3b downgrades to "kill child, user restarts manually" — still useful, less seamless. |
| Live SSE recommendation events spam the dashboard | B3a | Coalesce: only emit when a *new* threshold is crossed. State machine per profile (`below_warn` → `warn` → `soft` → `hard`); transitions are events, steady state is not. |
| User over-relies on auto-switch and never notices a profile-level auth fail | B3b | The supervisor's switch logic explicitly skips auth-failed profiles (already in the evaluator). On unavailable=all, surface a hard error rather than silently falling back. |
| Anthropic introduces a new plan tier we don't know about | n/a | Plan tier is a free-form string; the only validation is "non-empty if quota tracking enabled." Unknown tiers fall back to user-provided caps. |

## 11. Verification required before implementation

Three items the implementation agent cannot resolve from this spec alone:

### 11.1 Does Claude Code follow symlinks for `<CLAUDE_CONFIG_DIR>/projects/`?

**Test:** create a profile with a real `projects/` directory, then replace it with a symlink to another directory. Start `claude`, send one message, verify the new JSONL lands in the symlink target. Required before B3b's shared-history architecture is locked.

### 11.2 Does Claude Code expose a `--resume <session-id>` flag?

**Test:** `claude --help`. If absent, search for equivalent (`--session`, `--continue`, env-var-based). Required before B3b's relaunch path is locked.

### 11.3 Does any Claude Code hook payload include `anthropic-ratelimit-*` data?

**Test:** force a 429 (e.g., hammer a Pro account briefly) and inspect the `StopFailure` payload via `ccx hooks record` debug mode. If headers are present, B1 gains a much more accurate truth source and §6.3 is revisited. If absent (expected), proceed as specified.

These verifications live as **Task 0 / Pre-flight** steps in the relevant plan files.

## 12. Open questions (for future iterations)

- **Should ProfileLimits expose thresholds?** Today they're constants. If real users want a "warn me at 60% on my work account" knob, add `Thresholds5h`, `ThresholdsWeekly` fields in a v0.3 amendment.
- **What about Bedrock / Vertex routing?** Out of scope for v0.2 — only the direct Anthropic API tiers are modeled. If users care, a `BedrockTier` flag could be added later.
- **Should the supervisor work without ccx-installed hooks?** Currently no — the supervisor reads `Stop` events from `hook_events`. A pollable-only fallback (parse JSONL turn boundaries) could be added but pulls in turn-inference fragility we deliberately avoided.

## 13. Appendix A — Anthropic plan cap landscape (May 2026)

Numeric caps shifted twice in 2026:
- **May 6, 2026:** Anthropic doubled all paid-plan 5h limits and removed the peak-hour multiplier.
- **June 2026 (announced May 27):** Anthropic raised Claude Code weekly limits by 50% through July 13, then revert.

**Published numbers as of 2026-05-24 (community-sourced; not authoritative — `Caps5hTurns` overrides):**

| Tier | 5h turns | Weekly turns | Notes |
|---|---|---|---|
| Pro | ~45 | N/A | Some users report soft weekly limits — leave as 0 by default. |
| Max 5x | ~225 | ~1,200 | Pre-May-6 was ~110. |
| Max 20x | ~900 | ~4,500 | Pre-May-6 was ~440. |
| API | ∞ | ∞ | Token-cost view only. |

Sources to be re-validated by the implementation agent before B1 lands:
- <https://claudefa.st/blog/guide/development/higher-usage-limits>
- <https://apidog.com/blog/claude-code-weekly-limits-50-percent-increase-july-2026/>
- <https://support.claude.com/en/articles/12429409-manage-extra-usage-for-paid-claude-plans>
- <https://intuitionlabs.ai/articles/claude-max-plan-pricing-usage-limits>

If Anthropic publishes authoritative numbers between spec-writing and B1 implementation, the agent should prefer those.

## 14. Done definition for v0.2.0

- All 5 plans (P0, B1, B2, B3a, B3b) merged.
- `ccx --version` reports `0.2.0`.
- `make ci` green across darwin/linux/windows.
- Manual smoke: `ccx profile add demo --plan-tier max5`, install hooks, run one `claude` session, verify the quota panel updates in `ccx dashboard`.
- Documentation: README gains a "Plan-aware quota" subsection above the "Usage analytics" subsection.
- `docs/troubleshooting.md` gains entries for "quota panel is empty" and "supervisor didn't swap when I expected".
- v0.2.0 git tag pushed; GitHub Release published with signed binaries.

---

**Approved for plan decomposition.** See `docs/superpowers/plans/2026-05-24-ccx-quota-plan-index.md` for the five-plan implementation registry.
