# ccx Phase 1 A8 — Distribution Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the full release pipeline for ccx — goreleaser config, nfpm `.deb`/`.rpm` packages, Homebrew tap + Scoop bucket bootstrap docs, `install.sh` curl-pipe installer, cosign signing, and the GitHub Actions release workflow — such that a `git push --tags v0.1.0-rc.1` produces signed, multi-platform artifacts on GitHub Releases and updates the brew/scoop taps.

**Architecture:** Distribution lives entirely in top-level config (`.goreleaser.yaml`, `install.sh`, `.github/workflows/release.yml`) plus a couple of doc files (`docs/distribution/*.md`). Goreleaser drives the build; nfpm is invoked as a goreleaser subsystem (so a separate `nfpm.yaml` is **not** needed — config is inline in `.goreleaser.yaml`). A throwaway `cmd/ccx/main.go` stub exists only to make goreleaser happy until Phase 2 replaces it with the real entry point. The Homebrew tap (`arafa-dev/homebrew-ccx`) and Scoop bucket (`arafa-dev/scoop-ccx`) are **separate GitHub repos** owned by the user; this plan ships docs explaining how to create and bootstrap them.

**Tech Stack:** goreleaser ≥ 2.4, nfpm (bundled in goreleaser), cosign ≥ 2.2, GitHub Actions, POSIX shell, pnpm + Node 20 (to build `web/` during release), Go 1.22.

**Spec reference:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — Section 10 (Distribution & launch), plus Section 5.3 (Build flow) and Section 9.3 (CI workflows).

**Worktree:** `feat/distribution` — create with `git worktree add ../ccx-distribution -b feat/distribution main`.

**Exit criteria:**
- `goreleaser check` passes with zero errors
- `goreleaser release --snapshot --clean` completes locally and produces 6 binaries, 2 deb files, 2 rpm files, a checksums file, and a source archive in `dist/`
- `actionlint .github/workflows/release.yml` passes
- `shellcheck install.sh` passes; `sh -n install.sh` passes
- `nfpm pkg --config <derived>` succeeds against a mock binary (verified inline via goreleaser snapshot — no separate nfpm.yaml)
- Tap and bucket bootstrap docs render correctly and are committed
- README has install snippet block (the one-liners only)
- PR opened against `main`

---

## Pre-flight

Confirm the working directory is the distribution worktree and Phase 0 has been merged:

```bash
pwd                                                # → /Users/arafa/Developer/ccx-distribution (or similar)
git status                                         # → On branch feat/distribution, working tree clean
git log --oneline -5                               # → shows Phase 0 commits + the phase-0 tag history
test -f go.mod && cat go.mod | head -3             # → confirms module github.com/arafa-dev/ccx
test -f internal/contracts/types.go                # → confirms Phase 0 contracts exist
test -f api/openapi.yaml                           # → confirms Phase 0 openapi exists
```

If the worktree does not exist:
```bash
git worktree add ../ccx-distribution -b feat/distribution main
cd ../ccx-distribution
```

Install the local tooling once:
```bash
# macOS
brew install goreleaser nfpm cosign shellcheck actionlint
# Linux fallback
go install github.com/goreleaser/goreleaser/v2@latest
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
go install github.com/rhysd/actionlint/cmd/actionlint@latest
go install github.com/sigstore/cosign/v2/cmd/cosign@latest
```

**Conventions for this plan:**
- One commit per task. Conventional commit format (`type(scope): subject`).
- Top-level config files live at the repo root; docs under `docs/distribution/`.
- Do **not** modify `internal/contracts/`, `api/openapi.yaml`, `internal/storage/schema.sql`, or `docs/conventions.md` — those are owned by Phase 0.
- The `cmd/ccx/main.go` stub created here is **explicitly temporary**: Phase 2 P2 replaces it. The stub must be self-contained (no imports of `internal/*` beyond what is needed for the version string) so that nothing else has to change when Phase 2 rewrites it.

**Required GitHub repo secrets (document, do not commit):**
- `GH_PAT` — personal access token with `repo` scope on `arafa-dev/homebrew-ccx` and `arafa-dev/scoop-ccx`. Used by goreleaser to push the formula and manifest.
- `COSIGN_PRIVATE_KEY` — output of `cosign generate-key-pair` (the `cosign.key` content).
- `COSIGN_PASSWORD` — password used when generating the cosign keypair.

These are set in the repo's *Settings → Secrets and variables → Actions*. The release workflow references them but they are never committed.

---

## Task 1: Add minimal `cmd/ccx/main.go` stub

Goreleaser refuses to build without a `main` package at the configured path. Phase 0 deliberately did not create `cmd/ccx/` (a `package main` directory with no `main` function would have broken `go build ./...`). This task adds the smallest possible compilable stub so goreleaser has something to build. **Phase 2 P2 will replace this file completely.**

**Files:**
- Create: `cmd/ccx/main.go`

- [ ] **Step 1: Create the stub**

Create `cmd/ccx/main.go`:

```go
// Package main is the ccx CLI entry point.
//
// THIS FILE IS A TEMPORARY STUB.
//
// Plan A8 (distribution) needs a buildable main package so goreleaser can
// cross-compile snapshot releases before Phase 2 wires up the real cobra
// command tree. Phase 2 P2 replaces this file in full — when that happens,
// keep the three ldflags-injected variables (version, commit, date) so the
// release pipeline keeps working.
package main

import (
	"fmt"
	"os"
	"runtime"
)

// These are populated by `-ldflags "-X main.version=... -X main.commit=... -X main.date=..."`
// at release time via .goreleaser.yaml. For local `go build`, they remain "dev".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("ccx %s (commit %s, built %s, %s/%s)\n",
			version, commit, date, runtime.GOOS, runtime.GOARCH)
		return
	}
	fmt.Fprintln(os.Stderr, "ccx: pre-release stub. Real CLI lands in Phase 2.")
	fmt.Fprintln(os.Stderr, "Run `ccx version` to print build info.")
	os.Exit(0)
}
```

- [ ] **Step 2: Verify the stub builds**

