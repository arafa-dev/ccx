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
brew install arafa-dev/ccx/ccx

# Windows
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop install ccx

# Debian / Ubuntu
curl -fsSL https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.deb -o /tmp/ccx.deb
sudo dpkg -i /tmp/ccx.deb

# One-liner (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/arafa-dev/ccx/main/install.sh | sh

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

# 6. Keep usage fresh in the background
ccx daemon start
ccx daemon status
```

![dashboard screenshot](docs/assets/dashboard.png)

## Why ccx?

If you run more than one Claude Code account — Pro plus Max, work plus personal,
multiple Pro accounts on a shared machine — you've probably ended up with a
directory of shell aliases that hot-swap `~/.claude`, plus `ccusage` open in
another tab for analytics. Two tools to do one job.

ccx is the one tool. It switches accounts (the part `ccusage` doesn't do) and
tracks usage (the part the bash-alias scripts don't do), all from a single Go
binary.

## How it works

ccx never proxies API calls. The upstream `claude` CLI does what it always did —
ccx just chooses *which* config dir and *which* keychain entry `claude` reads
from, by exporting `CLAUDE_CONFIG_DIR` in your shell. The optional daemon,
hooks, and suggestions all work from local files and the local SQLite cache.

![architecture diagram](docs/assets/architecture.png)

For details, see [`docs/architecture.md`](docs/architecture.md).

## Common usage

Background daemon:

```bash
ccx daemon start              # run local scanner/dashboard API in the background
ccx daemon status             # show pid and URL; add --json for paths
ccx daemon logs --follow      # stream ~/.ccx/daemon.log
ccx daemon restart            # stop and start the daemon
ccx daemon stop
```

Hook telemetry:

```bash
ccx hooks install             # install ccx-managed hooks for all profiles
ccx hooks install --profile work
ccx hooks status
ccx hooks uninstall --profile work
```

Hooks update each profile's `settings.json`, create
`settings.json.ccx-backup-*` files before modifying existing settings, and
record local session telemetry such as starts, stops, failures, and compactions.

Advisory profile suggestions:

```bash
ccx profile set work \
  --daily-tokens 200000 \
  --weekly-tokens 1000000 \
  --monthly-usd 100 \
  --priority 5 \
  --rate-limit-cooldown 5h \
  --suggestions enabled

ccx suggest
ccx suggest --json
```

`ccx suggest` ranks registered profiles by configured budgets, recent usage,
hook failures, cooldowns, and priority. It prints a suggested `ccx use <name>`
command; it does not switch accounts automatically.

## Plan-aware quota

ccx v0.2 adds quota windows for Claude plan tiers. Configure each profile with
its plan tier and optional caps:

```bash
ccx profile set personal --plan-tier pro
ccx profile set max-work --plan-tier max5 --caps-5h-turns 225 --caps-weekly-turns 900
```

Hook telemetry counts completed turns from local `Stop` events, so
`ccx usage --quota` and the dashboard quota panel can show 5-hour and weekly
pressure without proxying Claude traffic.

```bash
ccx usage --quota
ccx dashboard --daemon
```

Suggestions are pressure-aware. Near a soft or hard cap, `ccx suggest` lowers a
profile's score or marks it unavailable, and `ccx run` can pick a healthier
profile before launching Claude:

```bash
ccx run -- claude -p "summarize this repo"
ccx run --supervise -- claude
```

`ccx run --supervise` waits for a completed turn, then relaunches Claude with
`--resume <session-id>` if the daemon reports a better profile. For continuity,
new profiles link their `projects/` directory to `~/.ccx/shared-projects/`.
Existing profiles can opt in safely with a dry run first:

```bash
ccx migrate-shared-history --dry-run
ccx migrate-shared-history
```

## Comparison

| Capability | **ccx** | ccusage | claude-account-switcher | ccs |
| --- | --- | --- | --- | --- |
| Profile switching | ✅ | ❌ | ✅ (bash only) | ✅ |
| Usage analytics | ✅ | ✅ (deepest) | ❌ | partial |
| Local dashboard | ✅ | ❌ | ❌ | ❌ |
| Single binary | ✅ | npm | shell script | TS + Tauri |
| Cross-platform | macOS · Linux · Windows | ✅ | macOS-focused | ✅ |
| Distribution | brew · scoop · apt · curl\|sh | npm | manual | GitHub Releases |

Be honest: `ccusage` has deeper analytics than ccx (per-message drilldowns,
custom date ranges). If analytics is your only need, use ccusage. If you also
juggle accounts, ccx replaces both.

## Configuration

ccx state lives in `~/.ccx/`:

- `~/.ccx/profiles.toml` — registered profile list (human-editable)
- `~/.ccx/state.db` — SQLite cache of parsed events
- `~/.ccx/pricing.yaml` — optional override for the embedded model pricing table

Profile config dirs live wherever you put them (commonly
`~/.claude-profiles/<name>`). On macOS the credential lives in Keychain under a
service name derived from the config dir path — ccx does not touch your
credentials directly.

See [`docs/installation.md`](docs/installation.md) for per-platform setup details.

## Roadmap

Planned, not promised:

- Dashboard refinements on top of the background daemon
- Richer hook-based session analytics
- More transparent headroom scoring and suggestion explanations
- Team workspace primitives, profile sync via git

## Contributing

Issues, bug reports, and PRs welcome. Please read
[`CONTRIBUTING.md`](CONTRIBUTING.md) and
[`docs/conventions.md`](docs/conventions.md) before opening a PR.

## License

[MIT](LICENSE)
