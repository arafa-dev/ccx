#!/bin/sh
# install.sh - curl-pipe installer for ccx.
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
USE_SUDO=0

log() {
    printf "ccx-install: %s\n" "$*" >&2
}

die() {
    log "error: $*"
    exit 1
}

usage() {
    cat <<'EOF'
install.sh - curl-pipe installer for ccx.

Usage:
  curl -fsSL https://raw.githubusercontent.com/arafa-dev/ccx/main/install.sh | sh
  curl -fsSL https://raw.githubusercontent.com/arafa-dev/ccx/main/install.sh | sh -s -- --version v0.1.0

Flags:
  --version vX.Y.Z   Install a specific version (default: latest release)
  --bin-dir DIR      Override install directory
  --help             Show this help

Supported platforms: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64.
Windows users: please use Scoop (see README).
EOF
    exit 0
}

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

uname_s="$(uname -s 2>/dev/null || echo unknown)"
case "$uname_s" in
    Darwin) os="darwin" ;;
    Linux) os="linux" ;;
    *) die "unsupported OS: $uname_s. Windows users: use Scoop. See https://github.com/$REPO" ;;
esac

uname_m="$(uname -m 2>/dev/null || echo unknown)"
case "$uname_m" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "unsupported architecture: $uname_m (only amd64 and arm64 are supported)" ;;
esac

if [ -z "$VERSION" ]; then
    log "resolving latest release tag from github.com/$REPO ..."
    VERSION="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" \
        | sed 's#.*/tag/##')"
    [ -n "$VERSION" ] || die "could not resolve latest version; pass --version vX.Y.Z explicitly"
fi

case "$VERSION" in
    v*) tag="$VERSION"; ver="${VERSION#v}" ;;
    *) tag="v$VERSION"; ver="$VERSION" ;;
esac

if [ -z "$BIN_DIR" ]; then
    if [ -w "$DEFAULT_BIN_DIR_ROOT" ] || [ "$(id -u)" = "0" ]; then
        BIN_DIR="$DEFAULT_BIN_DIR_ROOT"
    elif command -v sudo >/dev/null 2>&1; then
        BIN_DIR="$DEFAULT_BIN_DIR_ROOT"
        USE_SUDO=1
    else
        BIN_DIR="$DEFAULT_BIN_DIR_USER"
    fi
fi

command -v curl >/dev/null 2>&1 || die "curl is required but not installed"
command -v tar >/dev/null 2>&1 || die "tar is required but not installed"

if command -v sha256sum >/dev/null 2>&1; then
    SHA_TOOL="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    SHA_TOOL="shasum"
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

log "verifying SHA-256 ..."
expected="$(awk -v name="$archive_name" '$2 == name { print $1; found = 1 } END { if (!found) exit 1 }' "$tmpdir/checksums.txt" || true)"
[ -n "$expected" ] || die "no checksum entry found for $archive_name in checksums.txt"
if [ "$SHA_TOOL" = "sha256sum" ]; then
    actual="$(cd "$tmpdir" && sha256sum "$archive_name" | awk '{ print $1 }')"
else
    actual="$(cd "$tmpdir" && shasum -a 256 "$archive_name" | awk '{ print $1 }')"
fi
[ "$expected" = "$actual" ] || die "checksum mismatch: expected $expected, got $actual"

log "extracting $archive_name"
(cd "$tmpdir" && tar -xzf "$archive_name")
[ -f "$tmpdir/ccx" ] || die "ccx binary missing from extracted archive"
chmod +x "$tmpdir/ccx"

log "installing to $BIN_DIR/ccx"
if [ "$USE_SUDO" = "1" ]; then
    sudo mkdir -p "$BIN_DIR"
    sudo install -m 0755 "$tmpdir/ccx" "$BIN_DIR/ccx"
else
    mkdir -p "$BIN_DIR"
    if ! install -m 0755 "$tmpdir/ccx" "$BIN_DIR/ccx" 2>/dev/null; then
        cp "$tmpdir/ccx" "$BIN_DIR/ccx"
        chmod 0755 "$BIN_DIR/ccx"
    fi
fi

log "installed ccx $tag to $BIN_DIR/ccx"
case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *)
        log "note: $BIN_DIR is not on your PATH. Add it with:"
        printf '%s\n' "export PATH=\"$BIN_DIR:\$PATH\"" >&2
        ;;
esac

log "run 'ccx --help' to get started."