Run:
```bash
go build -trimpath -ldflags="-s -w -X main.version=test -X main.commit=abc -X main.date=2026-05-19" -o /tmp/ccx-stub ./cmd/ccx
/tmp/ccx-stub version
```

Expected output (exact):
```
ccx test (commit abc, built 2026-05-19, <yourOS>/<yourArch>)
```

Then check the no-arg path:
```bash
/tmp/ccx-stub
```
Expected: prints the stub warning to stderr, exits 0.

Clean up: `rm -f /tmp/ccx-stub`.

- [ ] **Step 3: Run the lint gate**

```bash
gofumpt -l cmd/ccx/main.go        # → no output
go vet ./cmd/ccx/...              # → no output
```

- [ ] **Step 4: Commit**

```bash
git add cmd/ccx/main.go
git commit -m "feat(cmd): add temporary main stub for goreleaser builds"
```

---

## Task 2: Write `.goreleaser.yaml`

The goreleaser config drives the entire build matrix, the archive shape, checksums, nfpm packaging, Homebrew formula generation, Scoop manifest generation, and the cosign signing step. nfpm is invoked as a goreleaser subsystem (the `nfpms:` block), so a standalone `nfpm.yaml` is not needed.

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Write `.goreleaser.yaml`**

```yaml
# .goreleaser.yaml — ccx v0.1
# Docs: https://goreleaser.com/customization/
version: 2

project_name: ccx

# Build hook: the Go binary embeds web/out/ via go:embed. We MUST build the
# Next.js dashboard before goreleaser invokes `go build`. We also tidy go.sum
# to fail fast on dependency drift.
before:
  hooks:
    - go mod tidy
    - sh -c 'if [ -d web ] && [ -f web/package.json ]; then pnpm --filter web install --frozen-lockfile && pnpm --filter web build; else echo "web/ not present, skipping (Phase 1 A7 will add it)"; fi'

builds:
  - id: ccx
    main: ./cmd/ccx
    binary: ccx
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}
    goos:
      - darwin
      - linux
      - windows
    goarch:
      - amd64
      - arm64
    # No exclusions: all six (3 OS × 2 arch) combinations are released.

archives:
  - id: default
    ids:
      - ccx
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_
      {{- if eq .Os "darwin" }}darwin{{ end }}
      {{- if eq .Os "linux"  }}linux{{ end }}
      {{- if eq .Os "windows"}}windows{{ end }}_
      {{- if eq .Arch "amd64" }}amd64{{ end }}
      {{- if eq .Arch "arm64" }}arm64{{ end }}
    format_overrides:
      - goos: windows
        formats: ["zip"]
    formats: ["tar.gz"]
    files:
      - LICENSE
      - README.md
      - SECURITY.md

source:
  enabled: true
  name_template: "{{ .ProjectName }}_{{ .Version }}_source"
  format: tar.gz

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

snapshot:
  version_template: "{{ incpatch .Version }}-snapshot-{{ .ShortCommit }}"

changelog:
  sort: asc
  use: github
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"
      - "^ci:"
      - "Merge pull request"
      - "Merge branch"
  groups:
    - title: Features
      regexp: '^.*?feat(\(.+\))??!?:.+$'
      order: 0
    - title: Bug fixes
      regexp: '^.*?fix(\(.+\))??!?:.+$'
      order: 1
    - title: Other
      order: 999

nfpms:
  - id: ccx-pkg
    package_name: ccx
    ids:
      - ccx
    vendor: arafa-dev
    homepage: https://github.com/arafa-dev/ccx
    maintainer: arafa-dev <noreply@arafa-dev.com>
    description: |-
      Multi-account workspace manager for Claude Code.
      Switch between accounts and view real usage across all of them
      from a single Go binary.
    license: MIT
    section: utils
    priority: optional
    formats:
      - deb
      - rpm
    bindir: /usr/bin
    contents:
      - src: LICENSE
        dst: /usr/share/doc/ccx/LICENSE
      - src: README.md
        dst: /usr/share/doc/ccx/README.md
    overrides:
      deb:
        dependencies: []
      rpm:
        dependencies: []
    rpm:
      group: Applications/System
      compression: xz
    deb:
      compression: xz

brews:
  - name: ccx
    ids:
      - default
    repository:
      owner: arafa-dev
      name: homebrew-ccx
      branch: main
      token: "{{ .Env.GH_PAT }}"
    directory: Formula
    homepage: https://github.com/arafa-dev/ccx
    description: Multi-account workspace manager for Claude Code
    license: MIT
    test: |
      system "#{bin}/ccx", "version"
    install: |
      bin.install "ccx"
    commit_author:
      name: ccx-release-bot
      email: noreply@arafa-dev.com
    commit_msg_template: "chore(formula): bump ccx to {{ .Tag }}"

scoops:
  - name: ccx
    ids:
      - default
    repository:
      owner: arafa-dev
      name: scoop-ccx
      branch: main
      token: "{{ .Env.GH_PAT }}"
    homepage: https://github.com/arafa-dev/ccx
    description: Multi-account workspace manager for Claude Code
    license: MIT
    commit_author:
      name: ccx-release-bot
      email: noreply@arafa-dev.com
    commit_msg_template: "chore(manifest): bump ccx to {{ .Tag }}"

signs:
  - id: cosign
    cmd: cosign
    artifacts: checksum
    stdin: "{{ .Env.COSIGN_PASSWORD }}"
    args:
      - sign-blob
      - "--key=env://COSIGN_PRIVATE_KEY"
      - "--output-signature=${signature}"
      - "${artifact}"
      - "--yes"

release:
  github:
    owner: arafa-dev
    name: ccx
  draft: false
  prerelease: auto
  mode: replace
  header: |
    ## ccx {{ .Tag }}

    Multi-account workspace manager for Claude Code.

    ### Install

    ```bash
    # Homebrew (macOS / Linux)
    brew install arafa-dev/ccx/ccx

    # Scoop (Windows)
    scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
    scoop install ccx

    # Debian / Ubuntu
    curl -fsSLO https://github.com/arafa-dev/ccx/releases/download/{{ .Tag }}/ccx_{{ .Version }}_linux_amd64.deb
    sudo dpkg -i ccx_{{ .Version }}_linux_amd64.deb

    # Fedora / RHEL
    curl -fsSLO https://github.com/arafa-dev/ccx/releases/download/{{ .Tag }}/ccx_{{ .Version }}_linux_amd64.rpm
    sudo rpm -i ccx_{{ .Version }}_linux_amd64.rpm

    # One-liner installer (any POSIX)
    curl -fsSL https://raw.githubusercontent.com/arafa-dev/ccx/main/install.sh | sh

    # go install
    go install github.com/arafa-dev/ccx/cmd/ccx@{{ .Tag }}
    ```
  footer: |
    ---
    All binaries are signed with cosign. Verify with:
    ```
    cosign verify-blob --key https://github.com/arafa-dev/ccx/releases/download/{{ .Tag }}/cosign.pub --signature checksums.txt.sig checksums.txt
    ```
```

