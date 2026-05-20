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
