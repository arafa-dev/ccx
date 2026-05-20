# ccx

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

> Multi-account workspace manager for Claude Code - switch between accounts in one command, see your real usage across all of them, all from a single Go binary.

<!-- Plan A9 adds: hero GIF, dashboard screenshot, comparison table, quick-start, roadmap. -->

## License

MIT - see [LICENSE](./LICENSE).