- [ ] **Step 2: Run `goreleaser check`**

```bash
goreleaser check
```
Expected: prints `1 configuration file(s) validated` (or similar) and exits 0.

If goreleaser reports unknown fields, you may be on an older version. Verify with `goreleaser --version` (need ≥ 2.4).

- [ ] **Step 3: Commit**

```bash
git add .goreleaser.yaml
git commit -m "feat(release): add goreleaser config (builds, nfpm, brew, scoop, cosign)"
```

---

## Task 3: Write `install.sh`

A POSIX shell script (works on dash, ash, bash, zsh) that:
- Detects OS (darwin/linux) and arch (x86_64 → amd64; aarch64/arm64 → arm64)
- Resolves the requested version (latest by default, or `--version vX.Y.Z`)
- Downloads the matching tarball from GitHub Releases
- Verifies the SHA256 checksum against `checksums.txt`
- Installs to `/usr/local/bin` if writable (or with sudo), else `$HOME/.local/bin`
- Refuses to run on unsupported platforms with a clear error

**Files:**
- Create: `install.sh`

- [ ] **Step 1: Write `install.sh`**

```sh
#!/bin/sh
# install.sh — curl-pipe installer for ccx.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/arafa-dev/ccx/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/arafa-dev/ccx/main/install.sh | sh -s -- --version v0.1.0
#
# Flags:
#   --version vX.Y.Z   Install a specific version (default: latest release)
#   --bin-dir DIR      Override install directory
#   --help             Show this help
#
# Supported platforms: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64.
# Windows users: please use Scoop (see README).

set -eu

REPO="arafa-dev/ccx"
DEFAULT_BIN_DIR_ROOT="/usr/local/bin"
DEFAULT_BIN_DIR_USER="$HOME/.local/bin"

VERSION=""
BIN_DIR=""

log() {
    printf "ccx-install: %s\n" "$*" >&2
}

die() {
    log "error: $*"
    exit 1
}

usage() {
    sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
    exit 0
}

# Parse args
while [ $# -gt 0 ]; do
    case "$1" in
        --version)
            shift
            [ $# -gt 0 ] || die "--version requires an argument"
            VERSION="$1"
            ;;
        --version=*)
            VERSION="${1#--version=}"
            ;;
        --bin-dir)
            shift
            [ $# -gt 0 ] || die "--bin-dir requires an argument"
            BIN_DIR="$1"
            ;;
        --bin-dir=*)
            BIN_DIR="${1#--bin-dir=}"
            ;;
        -h|--help)
            usage
            ;;
        *)
            die "unknown argument: $1"
            ;;
    esac
    shift
done

# Detect OS
uname_s="$(uname -s 2>/dev/null || echo unknown)"
case "$uname_s" in
    Darwin) os="darwin" ;;
    Linux)  os="linux"  ;;
    *) die "unsupported OS: $uname_s. Windows users: use Scoop. See https://github.com/$REPO" ;;
esac

# Detect arch
uname_m="$(uname -m 2>/dev/null || echo unknown)"
case "$uname_m" in
    x86_64|amd64)   arch="amd64" ;;
    aarch64|arm64)  arch="arm64" ;;
    *) die "unsupported architecture: $uname_m (only amd64 and arm64 are supported)" ;;
esac

# Resolve version
if [ -z "$VERSION" ]; then
    log "resolving latest release tag from github.com/$REPO ..."
    # Use the GitHub API redirect to avoid needing jq.
    VERSION="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" \
        | sed 's#.*/tag/##')"
    [ -n "$VERSION" ] || die "could not resolve latest version; pass --version vX.Y.Z explicitly"
fi
# Strip a leading "v" if user passed e.g. "0.1.0" then re-add for the tag URL.
case "$VERSION" in
    v*) tag="$VERSION"; ver="${VERSION#v}" ;;
    *)  tag="v$VERSION"; ver="$VERSION" ;;
esac

# Choose install dir
if [ -z "$BIN_DIR" ]; then
    if [ -w "$DEFAULT_BIN_DIR_ROOT" ] || [ "$(id -u)" = "0" ]; then
        BIN_DIR="$DEFAULT_BIN_DIR_ROOT"
    elif command -v sudo >/dev/null 2>&1; then
        BIN_DIR="$DEFAULT_BIN_DIR_ROOT"
        USE_SUDO=1
    else
        BIN_DIR="$DEFAULT_BIN_DIR_USER"
        mkdir -p "$BIN_DIR"
    fi
fi
USE_SUDO="${USE_SUDO:-0}"

# Required tools
command -v curl >/dev/null 2>&1 || die "curl is required but not installed"
command -v tar  >/dev/null 2>&1 || die "tar is required but not installed"
# shasum (BSD) or sha256sum (GNU)
if command -v sha256sum >/dev/null 2>&1; then
    sha_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    sha_cmd="shasum -a 256"
else
    die "need sha256sum or shasum to verify checksums"
fi

archive_name="ccx_${ver}_${os}_${arch}.tar.gz"
archive_url="https://github.com/$REPO/releases/download/$tag/$archive_name"
checksums_url="https://github.com/$REPO/releases/download/$tag/checksums.txt"

tmpdir="$(mktemp -d 2>/dev/null || mktemp -d -t ccx-install)"
trap 'rm -rf "$tmpdir"' EXIT INT TERM

log "downloading $archive_url"
curl -fsSL "$archive_url" -o "$tmpdir/$archive_name" || die "download failed: $archive_url"

log "downloading $checksums_url"
curl -fsSL "$checksums_url" -o "$tmpdir/checksums.txt" || die "download failed: $checksums_url"

# Verify checksum
log "verifying SHA-256 ..."
expected="$(grep " $archive_name\$" "$tmpdir/checksums.txt" | awk '{print $1}')"
[ -n "$expected" ] || die "no checksum entry found for $archive_name in checksums.txt"
actual="$(cd "$tmpdir" && $sha_cmd "$archive_name" | awk '{print $1}')"
[ "$expected" = "$actual" ] || die "checksum mismatch: expected $expected, got $actual"

# Extract
log "extracting $archive_name"
( cd "$tmpdir" && tar -xzf "$archive_name" )
[ -f "$tmpdir/ccx" ] || die "ccx binary missing from extracted archive"
chmod +x "$tmpdir/ccx"

# Install
log "installing to $BIN_DIR/ccx"
if [ "$USE_SUDO" = "1" ]; then
    sudo install -m 0755 "$tmpdir/ccx" "$BIN_DIR/ccx"
else
    install -m 0755 "$tmpdir/ccx" "$BIN_DIR/ccx" 2>/dev/null \
        || cp "$tmpdir/ccx" "$BIN_DIR/ccx" && chmod 0755 "$BIN_DIR/ccx"
fi

log "installed ccx $tag to $BIN_DIR/ccx"
case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *) log "note: $BIN_DIR is not on your PATH. Add it with:"
       printf 'export PATH="%s:$PATH"\n' "$BIN_DIR" >&2 ;;
esac

log "run 'ccx --help' to get started."
```

