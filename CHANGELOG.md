# Changelog

All notable changes to ccx are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and ccx adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] — 2026-05-25

### Added

- Plan-aware quota tracking for Pro, Max 5x, and Max 20x profiles, including
  5-hour and weekly turn windows.
- `ccx usage --quota` and dashboard quota panels backed by local hook `Stop`
  events.
- Pressure-aware `ccx suggest` scoring with soft/hard quota gates.
- `ccx run` pre-launch profile selection and optional shell wrapper integration.
- `ccx run --supervise` mid-session auto-switching with `claude --resume`.
- Shared Claude history migration via `ccx migrate-shared-history`.
- Daemon recommendation events over `/api/recommendations/live`.

### Changed

- New profiles link `projects/` to `~/.ccx/shared-projects/` for cross-profile
  resume continuity.
- Headroom recommendations now account for configured plan tier, explicit caps,
  quota pressure, recent hook failures, and profile priority.

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

[Unreleased]: https://github.com/arafa-dev/ccx/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/arafa-dev/ccx/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/arafa-dev/ccx/releases/tag/v0.1.0
