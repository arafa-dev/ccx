# ccx Phase 1 A9 — Documentation & launch assets

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce the marketing-grade `README.md`, per-topic docs, CHANGELOG, vhs demo tape, and GitHub issue templates required for a credible v0.1.0 launch.

**Architecture:** Pure documentation. No Go code. Each doc is its own file with a single clear responsibility. The `README.md` is the primary asset; everything else is supporting reference.

**Tech Stack:** Markdown (CommonMark + GFM), vhs (terminal GIF recorder), markdownlint-cli, cspell, lychee, GitHub Actions.

**Spec reference:** [`../specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — sections 2, 6, 10.2, 12.

**Worktree:** `feat/docs` (off `main`, after Phase 0 is merged).

**Exit criteria:**
- `README.md` exists at repo root with all 11 sections from spec §10.2
- `docs/installation.md`, `docs/shell-integration.md`, `docs/architecture.md`, `docs/troubleshooting.md` exist
- `CHANGELOG.md` exists with v0.1.0 skeleton
- `docs/assets/demo.tape` exists and produces a working GIF when run with `vhs`
- `docs/assets/README.md` documents Excalidraw diagram requirements
- `.github/ISSUE_TEMPLATE/{bug,feature}.yml` exist
- `.github/workflows/docs.yml` runs markdownlint, cspell, and lychee
- All checks green in CI

---

## Pre-flight

```bash
git checkout main && git pull --ff-only
git worktree add ../ccx-docs -b feat/docs main
cd ../ccx-docs
mkdir -p docs/assets docs/distribution .github/ISSUE_TEMPLATE
```

Verify Phase 0 is merged: `git log --oneline | grep "feat: add Phase 1 package stubs"` returns a hit.

---

## Task 1: Write the top-level `README.md`

**Files:** Create: `README.md`

- [ ] **Step 1: Write `README.md`**

```markdown
# ccx — Claude Code eXtended

[![CI](https://github.com/arafa-dev/ccx/actions/workflows/ci.yml/badge.svg)](https://github.com/arafa-dev/ccx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/arafa-dev/ccx)](https://github.com/arafa-dev/ccx/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/arafa-dev/ccx)](https://goreportcard.com/report/github.com/arafa-dev/ccx)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> Switch between Claude Code accounts in one command, see your real usage across all of them — from a single Go binary.

![ccx demo](docs/assets/demo.gif)

## Install

```bash
# macOS / Linux
brew install arafa-dev/tap/ccx

# Windows
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop install ccx

# Debian / Ubuntu
curl -fsSL https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.deb -o /tmp/ccx.deb
sudo dpkg -i /tmp/ccx.deb

# One-liner (macOS / Linux)
curl -fsSL https://ccx.sh/install | sh

# From source
go install github.com/arafa-dev/ccx/cmd/ccx@latest
```

## 60-second quick start

```bash
# 1. Register your existing default account as a profile
ccx profile add personal --config-dir ~/.claude

# 2. Add a second profile and authenticate it
ccx profile add work --config-dir ~/.claude-profiles/work
eval "$(ccx use work)"
claude /login        # authenticate the work account

# 3. Switch any time
eval "$(ccx use personal)"

# 4. See usage across all profiles
ccx usage

# 5. Open the dashboard
ccx dashboard
```

![dashboard screenshot](docs/assets/dashboard.png)

## Why ccx?

If you run more than one Claude Code account — Pro plus Max, work plus personal, multiple Pro accounts on a shared machine — you've probably ended up with a directory of shell aliases that hot-swap `~/.claude`, plus `ccusage` open in another tab for analytics. Two tools to do one job.

ccx is the one tool. It switches accounts (the part `ccusage` doesn't do) and tracks usage (the part the bash-alias scripts don't do), all from a single Go binary.

## How it works

ccx never proxies API calls. The upstream `claude` CLI does what it always did — ccx just chooses *which* config dir and *which* keychain entry `claude` reads from, by exporting `CLAUDE_CONFIG_DIR` in your shell.

![architecture diagram](docs/assets/architecture.png)

For details, see [`docs/architecture.md`](docs/architecture.md).

## Comparison

|  | **ccx** | ccusage | claude-account-switcher | ccs |
|---|---|---|---|---|
| Profile switching | ✅ | ❌ | ✅ (bash only) | ✅ |
| Usage analytics | ✅ | ✅ (deepest) | ❌ | partial |
| Local dashboard | ✅ | ❌ | ❌ | ❌ |
| Single binary | ✅ | npm | shell script | TS + Tauri |
| Cross-platform | macOS · Linux · Windows | ✅ | macOS-focused | ✅ |
| Distribution | brew · scoop · apt · curl\|sh | npm | manual | GitHub Releases |

Be honest: `ccusage` has deeper analytics than ccx (per-message drilldowns, custom date ranges). If analytics is your only need, use ccusage. If you also juggle accounts, ccx replaces both.

## Configuration

ccx state lives in `~/.ccx/`:

- `~/.ccx/profiles.toml` — registered profile list (human-editable)
- `~/.ccx/state.db` — SQLite cache of parsed events
- `~/.ccx/pricing.yaml` — optional override for the embedded model pricing table

Profile config dirs live wherever you put them (commonly `~/.claude-profiles/<name>`). On macOS the credential lives in Keychain under a service name derived from the config dir path — ccx does not touch your credentials directly.

See [`docs/installation.md`](docs/installation.md) for per-platform setup details.

## Roadmap

Planned, not promised:

- **v0.2** — Long-running daemon (`ccx daemon start`) so the dashboard updates without a foreground process
- **v0.3** — Claude Code hooks integration for per-session telemetry (durations, exit codes)
- **v0.4** — Advisory routing: `ccx suggest` recommends which profile has headroom (never proxies)
- **v0.5+** — Team workspace primitives, profile sync via git

## Contributing

Issues, bug reports, and PRs welcome. Please read [`CONTRIBUTING.md`](CONTRIBUTING.md) and [`docs/conventions.md`](docs/conventions.md) before opening a PR.

## License

[MIT](LICENSE)
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add top-level README"
```

---

## Task 2: Write `docs/installation.md`

**Files:** Create: `docs/installation.md`

- [ ] **Step 1: Write `docs/installation.md`**

```markdown
# Installing ccx

## macOS

```bash
brew install arafa-dev/tap/ccx
```

Requires Homebrew. After install, set up shell integration once:

```bash
echo 'eval "$(ccx init zsh)"' >> ~/.zshrc   # or ~/.bashrc for bash
source ~/.zshrc
```

## Linux

### Debian / Ubuntu

```bash
curl -fsSL https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.deb -o /tmp/ccx.deb
sudo dpkg -i /tmp/ccx.deb
```

For arm64: replace `amd64` with `arm64`.

### Fedora / RHEL

```bash
sudo rpm -i https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.rpm
```

### Other distros (binary)

```bash
curl -fsSL https://ccx.sh/install | sh
```

The script picks the right binary for your OS and arch, verifies its SHA256, and installs it to `/usr/local/bin/ccx` (or `~/.local/bin/ccx` if not root).

Then set up shell integration:

```bash
echo 'eval "$(ccx init bash)"' >> ~/.bashrc    # or ~/.zshrc for zsh
echo 'ccx init fish | source' >> ~/.config/fish/config.fish   # for fish
source ~/.bashrc
```

## Windows

```powershell
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop install ccx
```

Then add this to your PowerShell profile (`$PROFILE`):

```powershell
ccx init pwsh | Out-String | Invoke-Expression
```

## From source

Requires Go 1.22+:

```bash
go install github.com/arafa-dev/ccx/cmd/ccx@latest
```

## Verifying the install

```bash
ccx version
ccx doctor
```

`ccx doctor` reports the status of your install and any registered profiles. Every check should show ✅ for a healthy setup.

## Uninstalling

```bash
brew uninstall ccx                              # macOS
sudo apt remove ccx                             # Debian/Ubuntu
sudo dnf remove ccx                             # Fedora
scoop uninstall ccx                             # Windows
rm /usr/local/bin/ccx                           # binary install
rm -rf ~/.ccx                                   # state directory
```
```

- [ ] **Step 2: Commit**

```bash
git add docs/installation.md
git commit -m "docs: add installation guide"
```

---

## Task 3: Write `docs/shell-integration.md`

**Files:** Create: `docs/shell-integration.md`

- [ ] **Step 1: Write `docs/shell-integration.md`**

```markdown
# How `ccx use` actually switches accounts

ccx never modifies your shell from outside. It can't — child processes can't change parent-shell env vars. Instead, `ccx use` *prints* the right `export` statements, and your shell `eval`s them.

## The mechanism

```bash
$ eval "$(ccx use work)"
```

`ccx use work` writes to stdout:

```sh
export CLAUDE_CONFIG_DIR='/Users/you/.claude-profiles/work'
export CCX_ACTIVE_PROFILE='work'
```

Your shell evaluates those exports. The `claude` CLI you run next reads its config from `CLAUDE_CONFIG_DIR`.

## Per-OS credential isolation

- **macOS** — Claude Code derives its Keychain service name from the config dir path via SHA256. Switching `CLAUDE_CONFIG_DIR` automatically routes to a different Keychain entry. No file copying.
- **Linux** — `.credentials.json` lives inside `CLAUDE_CONFIG_DIR`. Switching the env var switches the file.
- **Windows** — Same as Linux. `.credentials.json` inside `CLAUDE_CONFIG_DIR`.

## Shell init

Typing `eval "$(ccx use work)"` every time is awkward. `ccx init <shell>` emits a wrapper function that hides the `eval`:

```bash
eval "$(ccx init zsh)"        # one-time, in ~/.zshrc
ccx use work                  # works directly from now on
```

The wrapper is shell-specific. Supported shells: `zsh`, `bash`, `fish`, `pwsh`.

## What happens to the active profile

`ccx use` sets `CCX_ACTIVE_PROFILE` so other ccx commands know which profile you're using. `ccx profile current` reads it. `ccx usage` defaults to it.

If you log into a new shell without running `ccx use`, no profile is active — `claude` falls back to the default `~/.claude` config.

## Authenticating a new profile

The first time you switch to a profile, no credentials exist yet:

```bash
ccx profile add work --config-dir ~/.claude-profiles/work
eval "$(ccx use work)"
claude /login         # authenticate; credentials go to Keychain (macOS) or .credentials.json (Linux/Windows)
```

Subsequent `ccx use work` activations skip the login — the credentials persist.

## Troubleshooting

See [`docs/troubleshooting.md`](troubleshooting.md).
```

- [ ] **Step 2: Commit**

```bash
git add docs/shell-integration.md
git commit -m "docs: explain shell integration mechanism"
```

---

## Task 4: Write `docs/architecture.md`

**Files:** Create: `docs/architecture.md`

- [ ] **Step 1: Write `docs/architecture.md`**

```markdown
# ccx architecture

## High level

ccx is a single Go binary. It does three things: manage a registry of Claude Code accounts ("profiles"), parse the JSONL session files those accounts produce, and present that data via a CLI or an embedded web dashboard.

ccx never proxies API calls. The upstream `claude` CLI handles all Anthropic API communication. ccx only manipulates the environment that `claude` runs in.

![architecture diagram](assets/architecture.png)

## Components

| Layer | Package | Responsibility |
|---|---|---|
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

```
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
|---|---|
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
- ccx never reads credential contents; on macOS the OS Keychain holds them, on Linux/Windows the file lives inside `CLAUDE_CONFIG_DIR` and ccx never opens it

See [`SECURITY.md`](../SECURITY.md) for disclosure policy.
```

- [ ] **Step 2: Commit**

```bash
git add docs/architecture.md
git commit -m "docs: add architecture overview"
```

---

## Task 5: Write `docs/troubleshooting.md`

**Files:** Create: `docs/troubleshooting.md`

- [ ] **Step 1: Write `docs/troubleshooting.md`**

```markdown
# Troubleshooting

## `ccx use foo` does nothing

You probably forgot to wrap it in `eval` (or didn't run `ccx init <shell>` once).

Workaround:

```bash
eval "$(ccx use foo)"
```

Permanent fix:

```bash
echo 'eval "$(ccx init zsh)"' >> ~/.zshrc        # or bash, fish, pwsh
source ~/.zshrc
```

## After switching, `claude` still uses the old account (macOS)

Claude Code derives its Keychain service name from `CLAUDE_CONFIG_DIR` via SHA256. If you switched but `claude` still asks you to log in, run:

```bash
ccx doctor
```

It will report which profile the env says is active and whether a matching Keychain entry exists. If the Keychain entry is missing for the active profile, run `claude /login` once to create it.

## `ccx dashboard` says port already in use

ccx tries ports 7777–7787 in order. If all are taken (unlikely):

```bash
ccx dashboard --port 8888
```

## `ccx usage` shows $0 even though I've used Claude

Check:

1. `ccx profile current` — is a profile active?
2. `ccx doctor` — does the profile's config dir exist?
3. Look at `~/.ccx/state.db` — is it being written? `ls -la ~/.ccx/`
4. Look at the JSONL source — `ls -la $CLAUDE_CONFIG_DIR/projects/`

If all of those look fine, run `ccx usage --verbose` and file an issue with the output.

## Cost numbers look wrong

ccx ships an embedded pricing table that's accurate as of the version's release date. If Anthropic changes prices, your numbers will drift until ccx ships an updated table.

To override:

```bash
cat > ~/.ccx/pricing.yaml <<'EOF'
last_updated: 2026-06-01
models:
  - model: claude-opus-4-7
    effective_from: 2026-06-01
    input_per_mtok: 12.00
    output_per_mtok: 60.00
    cache_read_per_mtok: 1.20
    cache_create_per_mtok: 15.00
EOF
```

User overrides layer on top of the embedded table by model name. Run `ccx usage` again to see the new numbers.

## Pre-existing `~/.claude` — should I migrate?

No migration needed. Register it as a profile:

```bash
ccx profile add personal --config-dir ~/.claude
```

Your existing data is now visible to ccx. No files were moved.

## Filing a bug

```bash
ccx doctor > doctor.txt
ccx version > version.txt
```

Attach both to your issue.
```

- [ ] **Step 2: Commit**

```bash
git add docs/troubleshooting.md
git commit -m "docs: add troubleshooting guide"
```

---

## Task 6: Write `CHANGELOG.md` skeleton

**Files:** Create: `CHANGELOG.md`

- [ ] **Step 1: Write `CHANGELOG.md`**

```markdown
# Changelog

All notable changes to ccx are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and ccx adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] — 2026-07-21

### Added

- `ccx profile add | list | rm | current` — TOML-backed profile registry at `~/.ccx/profiles.toml`
- `ccx use <name>` — POSIX `export` / PowerShell `$env:` snippets for shell `eval`
- `ccx use` (no argument) — interactive TUI picker (bubbletea)
- `ccx init zsh | bash | fish | pwsh` — rc-file wrapper-function snippet
- `ccx usage [--profile] [--since] [--json]` — aggregated usage with cost estimates
- `ccx dashboard` — embedded Next.js dashboard served on `127.0.0.1:7777`
- `ccx doctor` — structured diagnostic report
- `ccx version`
- SQLite-backed event cache with incremental scan cursors
- Embedded pricing table for claude-opus-4-7, claude-sonnet-4-6, claude-haiku-4-5
- User-overridable pricing via `~/.ccx/pricing.yaml`
- Cross-platform distribution: Homebrew tap, Scoop bucket, .deb, .rpm, `curl|sh` install script, `go install`
- Signed release artifacts via cosign

[Unreleased]: https://github.com/arafa-dev/ccx/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/arafa-dev/ccx/releases/tag/v0.1.0
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add CHANGELOG with v0.1.0 entry"
```

---

## Task 7: Write `docs/assets/demo.tape` (vhs script)

**Files:** Create: `docs/assets/demo.tape`

- [ ] **Step 1: Install vhs if not present**

```bash
brew install vhs 2>/dev/null || go install github.com/charmbracelet/vhs@latest
vhs --version
```

- [ ] **Step 2: Write `docs/assets/demo.tape`**

```
Output docs/assets/demo.gif

Set Theme "Catppuccin Mocha"
Set FontSize 16
Set Width 1200
Set Height 675
Set Padding 24
Set TypingSpeed 70ms
Set PlaybackSpeed 1.0

Type "ccx profile list"
Enter
Sleep 1.5s

Type "ccx use work"
Enter
Sleep 800ms

Type "claude --version"
Enter
Sleep 1.2s

Type "ccx usage"
Enter
Sleep 2s

Type "ccx dashboard"
Enter
Sleep 2.5s
```

- [ ] **Step 3: Test-render the GIF**

```bash
vhs docs/assets/demo.tape
file docs/assets/demo.gif
```

Expected: `docs/assets/demo.gif: GIF image data ...`

Note: at Phase 1 time the binary doesn't actually do anything yet, so vhs will record the error output. That's fine — Phase 3 (Polish) re-records once the real binary works.

- [ ] **Step 4: Commit**

```bash
git add docs/assets/demo.tape
# Do NOT commit the .gif — it gets re-rendered in Phase 3.
git commit -m "docs: add vhs demo tape"
```

---

## Task 8: Document the architecture diagram source

**Files:** Create: `docs/assets/README.md`

The architecture PNG/SVG is a hand-authored asset. This task documents what to put in it so anyone (including the user) can regenerate.

- [ ] **Step 1: Write `docs/assets/README.md`**

```markdown
# Diagram assets

This directory holds non-code assets referenced from documentation.

## `architecture.png` and `architecture.svg`

Hand-authored in Excalidraw. Source: `architecture.excalidraw` (commit it alongside).

### What the diagram shows

Three lanes, left-to-right:

1. **User terminal**
   - Box: `$ ccx use work`
   - Arrow → ccx CLI box

2. **ccx CLI (a single box, slightly larger)**
   - Inside: list of subcomponents — `profile`, `scanner`, `storage`, `pricing`, `shell`, `server`, `tui`, `doctor`
   - Arrow down → SQLite cylinder (`~/.ccx/state.db`)
   - Arrow up → Browser box (`localhost:7777`)

3. **Upstream claude (a separate box)**
   - Box: `claude` CLI
   - Arrow up to `Anthropic API` cloud (greyed)
   - Annotation: `CLAUDE_CONFIG_DIR` arrow connecting ccx CLI → claude CLI

### Annotations

- Label the arrow from ccx to claude with `CLAUDE_CONFIG_DIR` (this is the whole switching mechanism)
- Label the SQLite cylinder as `incremental cache (rebuilt from JSONL)`
- Label the JSONL source: `~/.claude*/projects/<cwd>/<uuid>.jsonl`

### Export settings

- PNG: 2400×1350 (2x retina), white background
- SVG: same dimensions, transparent background

After exporting, place both at `docs/assets/architecture.png` and `docs/assets/architecture.svg`. README references the PNG.

## `dashboard.png`

Screenshot of the dashboard in dark mode, taken from a 1440×900 viewport with realistic data. Captured during Phase 3 (Polish) after the dashboard is functional.

## `demo.gif`

Rendered from `demo.tape` via `vhs docs/assets/demo.tape`. Rendered fresh during Phase 3 with the working binary.
```

- [ ] **Step 2: Commit**

```bash
git add docs/assets/README.md
git commit -m "docs: document diagram asset requirements"
```

---

## Task 9: Add GitHub issue templates

**Files:**
- Create: `.github/ISSUE_TEMPLATE/bug.yml`
- Create: `.github/ISSUE_TEMPLATE/feature.yml`
- Create: `.github/ISSUE_TEMPLATE/config.yml`

- [ ] **Step 1: Write `.github/ISSUE_TEMPLATE/bug.yml`**

```yaml
name: Bug report
description: Something isn't working as documented.
labels: ["bug"]
body:
  - type: textarea
    id: what
    attributes:
      label: What happened?
      description: A clear, terse description of the bug.
    validations:
      required: true
  - type: textarea
    id: expected
    attributes:
      label: What did you expect to happen?
    validations:
      required: true
  - type: textarea
    id: repro
    attributes:
      label: Steps to reproduce
      description: |
        The minimum commands or actions needed.
      placeholder: |
        1. ccx profile add work --config-dir ...
        2. eval "$(ccx use work)"
        3. ccx usage
    validations:
      required: true
  - type: textarea
    id: doctor
    attributes:
      label: Output of `ccx doctor`
      render: shell
    validations:
      required: true
  - type: textarea
    id: version
    attributes:
      label: Output of `ccx version`
      render: shell
    validations:
      required: true
  - type: dropdown
    id: os
    attributes:
      label: Operating system
      options:
        - macOS (Apple Silicon)
        - macOS (Intel)
        - Linux (amd64)
        - Linux (arm64)
        - Windows
    validations:
      required: true
```

- [ ] **Step 2: Write `.github/ISSUE_TEMPLATE/feature.yml`**

```yaml
name: Feature request
description: Suggest an improvement.
labels: ["enhancement"]
body:
  - type: textarea
    id: problem
    attributes:
      label: What problem does this solve?
      description: Start with the problem, not the proposed solution.
    validations:
      required: true
  - type: textarea
    id: proposal
    attributes:
      label: Proposed solution
      description: Optional. If you have a concrete idea, describe it.
  - type: textarea
    id: alternatives
    attributes:
      label: Alternatives considered
      description: Workarounds you tried, other tools, why they didn't work.
```

- [ ] **Step 3: Write `.github/ISSUE_TEMPLATE/config.yml`**

```yaml
blank_issues_enabled: false
contact_links:
  - name: Anthropic Claude Code bugs
    url: https://github.com/anthropics/claude-code/issues
    about: Bugs in the upstream `claude` CLI go here, not to ccx.
  - name: GitHub Discussions
    url: https://github.com/arafa-dev/ccx/discussions
    about: For questions, ideas, and general discussion (not bugs).
```

- [ ] **Step 4: Commit**

```bash
git add .github/ISSUE_TEMPLATE/
git commit -m "ci: add GitHub issue templates"
```

---

## Task 10: Add markdownlint, cspell, lychee configs

**Files:**
- Create: `.markdownlint.json`
- Create: `.cspell.json`
- Create: `.lycheeignore`

- [ ] **Step 1: Write `.markdownlint.json`**

```json
{
  "default": true,
  "MD013": { "line_length": 120, "code_blocks": false, "tables": false },
  "MD024": { "siblings_only": true },
  "MD033": false,
  "MD041": false
}
```

- [ ] **Step 2: Write `.cspell.json`**

```json
{
  "version": "0.2",
  "language": "en",
  "ignorePaths": [
    "node_modules",
    "dist",
    "web/out",
    "**/*.svg",
    "**/go.sum"
  ],
  "words": [
    "anthropic",
    "arafa",
    "bubbletea",
    "ccusage",
    "ccx",
    "cobra",
    "cosign",
    "fsnotify",
    "gofumpt",
    "goreleaser",
    "lefthook",
    "lipgloss",
    "modernc",
    "nfpm",
    "pelletier",
    "pnpm",
    "pwsh",
    "Recharts",
    "shadcn",
    "sqlite",
    "vhs"
  ]
}
```

- [ ] **Step 3: Write `.lycheeignore`**

```
# Placeholder URLs that won't exist until launch
https://ccx.sh/install
https://github.com/arafa-dev/ccx/releases/latest/.*
https://github.com/arafa-dev/scoop-ccx
https://github.com/arafa-dev/homebrew-ccx
# Trademark links that occasionally rate-limit
https://trademarks.justia.com/.*
```

- [ ] **Step 4: Commit**

```bash
git add .markdownlint.json .cspell.json .lycheeignore
git commit -m "ci: add docs-linting configs"
```

---

## Task 11: Add docs-linting CI workflow

**Files:** Create: `.github/workflows/docs.yml`

- [ ] **Step 1: Write `.github/workflows/docs.yml`**

```yaml
name: Docs

on:
  pull_request:
    paths:
      - "**/*.md"
      - "docs/**"
      - ".markdownlint.json"
      - ".cspell.json"
      - ".lycheeignore"
      - ".github/workflows/docs.yml"
  push:
    branches: [main]
    paths:
      - "**/*.md"
      - "docs/**"

permissions:
  contents: read

concurrency:
  group: docs-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    runs-on: ubuntu-22.04
    steps:
      - uses: actions/checkout@v4
      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version: "20"
      - name: markdownlint
        run: npx --yes markdownlint-cli "**/*.md" --ignore node_modules --ignore web/out
      - name: cspell
        run: npx --yes cspell --no-progress "**/*.md"
      - name: lychee link check
        uses: lycheeverse/lychee-action@v2
        with:
          args: "--no-progress --exclude-file .lycheeignore README.md docs/**/*.md"
          fail: true
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/docs.yml
git commit -m "ci: add docs-linting workflow"
```

---

## Task 12: Run the full docs gate locally

- [ ] **Step 1: markdownlint**

```bash
npx --yes markdownlint-cli "**/*.md" --ignore node_modules --ignore web/out
```

Expected: exit 0, no findings.

- [ ] **Step 2: cspell**

```bash
npx --yes cspell --no-progress "**/*.md"
```

Expected: 0 issues.

- [ ] **Step 3: lychee**

```bash
npx --yes lychee --no-progress --exclude-file .lycheeignore "README.md" "docs/**/*.md"
```

Expected: 0 dead links.

If any step fails, fix the source file and re-run from Step 1 before continuing.

---

## Done definition

- [ ] `README.md` exists at repo root with all 11 sections from spec §10.2
- [ ] `docs/installation.md`, `docs/shell-integration.md`, `docs/architecture.md`, `docs/troubleshooting.md` exist
- [ ] `CHANGELOG.md` has a v0.1.0 entry
- [ ] `docs/assets/demo.tape` exists; `vhs docs/assets/demo.tape` produces a GIF
- [ ] `docs/assets/README.md` documents the architecture diagram requirements
- [ ] `.github/ISSUE_TEMPLATE/{bug,feature,config}.yml` exist
- [ ] `.markdownlint.json`, `.cspell.json`, `.lycheeignore` exist
- [ ] `.github/workflows/docs.yml` exists
- [ ] Local run of markdownlint + cspell + lychee all pass
- [ ] PR opened against `main`; CI green

After merge:

```bash
git tag -a phase-1-A9 -m "Phase 1 A9 (docs) complete"
git push origin phase-1-A9
```