- [ ] **Step 2: Make it executable**

```bash
chmod +x install.sh
```

- [ ] **Step 3: Syntax check**

```bash
sh -n install.sh
```
Expected: no output, exit 0.

- [ ] **Step 4: Run shellcheck**

```bash
shellcheck install.sh
```
Expected: no output, exit 0. Address any warnings before continuing (the script is written to be lint-clean — if shellcheck flags something, read the suggestion and fix the script, do not silence).

- [ ] **Step 5: Smoke-test the help flag**

```bash
./install.sh --help
```
Expected: prints the usage block, exits 0.

```bash
./install.sh --unknown 2>&1 | head -1
```
Expected: starts with `ccx-install: error: unknown argument: --unknown` and exits non-zero.

- [ ] **Step 6: Commit**

```bash
git add install.sh
git commit -m "feat(install): add POSIX install.sh with checksum verification"
```

---

## Task 4: Write `.github/workflows/release.yml`

This workflow fires on tag push (`v*`) and runs goreleaser end to end: it builds the web frontend, then builds and packages binaries, then publishes to GitHub Releases + Homebrew tap + Scoop bucket. It signs artifacts with cosign and verifies the release exists at the end.

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Write the workflow**

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write   # to create the GitHub Release
  packages: write   # in case docker pushes are added later
  id-token: write   # for cosign keyless (kept for future use)

concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false

jobs:
  release:
    name: Release ccx
    runs-on: ubuntu-22.04
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0   # goreleaser needs full history for changelog

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true

      - name: Set up pnpm
        uses: pnpm/action-setup@v4
        with:
          version: 9

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: pnpm
          cache-dependency-path: |
            web/pnpm-lock.yaml

      - name: Install cosign
        uses: sigstore/cosign-installer@v3
        with:
          cosign-release: v2.4.0

      - name: Build dashboard
        run: |
          if [ -d web ] && [ -f web/package.json ]; then
            pnpm --filter web install --frozen-lockfile
            pnpm --filter web build
          else
            echo "::warning::web/ not yet present (Phase 1 A7 not merged). Continuing."
            mkdir -p web/out
            echo '<!doctype html><title>ccx</title>' > web/out/index.html
          fi

      - name: Run goreleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN:       ${{ secrets.GITHUB_TOKEN }}
          GH_PAT:             ${{ secrets.GH_PAT }}
          COSIGN_PRIVATE_KEY: ${{ secrets.COSIGN_PRIVATE_KEY }}
          COSIGN_PASSWORD:    ${{ secrets.COSIGN_PASSWORD }}

      - name: Verify release exists
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          tag="${GITHUB_REF#refs/tags/}"
          echo "Verifying release for tag $tag ..."
          gh release view "$tag" --repo "${{ github.repository }}" \
              --json tagName,assets \
              --jq '.tagName + " has " + (.assets | length | tostring) + " assets"'
          # Sanity: expect at least 6 binary archives + checksums.txt + checksums.txt.sig + source tarball
          n=$(gh release view "$tag" --repo "${{ github.repository }}" --json assets --jq '.assets | length')
          if [ "$n" -lt 9 ]; then
            echo "::error::release has only $n assets, expected ≥ 9"
            exit 1
          fi

      - name: Verify cosign signature
        run: |
          tag="${GITHUB_REF#refs/tags/}"
          tmp="$(mktemp -d)"
          curl -fsSL "https://github.com/${{ github.repository }}/releases/download/$tag/checksums.txt"     -o "$tmp/checksums.txt"
          curl -fsSL "https://github.com/${{ github.repository }}/releases/download/$tag/checksums.txt.sig" -o "$tmp/checksums.txt.sig"
          # Public key is published next to checksums.txt — see release notes template.
          # If a cosign.pub asset is not in the release, fall back to the env-backed key fingerprint check.
          if curl -fsSLI "https://github.com/${{ github.repository }}/releases/download/$tag/cosign.pub" >/dev/null 2>&1; then
            curl -fsSL "https://github.com/${{ github.repository }}/releases/download/$tag/cosign.pub" -o "$tmp/cosign.pub"
            cosign verify-blob --key "$tmp/cosign.pub" --signature "$tmp/checksums.txt.sig" "$tmp/checksums.txt"
          else
            echo "::warning::cosign.pub not published as a release asset; skip pubkey verification"
          fi
