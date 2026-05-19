# ccx Phase 3 — Polish & Launch

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans for the human-in-the-loop tasks here. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take the integrated v0.1.0-rc binary, validate it on real users, polish the launch assets, ship v0.1.0, and execute a coordinated Show HN / Twitter / Discord launch.

**Architecture:** This phase is process, not code. Most tasks are observational, communication, or asset capture. A few small code fixes will surface from friend-testing — handle those as targeted patch commits on `main`.

**Tech Stack:** vhs (GIF recording), Excalidraw (diagram), OBS or QuickTime (screen recording), `gh` CLI, browser screenshots.

**Spec reference:** [`../specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — sections 10.3, 10.4.

**Worktree:** `main`. No long-lived feature branch. Friend-feedback fixes are short-lived `fix/<topic>` branches.

**Pre-flight:**

```bash
git checkout main && git pull --ff-only
git log --oneline | grep -E "phase-(0|1-A[1-9]|2)" | wc -l   # expect 11 (P0 + A1..A9 + P2)
make ci                                                        # CI gate green locally
make build && ./dist/ccx version                                # binary runs
```

If pre-flight fails, fix before continuing.

---

## Task 1: Friend-test (3-5 people, 30-60 minutes each)

This is the most important task. It surfaces every assumption you didn't realize you made.

- [ ] **Step 1: Pick testers**

Choose 3-5 people who:
- Use Claude Code daily
- Are on a mix of macOS / Linux / Windows
- Will give honest feedback

DM them: "I built a thing for managing Claude Code accounts. 30 min, screen-share, you install + use it, I watch and don't help. Worth a beer?"

- [ ] **Step 2: Build a release-candidate**

```bash
git tag v0.1.0-rc1
git push origin v0.1.0-rc1
gh release create v0.1.0-rc1 --prerelease --notes "Release candidate for friend-testing only."
gh release upload v0.1.0-rc1 dist/ccx-*
```

Goreleaser (via release.yml) will produce all the artifacts automatically.

- [ ] **Step 3: Run each session**

Open a shared doc with a stopwatch. For each tester:

| Time | What you watch for |
|---|---|
| 0:00 | Hand them the install one-liner. **Don't explain anything.** |
| 0:00–5:00 | Watch them install. Note every place they pause or look confused. |
| 5:00–10:00 | Ask: "Now register your existing Claude Code account." Watch them figure out the syntax. |
| 10:00–15:00 | Ask: "Now make a second one and switch between them." Watch shell-init confusion. |
| 15:00–25:00 | Ask: "How much have you spent today?" Watch them discover `ccx usage`. |
| 25:00–35:00 | Ask: "Show me your dashboard." Watch their reaction. |
| 35:00–end | Open-ended feedback. Don't defend choices. Take notes. |

- [ ] **Step 4: Consolidate feedback**

Create `docs/superpowers/friend-test-notes.md` (do not commit — local-only). For each tester, list:
- Time-to-first-`ccx use` (the killer metric)
- Where they got stuck
- What surprised them (positive or negative)
- One direct quote, verbatim

Identify the **top 3 friction points** across all sessions.

---

## Task 2: Fix top 3 friction points

This is open-ended. Likely candidates based on what historically trips up Claude Code users:

| Friction (anticipated) | Likely fix |
|---|---|
| "I forgot to `eval`" | Improve the `ccx use` output to remind the user; add a `--dry-run`-style flag that explains what's happening |
| "Doctor said warn but I don't know how to fix it" | Make every `warn`/`fail` doctor check have a `Remediation:` line; review them all |
| "Pricing numbers seem off" | Add `--show-rates` flag to `ccx usage` that prints the embedded rate table; document override in README |
| "I broke my shell rc" | Add `ccx init --check` mode that verifies the rc file already has the snippet, rather than blindly appending |
| "Dashboard didn't open" | Add a better error when `xdg-open` is missing; print the URL prominently anyway |

For each friction:

- [ ] **Step 1:** Open an issue with the verbatim user quote in the body.
- [ ] **Step 2:** Branch `fix/<topic>`, write the fix following TDD (failing test → fix → passing test).
- [ ] **Step 3:** Push, open PR, get CI green, merge.
- [ ] **Step 4:** Close the issue with the PR number.

---

## Task 3: Capture the dashboard screenshot

**Files:** `docs/assets/dashboard.png`

- [ ] **Step 1: Generate realistic seed data**

```bash
# Set up a test home with 3 profiles + several days of fake JSONL
./scripts/seed-demo-data.sh    # write this script — see Step 2
```

The seed script populates `~/.claude-profiles/{personal,work,side}/projects/.../*.jsonl` with realistic-looking events spanning the last 14 days. Use lorem-ipsum project names and realistic token counts (1k–100k per session).

- [ ] **Step 2: Write the seed script**

Create `scripts/seed-demo-data.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
HOME_BASE=${1:-$HOME}
for profile in personal work side; do
  cfg="$HOME_BASE/.claude-profiles/$profile"
  mkdir -p "$cfg/projects"
  for day in {0..13}; do
    ts=$(date -u -v-${day}d +"%Y-%m-%dT12:00:00Z" 2>/dev/null || date -u -d "$day days ago" +"%Y-%m-%dT12:00:00Z")
    proj_dir="$cfg/projects/-Users-arafa-Developer-demo$day"
    mkdir -p "$proj_dir"
    sid=$(uuidgen | tr 'A-Z' 'a-z')
    in_tok=$((RANDOM % 50000 + 5000))
    out_tok=$((RANDOM % 10000 + 1000))
    cache=$((RANDOM % 100000 + 10000))
    cat > "$proj_dir/$sid.jsonl" <<EOF
{"type":"user","uuid":"$(uuidgen)","sessionId":"$sid","timestamp":"$ts","cwd":"/Users/arafa/Developer/demo$day","message":{"content":[{"type":"text","text":"demo"}]}}
{"type":"assistant","uuid":"$(uuidgen)","sessionId":"$sid","timestamp":"$ts","message":{"model":"claude-opus-4-7","usage":{"input_tokens":$in_tok,"output_tokens":$out_tok,"cache_creation_input_tokens":$cache,"cache_read_input_tokens":$((cache*3))}}}
EOF
  done
done
echo "Seeded $HOME_BASE/.claude-profiles/{personal,work,side}/projects/"
```

Make executable:
```bash
chmod +x scripts/seed-demo-data.sh
```

- [ ] **Step 3: Seed and screenshot**

```bash
./scripts/seed-demo-data.sh "$HOME"
ccx profile add personal --config-dir ~/.claude-profiles/personal --color "#3B82F6"
ccx profile add work --config-dir ~/.claude-profiles/work --color "#10B981"
ccx profile add side --config-dir ~/.claude-profiles/side --color "#F59E0B"
ccx usage  # ingests, primes SQLite
ccx dashboard
```

Take a screenshot:
- macOS: `Cmd+Shift+4`, drag over browser window
- Save as `docs/assets/dashboard.png`
- Recommended dimensions: 2880×1620 (retina, 16:9) cropped from a 1440×900 viewport

- [ ] **Step 4: Commit**

```bash
git add scripts/seed-demo-data.sh docs/assets/dashboard.png
git commit -m "docs: add demo data seed script and dashboard screenshot"
```

---

## Task 4: Re-record `demo.gif`

**Files:** `docs/assets/demo.gif`

- [ ] **Step 1: Run vhs with the working binary + seeded data**

```bash
# Ensure data is seeded and profiles are registered
ls ~/.claude-profiles/

# Run vhs
vhs docs/assets/demo.tape
file docs/assets/demo.gif
```

Expected: a clean GIF showing `ccx profile list` → `ccx use work` → `claude --version` → `ccx usage` → `ccx dashboard`.

- [ ] **Step 2: Watch the GIF, fix the tape if it looks rough**

Common tweaks in `docs/assets/demo.tape`:
- Increase Sleep on tense moments
- Decrease TypingSpeed for the build-up moments
- Cut frames where the terminal sits idle

Re-render and review until you'd put it on Twitter.

- [ ] **Step 3: Commit**

```bash
git add docs/assets/demo.gif
git commit -m "docs: re-record demo.gif with working binary"
```

---

## Task 5: Hand-author the architecture diagram

**Files:** `docs/assets/architecture.excalidraw`, `docs/assets/architecture.png`, `docs/assets/architecture.svg`

This task is manual — Excalidraw needs a human.

- [ ] **Step 1: Open Excalidraw**

Use https://excalidraw.com (no login needed).

- [ ] **Step 2: Draw the diagram per `docs/assets/README.md`**

Three lanes left-to-right:
1. User terminal box → ccx CLI box
2. ccx CLI box (with sub-pills: profile, scanner, storage, pricing, shell, server, tui, doctor) → SQLite cylinder + Browser box (localhost:7777)
3. Upstream `claude` CLI → Anthropic API (greyed)

Annotate the connecting arrow with `CLAUDE_CONFIG_DIR`.

- [ ] **Step 3: Export**

In Excalidraw: File → Save to .excalidraw (raw source). File → Export as image → PNG (2x scale, white background) and SVG.

Save to:
- `docs/assets/architecture.excalidraw`
- `docs/assets/architecture.png`
- `docs/assets/architecture.svg`

- [ ] **Step 4: Commit**

```bash
git add docs/assets/architecture.*
git commit -m "docs: add architecture diagram (Excalidraw)"
```

---

## Task 6: Final README polish pass

**Files:** `README.md`

- [ ] **Step 1: Read the entire README out loud**

Note any sentence that's awkward, vague, or oversells. Edit it.

- [ ] **Step 2: Verify every link**

```bash
npx --yes lychee --no-progress --exclude-file .lycheeignore README.md
```

- [ ] **Step 3: Verify every command works**

Copy each command from the README's quick-start, paste into a fresh shell, verify behavior.

- [ ] **Step 4: Verify all referenced images render**

```bash
ls docs/assets/{demo.gif,dashboard.png,architecture.png}
```

- [ ] **Step 5: Commit any changes**

```bash
git add README.md
git commit -m "docs: README polish pass" || echo "no changes"
```

---

## Task 7: Final CHANGELOG entry + bump version

**Files:** `CHANGELOG.md`

- [ ] **Step 1: Update CHANGELOG.md**

Move the `[Unreleased]` items into a new `[0.1.0]` section if any were added since Task 6 of Plan A9. Update the date to the actual release date (today).

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: finalize CHANGELOG for v0.1.0"
```

---

## Task 8: Tag v0.1.0 and let goreleaser ship

- [ ] **Step 1: Sanity-check `main`**

```bash
git checkout main && git pull --ff-only
make ci
make build
./dist/ccx --version
./dist/ccx doctor
```

All should be clean.

- [ ] **Step 2: Tag**

```bash
git tag -a v0.1.0 -m "v0.1.0 — Initial release"
git push origin v0.1.0
```

- [ ] **Step 3: Watch the release job**

```bash
gh run watch
```

When it completes, verify:

```bash
gh release view v0.1.0     # release exists, has all artifacts
brew install arafa-dev/tap/ccx     # works fresh
scoop install ccx                   # works fresh (on Windows)
curl -fsSL https://ccx.sh/install | sh    # works fresh
```

If any verification fails, do not announce the launch. Fix and re-tag as v0.1.1.

---

## Task 9: Pre-launch checklist (T-minus 24 hours)

- [ ] **Step 1: Issue templates and labels ready**

```bash
gh label create "good-first-issue" --color "7057ff" --description "Good for newcomers" || true
gh label create "needs-triage" --color "ededed" || true
```

Create at least 3 `good-first-issue` issues — small things you'd love a contributor to take on.

- [ ] **Step 2: Draft the HN post (do not submit yet)**

Title: `Show HN: ccx — Multi-account workspace manager for Claude Code (Go, single binary)`

Body (first comment in HN convention):
```
Hi HN — I built ccx because I ended up juggling two Claude Code accounts (work and personal) with shell aliases that swap ~/.claude in and out, plus ccusage in a separate tab to track spend. Two tools to do one job.

ccx switches between Claude Code accounts (`ccx use work`) and tracks per-profile usage (`ccx usage`) from one binary. It does this by exporting CLAUDE_CONFIG_DIR — Claude Code's official env var for account isolation — so it never proxies API requests.

Single Go binary (no Node, no Tauri), distributes via Homebrew, Scoop, .deb, .rpm, and curl|sh. Embedded Next.js dashboard via go:embed. MIT.

Repo: https://github.com/arafa-dev/ccx
GIF: https://github.com/arafa-dev/ccx (top of README)

Honest caveats: ccusage has deeper analytics than ccx if all you need is reporting. ccx is for people who run multiple accounts and want both in one place.

Happy to answer questions about the design — especially the macOS Keychain isolation (Claude Code derives the keychain service name from the config dir path via SHA256, which makes per-profile credential routing automatic).
```

- [ ] **Step 3: Draft the tweet thread (do not post yet)**

Tweet 1 (with the demo GIF):
```
ccx — switch between Claude Code accounts in one command, and see your real usage across all of them. Single Go binary, macOS + Linux + Windows.

brew install arafa-dev/tap/ccx

(thread ↓)
```

Tweet 2 (with the dashboard screenshot):
```
The dashboard runs locally (127.0.0.1 only), no telemetry, no login. Embedded Next.js via go:embed.
```

Tweet 3 (with the architecture diagram):
```
The trick: ccx never proxies API calls. It just exports CLAUDE_CONFIG_DIR for the right profile before `claude` runs. macOS Keychain routes by config-dir hash automatically, so no credential copying.
```

Tweet 4:
```
Built it because I kept hot-swapping ~/.claude with shell aliases and reading ccusage in another tab. Two tools, one job. Now one tool.

https://github.com/arafa-dev/ccx
```

- [ ] **Step 4: Draft Discord / Reddit posts**

For r/ClaudeAI and the Anthropic Discord #show-and-tell channel — same content as the HN body, two paragraphs, link at the end.

- [ ] **Step 5: Pre-respond to the most likely critical comment**

Common HN comment patterns + drafted responses (keep these ready in a notes file):

| Comment pattern | Response |
|---|---|
| "ccusage already does this" | "ccusage does analytics, not switching. ccx does both. If you only need analytics, use ccusage — it goes deeper than we do." |
| "Why Go, not Rust/TypeScript?" | "Single binary distribution, mature CLI ecosystem (cobra/bubbletea), and pure-Go SQLite means clean cross-compile for Windows. TS via Bun was tempting but ccusage already lives there." |
| "Won't Anthropic just build this into claude itself?" | "Maybe — issue #44687 is open. ccx is useful today, and if Anthropic does ship native multi-account support, ccx still owns the usage-analytics half." |
| "What about the ToS — isn't routing requests banned?" | "We don't route. ccx never proxies API calls. It only swaps CLAUDE_CONFIG_DIR before you run `claude` yourself." |

---

## Task 10: Launch day

**Timing target:** Tuesday or Wednesday, 9:00 AM Pacific (peak HN traffic, US morning).

- [ ] **Step 1 (T-30m): Final smoke test**

```bash
brew uninstall ccx 2>/dev/null
brew install arafa-dev/tap/ccx
ccx --version
ccx doctor
```

- [ ] **Step 2 (T-0): Submit Show HN**

Submit the HN post. Note the URL.

- [ ] **Step 3 (T-2m): Post first comment**

As the OP, post the prepared body as the first comment (HN convention — submission is just the title + link).

- [ ] **Step 4 (T-5m): Tweet the thread**

Post the 4-tweet thread on X/Twitter. Include the GIF in tweet 1. Reply to your own tweet 1 with the HN link.

- [ ] **Step 5 (T-30m): Reddit + Discord**

Post to r/ClaudeAI and Anthropic Discord #show-and-tell.

- [ ] **Step 6 (T-1h to T+24h): Reply within 1 hour to every comment and issue**

This is the single highest-leverage thing you can do for launch momentum. Set notifications on. Refresh.

For each GitHub issue:
- Tag with the relevant label
- Thank them for filing
- Either: link to existing docs, or commit to fixing it within N days

- [ ] **Step 7 (T+24h): Decide on a v0.1.1 patch release**

If multiple users report the same friction, ship a v0.1.1 within 48 hours. This converts launch viewers into ongoing users.

---

## Task 11: Post-launch retrospective (T+7 days)

**Files:** `docs/superpowers/launch-retro.md` (local-only, do not commit)

Write down:
- Final star count, download count, contributor count
- What landed worse than expected
- What landed better than expected
- What the next 30 days should focus on (likely: v0.2 daemon, or whatever the loudest feedback pointed at)

This document feeds the v0.2 spec when you start it.

---

## Done definition

- [ ] Friend-tested with 3+ users on different OSes
- [ ] Top 3 friction points fixed and merged
- [ ] `docs/assets/demo.gif` re-rendered with working binary
- [ ] `docs/assets/dashboard.png` captured
- [ ] `docs/assets/architecture.{excalidraw,png,svg}` committed
- [ ] README polished and every link verified
- [ ] CHANGELOG updated with v0.1.0 entry and final release date
- [ ] `v0.1.0` tag pushed
- [ ] GitHub Release published with all artifacts (binaries × 6 + deb + rpm + checksums + signatures)
- [ ] `brew install arafa-dev/tap/ccx` works on a fresh Mac
- [ ] `scoop install ccx` works on a fresh Windows
- [ ] `curl -fsSL https://ccx.sh/install | sh` works on a fresh Linux
- [ ] Show HN post submitted with first comment
- [ ] Tweet thread published
- [ ] Reddit + Discord posts published
- [ ] All issues opened within first 24h replied to within 1h

Launch complete.

After launch, v0.2 planning starts: revisit the spec roadmap, decide on daemon vs. hooks vs. advisory routing based on launch-week feedback, write a new spec + plan index for v0.2.
