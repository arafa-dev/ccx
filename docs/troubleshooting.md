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

Claude Code derives its Keychain service name from `CLAUDE_CONFIG_DIR` via
SHA256. If you switched but `claude` still asks you to log in, run:

```bash
ccx doctor
```

It will report which profile the env says is active and whether a matching
Keychain entry exists. If the Keychain entry is missing for the active profile,
run `claude /login` once to create it.

## `ccx dashboard` says port already in use

ccx tries ports 7777–7787 in order. If all are taken (unlikely):

```bash
ccx dashboard --port 8888
```

If you want the dashboard to keep updating without a foreground terminal, run it
through the daemon:

```bash
ccx daemon start
ccx dashboard --daemon
```

## `ccx daemon status` says not running or shows a stale pid

Start with the structured status and logs:

```bash
ccx daemon status --json
ccx daemon logs
```

By default the daemon runtime files are in `~/.ccx/`: `daemon.pid`,
`daemon.json`, `daemon.log`, and `daemon.lock`. A stale pid means ccx found old
runtime state but the recorded process is no longer a matching live daemon.
Usually this is enough:

```bash
ccx daemon stop
ccx daemon start
```

If startup fails before the log is written, run it in the foreground to see the
error directly:

```bash
ccx daemon start --foreground
```

## `ccx hooks status` reports disabled, partial, or invalid

Check all profiles or one profile:

```bash
ccx hooks status
ccx hooks status --profile work --json
```

`disabled` means that profile's Claude Code `settings.json` has
`disableAllHooks: true`; ccx will not override that switch. `partial` means the
settings file is valid but one or more ccx-managed hook entries are missing; run
`ccx hooks install --profile work` to add them. `invalid` means ccx could not
parse the settings safely, so fix the JSON first and rerun install.

When ccx changes an existing settings file, it writes a
`settings.json.ccx-backup-*` file next to it before publishing the update.

## PowerShell blocks `ccx init pwsh`

Windows may block profile scripts on default PowerShell execution-policy settings. If `ccx init pwsh` output is
added to your `$PROFILE` but PowerShell refuses to run it, allow locally created scripts for your user:

```powershell
Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
```

Restart PowerShell, then run `ccx init pwsh | Out-String | Invoke-Expression` again.

## `ccx usage` shows $0 even though I've used Claude

Check:

1. `ccx profile current` — is a profile active?
2. `ccx doctor` — does the profile's config dir exist?
3. Look at `~/.ccx/state.db` — is it being written? `ls -la ~/.ccx/`
4. Look at the JSONL source — `ls -la $CLAUDE_CONFIG_DIR/projects/`

If all of those look fine, run `ccx usage --json` and file an issue with the output.

## `ccx suggest` returns no recommendation

Use JSON output to inspect every candidate and its reasons:

```bash
ccx suggest --json
```

Common reasons:

- Suggestions were disabled for the profile. Re-enable with
  `ccx profile set <name> --suggestions enabled`.
- The profile config dir is missing or unreadable. Check `ccx doctor` and the
  `config_dir` in `~/.ccx/profiles.toml`.
- A hook `StopFailure` in the 30-day lookback recorded `rate_limit`. The
  profile stays unavailable until its cooldown expires; the default is 5 hours.
  Override per profile with `ccx profile set <name> --rate-limit-cooldown 2h`.
- A hook `StopFailure` in the 30-day lookback recorded
  `authentication_failed` or `oauth_org_not_allowed`. ccx treats that profile
  as unavailable instead of recommending a likely-broken account; re-authenticate
  with `eval "$(ccx use <name>)"` and `claude /login` before using it manually.

`ccx suggest` is advisory: it prints a suggested `ccx use <name>` command but
does not switch accounts automatically.

## The quota panel is empty

The quota panel is driven by local Claude Code `Stop` hook events, not by JSONL
token entries alone. Check that hooks are installed and enabled for the profile:

```bash
ccx hooks status --profile work
ccx hooks install --profile work
```

Then complete one Claude turn under that profile and refresh the dashboard. If
the daemon is serving the dashboard, restart it after changing profile settings:

```bash
ccx daemon restart
```

## Supervisor didn't swap when I expected

The supervisor waits for Claude Code's next `Stop` hook before relaunching, so
the active turn can finish cleanly. By default ccx polls local hook telemetry
every 2 seconds; this can delay the swap by up to one poll interval after the
hook lands. Lower `--poll-interval` only if you need tighter feedback, and keep
it above 250ms to avoid hammering SQLite.

If it still does not swap, check that:

- The daemon is running; `ccx run --supervise` can run without daemon SSE, but
  live threshold events are degraded to local polling.
- At least one other profile is available in `ccx suggest --json`.
- Hooks are installed for the active profile, because the supervisor swaps only
  after a local `Stop` event identifies the resumable session.
- Existing profiles have shared history enabled. Run
  `ccx migrate-shared-history --dry-run` before using supervisor mode across
  older profiles.

## I changed plan tier and the cap didn't update

`ccx profile set <name> --plan-tier ...` updates the profile registry
immediately, but a running daemon keeps its in-memory profile list until it is
restarted. Restart it and refresh the dashboard:

```bash
ccx daemon restart
ccx usage --quota --profile <name>
```

Explicit cap overrides win over tier defaults. If a profile still shows an old
cap, inspect `~/.ccx/profiles.toml` for `caps_5h_turns` or
`caps_weekly_turns`, then clear or replace the override with
`ccx profile set`.

## Cost numbers look wrong

ccx ships an embedded pricing table that's accurate as of the version's release
date. If Anthropic changes prices, your numbers will drift until ccx ships an
updated table.

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