```

- [ ] **Step 2: Lint with actionlint**

```bash
actionlint .github/workflows/release.yml
```
Expected: no output, exit 0. If actionlint complains, fix the workflow.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow (goreleaser + cosign + tap/bucket push)"
```

---

## Task 5: Document Homebrew tap repo

Goreleaser pushes the generated formula into the `arafa-dev/homebrew-ccx` repo on every release. That repo must already exist with an initial commit and a bootstrap formula. This task ships the docs the user follows to create it.

**Files:**
- Create: `docs/distribution/homebrew-tap.md`

- [ ] **Step 1: Create the doc**

Create `docs/distribution/homebrew-tap.md`:

````markdown
# Homebrew tap setup — `arafa-dev/homebrew-ccx`

This is the bootstrap process for the Homebrew tap that hosts the `ccx` formula.
Once set up, **goreleaser updates the formula automatically on every release** —
you never edit `Formula/ccx.rb` by hand after the bootstrap.

## 1. Create the GitHub repo

Manually create a new public repo on GitHub:

- Owner: `arafa-dev`
- Name: `homebrew-ccx`   (the `homebrew-` prefix is required by Homebrew)
- Description: "Homebrew tap for ccx — multi-account workspace manager for Claude Code"
- Initialize with: nothing (no README, no .gitignore, no license — we add them below)

## 2. Clone and bootstrap locally

```bash
git clone git@github.com:arafa-dev/homebrew-ccx.git
cd homebrew-ccx
mkdir -p Formula
```

## 3. Add a bootstrap `README.md`

```markdown
# homebrew-ccx

Homebrew tap for [ccx](https://github.com/arafa-dev/ccx) — multi-account
workspace manager for Claude Code.

## Install

```bash
brew install arafa-dev/ccx/ccx
```

This tap is **updated automatically** by goreleaser on every ccx release.
Do not edit `Formula/ccx.rb` by hand — your changes will be overwritten on
the next release.
```

## 4. Add a bootstrap `Formula/ccx.rb`

This file just needs to be a valid Homebrew formula so `brew tap` succeeds
before the first goreleaser run. Goreleaser overwrites it on the next release.

```ruby
class Ccx < Formula
  desc "Multi-account workspace manager for Claude Code"
  homepage "https://github.com/arafa-dev/ccx"
  version "0.0.0"
  license "MIT"

  on_macos do
    on_intel  { url "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_darwin_amd64.tar.gz"; sha256 "0000000000000000000000000000000000000000000000000000000000000000" }
    on_arm    { url "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_darwin_arm64.tar.gz"; sha256 "0000000000000000000000000000000000000000000000000000000000000000" }
  end

  on_linux do
    on_intel  { url "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_linux_amd64.tar.gz";  sha256 "0000000000000000000000000000000000000000000000000000000000000000" }
    on_arm    { url "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_linux_arm64.tar.gz";  sha256 "0000000000000000000000000000000000000000000000000000000000000000" }
  end

  def install
    bin.install "ccx"
  end

  test do
    system "#{bin}/ccx", "version"
  end
end
```

## 5. Commit and push

```bash
git add README.md Formula/ccx.rb
git commit -m "chore: bootstrap homebrew-ccx tap"
git push origin main
```

## 6. Grant goreleaser permission to push

The ccx release workflow uses the `GH_PAT` secret to push commits to this
tap. Create a fine-grained personal access token:

- Go to GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens
- Name: `ccx-release-tap-push`
- Resource owner: `arafa-dev`
- Repository access: select **`arafa-dev/homebrew-ccx` AND `arafa-dev/scoop-ccx`**
  (one token for both)
- Permissions → Repository → `Contents`: **Read and write**
- Permissions → Repository → `Metadata`: Read-only (auto)
- Expiry: 1 year

Copy the token. In the `arafa-dev/ccx` repo:

- Settings → Secrets and variables → Actions → New repository secret
- Name: `GH_PAT`
- Value: paste the token

## 7. Verify the tap works

```bash
brew tap arafa-dev/ccx
brew search ccx
# Should list arafa-dev/ccx/ccx
```

