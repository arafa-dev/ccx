# How `ccx use` actually switches accounts

ccx never modifies your shell from outside. It can't — child processes can't
change parent-shell env vars. Instead, `ccx use` *prints* the right `export`
statements, and your shell `eval`s them.

## The mechanism

```bash
eval "$(ccx use work)"
```

`ccx use work` writes to stdout:

```sh
export CLAUDE_CONFIG_DIR='/Users/you/.claude-profiles/work'
export CCX_ACTIVE_PROFILE='work'
```

Your shell evaluates those exports. The `claude` CLI you run next reads its config from `CLAUDE_CONFIG_DIR`.

## Per-OS credential isolation

- **macOS** — Claude Code derives its Keychain service name from the config dir
  path via SHA256. Switching `CLAUDE_CONFIG_DIR` automatically routes to a
  different Keychain entry. No file copying.
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

`ccx use` sets `CCX_ACTIVE_PROFILE` so other ccx commands know which profile
you're using. `ccx profile current` reads it. `ccx usage` defaults to it.

If you log into a new shell without running `ccx use`, no profile is active —
`claude` falls back to the default `~/.claude` config.

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
