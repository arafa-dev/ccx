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