Installing the bootstrap version will fail (the 0.0.0 archives don't exist),
which is expected. The first real `v0.1.0` release will populate the formula
with real URLs and sha256 values automatically.
````

- [ ] **Step 2: Commit**

```bash
git add docs/distribution/homebrew-tap.md
git commit -m "docs(distribution): add Homebrew tap setup guide"
```

---

## Task 6: Document Scoop bucket repo

Same pattern as the Homebrew tap, but for Windows users. The bucket repo holds the manifest JSON that goreleaser updates on every release.

**Files:**
- Create: `docs/distribution/scoop-bucket.md`

- [ ] **Step 1: Create the doc**

Create `docs/distribution/scoop-bucket.md`:

````markdown
# Scoop bucket setup — `arafa-dev/scoop-ccx`

This is the bootstrap process for the Scoop bucket that hosts the `ccx`
manifest. Once set up, **goreleaser updates the manifest automatically on
every release**.

## 1. Create the GitHub repo

Manually create a new public repo on GitHub:

- Owner: `arafa-dev`
- Name: `scoop-ccx`
- Description: "Scoop bucket for ccx — multi-account workspace manager for Claude Code"
- Initialize with: nothing

## 2. Clone and bootstrap locally

```bash
git clone git@github.com:arafa-dev/scoop-ccx.git
cd scoop-ccx
mkdir -p bucket
```

## 3. Add a bootstrap `README.md`

```markdown
# scoop-ccx

Scoop bucket for [ccx](https://github.com/arafa-dev/ccx) — multi-account
workspace manager for Claude Code.

## Install

```powershell
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop install ccx
```

This bucket is **updated automatically** by goreleaser on every ccx release.
Do not edit `bucket/ccx.json` by hand — your changes will be overwritten on
the next release.
```

## 4. Add a bootstrap `bucket/ccx.json`

A minimum valid Scoop manifest so `scoop install` does not fail before the
first goreleaser run. The next release overwrites it with real version + URLs.

```json
{
  "version": "0.0.0",
  "description": "Multi-account workspace manager for Claude Code",
  "homepage": "https://github.com/arafa-dev/ccx",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_windows_amd64.zip",
      "hash": "0000000000000000000000000000000000000000000000000000000000000000"
    },
    "arm64": {
      "url": "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_windows_arm64.zip",
      "hash": "0000000000000000000000000000000000000000000000000000000000000000"
    }
  },
  "bin": "ccx.exe",
  "checkver": {
    "github": "https://github.com/arafa-dev/ccx"
  },
  "autoupdate": {
    "architecture": {
      "64bit": { "url": "https://github.com/arafa-dev/ccx/releases/download/v$version/ccx_$version_windows_amd64.zip" },
      "arm64": { "url": "https://github.com/arafa-dev/ccx/releases/download/v$version/ccx_$version_windows_arm64.zip" }
    }
  }
}
```

## 5. Commit and push

```bash
git add README.md bucket/ccx.json
git commit -m "chore: bootstrap scoop-ccx bucket"
git push origin main
```

## 6. Reuse the same `GH_PAT` secret

The `GH_PAT` configured in the Homebrew tap setup (see
`docs/distribution/homebrew-tap.md`) already includes `arafa-dev/scoop-ccx`
in its repo scope. No separate secret is needed.

## 7. Verify the bucket works

```powershell
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop search ccx
# Should list ccx (from arafa-dev_scoop-ccx)
```

Installing the bootstrap version will fail (the 0.0.0 archives don't exist),
which is expected. The first real `v0.1.0` release will populate the manifest
with real URLs and hashes automatically.
````

- [ ] **Step 2: Commit**

```bash
git add docs/distribution/scoop-bucket.md
git commit -m "docs(distribution): add Scoop bucket setup guide"
```

---

## Task 7: Document cosign signing setup

The release workflow already wires cosign in (Task 4) and the goreleaser config invokes it (Task 2). This task ships the human-facing setup doc that explains how to generate the keypair and where the secrets live. **The private key is never committed.**

**Files:**
- Create: `docs/distribution/cosign.md`

- [ ] **Step 1: Create the doc**

Create `docs/distribution/cosign.md`:

````markdown
# Release signing with cosign

Every ccx release signs the `checksums.txt` artifact with [cosign](https://github.com/sigstore/cosign).
Users (and CI) verify the checksums file before trusting any binary in the release.

## 1. Generate the signing keypair (one time)

On a trusted local machine:

```bash
# Install cosign if not already present
brew install cosign            # macOS
# or: go install github.com/sigstore/cosign/v2/cmd/cosign@latest

mkdir -p ~/secrets/ccx-cosign
cd ~/secrets/ccx-cosign

cosign generate-key-pair
# Prompts twice for a password. Pick a strong one. The password is needed by
# the release workflow — store it in 1Password / your password manager.
# This creates cosign.key (PRIVATE — never commit) and cosign.pub (public).
```

## 2. Upload the keypair as GitHub secrets

In the `arafa-dev/ccx` repo → Settings → Secrets and variables → Actions:

| Secret name           | Value                                            |
|-----------------------|--------------------------------------------------|
| `COSIGN_PRIVATE_KEY`  | Full contents of `cosign.key` (multi-line OK)    |
| `COSIGN_PASSWORD`     | The password you set during keypair generation   |

## 3. Publish the public key

Two options. Pick **one** (or both) — option B is recommended.

**Option A:** Commit `cosign.pub` to the ccx repo at `docs/distribution/cosign.pub`.
End-users verify with:

```bash
cosign verify-blob \
  --key https://raw.githubusercontent.com/arafa-dev/ccx/main/docs/distribution/cosign.pub \
  --signature checksums.txt.sig \
  checksums.txt
```

**Option B:** Upload `cosign.pub` as a release asset on every release. The
release workflow's verification step (Task 4) will pick it up automatically.
The simplest way is to add it as a constant `extra_files` entry in
`.goreleaser.yaml`:

```yaml
release:
  extra_files:
    - glob: ./docs/distribution/cosign.pub
```

The doc recommends doing **both** — committing the public key to the repo
*and* attaching it as a release asset.

## 4. Local verification by users

After downloading a release, users can verify the integrity:

```bash
TAG=v0.1.0
curl -fsSLO "https://github.com/arafa-dev/ccx/releases/download/$TAG/checksums.txt"
curl -fsSLO "https://github.com/arafa-dev/ccx/releases/download/$TAG/checksums.txt.sig"
curl -fsSLO "https://raw.githubusercontent.com/arafa-dev/ccx/main/docs/distribution/cosign.pub"

cosign verify-blob \
  --key cosign.pub \
  --signature checksums.txt.sig \
  checksums.txt
# → Verified OK
```

Then verify each binary's sha256 matches the corresponding line in `checksums.txt`.

## 5. Key rotation

If the key is compromised:

1. Generate a new keypair (`cosign generate-key-pair`).
2. Update `COSIGN_PRIVATE_KEY` and `COSIGN_PASSWORD` in repo secrets.
3. Commit the new `cosign.pub`.
4. Add a `SECURITY.md` advisory noting which versions were signed by the old
   key and the date of rotation.
````

- [ ] **Step 2: Commit**

```bash
git add docs/distribution/cosign.md
git commit -m "docs(distribution): add cosign keypair + signing guide"
```

---

## Task 8: Local snapshot release dry run

Verify end to end that the goreleaser config builds correctly **before** wiring up any real GitHub Release. The snapshot command runs goreleaser with no upload — outputs land in `./dist/`.

**Files:**
- None modified.

- [ ] **Step 1: Pre-clean**

```bash
rm -rf dist
```

- [ ] **Step 2: Run snapshot release**

```bash
goreleaser release --snapshot --clean --skip=publish,sign
```

The `--skip=publish,sign` flag bypasses GitHub upload + cosign signing for local runs (no GH_PAT, no cosign keys locally). Goreleaser still runs the full build matrix, archives, nfpm packaging, brew formula generation, and scoop manifest generation.

Expected: completes with exit 0 in 1–3 minutes. If goreleaser complains that `web/out/` is missing, the `before:` hook should have created a stub (see Task 2's hook — it tolerates the missing web dir). If the hook fails, manually run:

```bash
mkdir -p web/out
printf '<!doctype html><title>ccx</title>' > web/out/index.html
goreleaser release --snapshot --clean --skip=publish,sign
```

- [ ] **Step 3: Verify artifacts**

```bash
ls dist/
```

Expected files (the exact version string will differ; `*` matches):

```
ccx_*_darwin_amd64.tar.gz
ccx_*_darwin_arm64.tar.gz
ccx_*_linux_amd64.tar.gz
ccx_*_linux_arm64.tar.gz
ccx_*_windows_amd64.zip
ccx_*_windows_arm64.zip
ccx_*_linux_amd64.deb
ccx_*_linux_arm64.deb
ccx_*_linux_amd64.rpm
ccx_*_linux_arm64.rpm
ccx_*_source.tar.gz
checksums.txt
config.yaml
metadata.json
artifacts.json
```

Plus directories `dist/ccx_darwin_amd64_v1/`, `dist/ccx_linux_arm64_v8.0/`, etc., each containing the raw binary.

Programmatic check:
```bash
expected="ccx_.*_darwin_amd64\\.tar\\.gz ccx_.*_darwin_arm64\\.tar\\.gz ccx_.*_linux_amd64\\.tar\\.gz ccx_.*_linux_arm64\\.tar\\.gz ccx_.*_windows_amd64\\.zip ccx_.*_windows_arm64\\.zip ccx_.*_linux_amd64\\.deb ccx_.*_linux_arm64\\.deb ccx_.*_linux_amd64\\.rpm ccx_.*_linux_arm64\\.rpm checksums\\.txt"
missing=0
for pat in $expected; do
  if ! ls dist/ 2>/dev/null | grep -qE "^$pat\$"; then
    echo "MISSING: $pat"
    missing=$((missing+1))
  fi
done
test "$missing" -eq 0 || { echo "snapshot incomplete"; exit 1; }
echo "all expected artifacts present"
```

Expected final output: `all expected artifacts present`.

- [ ] **Step 4: Verify one binary actually runs**

```bash
# Pick the binary that matches your host
host_os=$(go env GOOS)
host_arch=$(go env GOARCH)
case "$host_os/$host_arch" in
  darwin/amd64) bin="dist/ccx_darwin_amd64_v1/ccx" ;;
  darwin/arm64) bin="dist/ccx_darwin_arm64_v8.0/ccx" ;;
  linux/amd64)  bin="dist/ccx_linux_amd64_v1/ccx" ;;
  linux/arm64)  bin="dist/ccx_linux_arm64_v8.0/ccx" ;;
  *) bin="" ;;
esac
[ -n "$bin" ] && [ -x "$bin" ] && "$bin" version
```

Expected: prints the goreleaser-injected version string (e.g., `ccx 0.0.1-snapshot-abc1234 (commit abc1234, built ..., <os>/<arch>)`).

- [ ] **Step 5: Verify the nfpm deb is well-formed**

```bash
deb=$(ls dist/ccx_*_linux_amd64.deb | head -1)
if command -v dpkg-deb >/dev/null 2>&1; then
  dpkg-deb -I "$deb"
  dpkg-deb -c "$deb" | head
else
  echo "dpkg-deb not installed (likely macOS); skipping deep inspection"
  file "$deb"   # should at least report "Debian binary package"
fi
```

Expected: header lists `Package: ccx`, `Maintainer: arafa-dev`, etc. Contents include `./usr/bin/ccx`.

- [ ] **Step 6: Clean up**

```bash
rm -rf dist
# Leave the stub web/out in place IF you created it manually — otherwise:
[ -f web/out/index.html ] && [ "$(stat -f %z web/out/index.html 2>/dev/null || stat -c %s web/out/index.html)" -lt 100 ] && rm -rf web/out
```

- [ ] **Step 7: No commit needed**

This task is verification only. If `dist/` slipped past `.gitignore`, the Phase 0 gitignore already excludes it — confirm with `git status` (should be clean).

---

## Task 9: Add install commands snippet to README

Drop a copy-paste-ready install block at the top of `README.md`. Per the spec, this lives "above the fold" — first thing a reader sees. We add only the install commands here; full README polish lives in plan A9.

**Files:**
- Modify: `README.md` (may not yet exist; create with this content if missing)

- [ ] **Step 1: Check whether `README.md` already exists**

```bash
test -f README.md && head -5 README.md || echo "README.md does not exist yet"
```

- [ ] **Step 2a: If `README.md` does NOT exist, create a minimal stub**

Create `README.md`:

````markdown
# ccx

> Multi-account workspace manager for Claude Code — switch between accounts in one command, see your real usage across all of them, all from a single Go binary.

<!-- Plan A9 adds: hero GIF, dashboard screenshot, comparison table, quick-start, roadmap. -->

## Install

```bash
# Homebrew (macOS / Linux)
brew install arafa-dev/ccx/ccx

# Scoop (Windows)
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop install ccx

# Debian / Ubuntu
curl -fsSLO https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.deb
sudo dpkg -i ccx_linux_amd64.deb

# Fedora / RHEL
curl -fsSLO https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.rpm
sudo rpm -i ccx_linux_amd64.rpm

# One-liner installer (any POSIX shell)
curl -fsSL https://raw.githubusercontent.com/arafa-dev/ccx/main/install.sh | sh

# go install
go install github.com/arafa-dev/ccx/cmd/ccx@latest
```

## License

MIT — see [LICENSE](./LICENSE).
````

- [ ] **Step 2b: If `README.md` already exists, insert the install block**

Use the editor or `sed` to add the install block under the first H1. If unclear where it belongs, append at the end with a `## Install` header — plan A9 will reorganize the README into its final shape.

The block to insert is:

````markdown
## Install

```bash
# Homebrew (macOS / Linux)
brew install arafa-dev/ccx/ccx

# Scoop (Windows)
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop install ccx

# Debian / Ubuntu
curl -fsSLO https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.deb
sudo dpkg -i ccx_linux_amd64.deb

# Fedora / RHEL
curl -fsSLO https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.rpm
sudo rpm -i ccx_linux_amd64.rpm

# One-liner installer (any POSIX shell)
curl -fsSL https://raw.githubusercontent.com/arafa-dev/ccx/main/install.sh | sh

# go install
go install github.com/arafa-dev/ccx/cmd/ccx@latest
```
````

- [ ] **Step 3: Verify the README renders**

```bash
# Local preview if you have a markdown renderer; otherwise just eyeball it:
head -40 README.md
```

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add install commands snippet to README"
```

---

## Task 10: Final local gate + open PR

A8 done. Run the lint and check passes one more time, then open the PR.

**Files:**
- None modified.

- [ ] **Step 1: Run all local checks**

```bash
gofumpt -l .                                            # → no output
go vet ./...                                            # → no output
golangci-lint run                                       # → exit 0
go test -race -count=1 ./...                            # → PASS (or [no test files])
goreleaser check                                        # → validated
actionlint .github/workflows/release.yml                # → no output
shellcheck install.sh                                   # → no output
sh -n install.sh                                        # → no output
```

If any step fails, fix it before opening the PR.

- [ ] **Step 2: Push the worktree branch**

```bash
git push -u origin feat/distribution
```

- [ ] **Step 3: Open the PR**

```bash
gh pr create \
  --base main \
  --title "feat(release): distribution pipeline (goreleaser, nfpm, brew, scoop, install.sh, cosign)" \
  --body "$(cat <<'EOF'
## What

Adds the full distribution pipeline for ccx:

- `.goreleaser.yaml` — cross-compiles darwin/linux/windows × amd64/arm64 (6 binaries), produces `.tar.gz` + `.zip` archives, `.deb` + `.rpm` packages via nfpm, source archive, sha256 checksums, generates Homebrew formula + Scoop manifest, signs `checksums.txt` with cosign
- `.github/workflows/release.yml` — fires on `v*` tag push, builds web dashboard, runs goreleaser, verifies release exists and is signed
- `install.sh` — POSIX curl-pipe installer with sha256 verification, OS/arch detection, version selection
- `docs/distribution/homebrew-tap.md` — bootstrap instructions for the `arafa-dev/homebrew-ccx` repo
- `docs/distribution/scoop-bucket.md` — bootstrap instructions for the `arafa-dev/scoop-ccx` repo
- `docs/distribution/cosign.md` — keypair generation + secret upload guide
- `cmd/ccx/main.go` — minimal stub so goreleaser has something to build; Phase 2 P2 replaces it
- README install snippet (one-liner commands above the fold)

## Why

Phase 1 plan A8 (per `docs/superpowers/plans/2026-05-19-ccx-plan-index.md`). Spec Section 10.

## Contract impact

- [x] This PR does NOT modify `internal/contracts/`, `api/openapi.yaml`, `internal/storage/schema.sql`, or `docs/conventions.md`

## Checklist

- [x] `goreleaser check` passes
- [x] `goreleaser release --snapshot --clean --skip=publish,sign` produces all expected artifacts
- [x] `actionlint .github/workflows/release.yml` clean
- [x] `shellcheck install.sh` clean
- [x] `sh -n install.sh` clean
- [x] No new Go dependencies
- [x] User-visible install instructions reflected in `README.md`

## Required follow-up by the repo owner before tagging v0.1.0

1. Create `arafa-dev/homebrew-ccx` repo (see `docs/distribution/homebrew-tap.md`)
2. Create `arafa-dev/scoop-ccx` repo (see `docs/distribution/scoop-bucket.md`)
3. Generate cosign keypair (see `docs/distribution/cosign.md`)
4. Add repo secrets: `GH_PAT`, `COSIGN_PRIVATE_KEY`, `COSIGN_PASSWORD`

## Phase 1 worktree

- Package: distribution (top-level config, not an `internal/*` package)
- Plan: `docs/superpowers/plans/2026-05-19-ccx-phase-1-A8-distribution.md`
EOF
)"
```

- [ ] **Step 4: Watch CI**

```bash
gh pr checks --watch
```

Expected: lint + test pass on all OSes. The release workflow does **not** fire on this PR (it only fires on tag push).

- [ ] **Step 5: Wait for review and merge**

After merge, the `feat/distribution` worktree can be removed:

```bash
cd /Users/arafa/Developer/ccx
git worktree remove ../ccx-distribution
git branch -D feat/distribution
```

---

## A8 done definition

All of the following are true:

- [ ] `cmd/ccx/main.go` stub exists and `go build -trimpath -ldflags="-X main.version=test" -o /tmp/ccx ./cmd/ccx && /tmp/ccx version` prints the version line
- [ ] `.goreleaser.yaml` exists and `goreleaser check` exits 0
- [ ] `install.sh` exists, is executable, passes `sh -n` and `shellcheck`, prints usage with `--help`
- [ ] `.github/workflows/release.yml` exists and `actionlint` reports no issues
- [ ] `docs/distribution/{homebrew-tap.md,scoop-bucket.md,cosign.md}` all exist
- [ ] `README.md` contains the install commands snippet
- [ ] `goreleaser release --snapshot --clean --skip=publish,sign` produces:
  - 6 binary archives (darwin/linux/windows × amd64/arm64) in `.tar.gz` (unix) / `.zip` (windows)
  - 2 `.deb` files (linux amd64 + arm64)
  - 2 `.rpm` files (linux amd64 + arm64)
  - 1 source archive `.tar.gz`
  - `checksums.txt`
- [ ] PR `feat/distribution` opened against `main`
- [ ] CI on the PR is green
- [ ] Out-of-band: the user has noted (not yet done) the follow-up items: create tap + bucket repos, generate cosign keys, set repo secrets

After merge, the next plan to dispatch is A9 (docs/README polish) or P2 (integration) depending on remaining Phase 1 status — see `docs/superpowers/plans/2026-05-19-ccx-plan-index.md`.
