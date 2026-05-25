# B3b Task 0 Verification Findings

Date: 2026-05-25
Claude Code: 2.1.150
ccx branch: feat/quota-supervisor

## Gate 0.1: Claude Code follows projects symlinks

PASS.

Command shape:

```bash
profile_dir=$(mktemp -d)
shared_dir=$(mktemp -d)
ln -s "$shared_dir" "$profile_dir/projects"
CLAUDE_CONFIG_DIR="$profile_dir" claude -p "hi"
find "$shared_dir" -name '*.jsonl' -mmin -2 -print
```

Result:

```text
exit=1
stdout=Not logged in - Please run /login
jsonl=/var/folders/dy/hw7c0dcx41l_c0n8g8c5s84w0000gn/T/tmp.UFuUxDys0m/-Users-arafa-Developer-ccx-quota-supervisor/48666f7c-5042-4ed1-a5f2-d88cf43f11b9.jsonl
```

The command could not complete a model call because the throwaway profile was
not logged in, but Claude Code still wrote the session JSONL through the
`projects` symlink into the shared target. The symlink was not replaced by a
real directory.

## Gate 0.2: Resume flag exists

PASS.

Command:

```bash
claude --help | grep -Ei "resume|continue|session"
```

Relevant output:

```text
-c, --continue                                    Continue the most recent conversation in the current directory
--fork-session                                    When resuming, create a new session ID instead of reusing the original (use with --resume or --continue)
-r, --resume [value]                              Resume a conversation by session ID, or open interactive picker with optional search term
--session-id <uuid>                               Use a specific session ID for the conversation (must be a valid UUID)
```

Supervisor relaunch should use:

```bash
claude --resume <session-id>
```

## Gate 0.3: Hook payload header coverage

PASS.

Command shape:

```bash
profile_dir=$(mktemp -d)
cat > "$profile_dir/settings.json" <<JSON
{
  "hooks": {
    "SessionStart": [{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"$script","timeout":5}]}],
    "Stop": [{"hooks":[{"type":"command","command":"$script","timeout":5}]}],
    "StopFailure": [{"matcher":"rate_limit|authentication_failed|oauth_org_not_allowed|billing_error|invalid_request|model_not_found|server_error|max_output_tokens|unknown","hooks":[{"type":"command","command":"$script","timeout":5}]}]
  }
}
JSON
HOOK_CAPTURE_FILE="$raw" CLAUDE_CONFIG_DIR="$profile_dir" claude -p "hi"
grep -Eio 'anthropic-ratelimit-[A-Za-z0-9_-]+' "$raw"
```

Observed raw payloads:

```json
{"session_id":"7b3e4011-52b6-4694-b773-db50a1c40c9e","transcript_path":"/var/folders/dy/hw7c0dcx41l_c0n8g8c5s84w0000gn/T/tmp.qvDgWSd6qN/projects/-Users-arafa-Developer-ccx-quota-supervisor/7b3e4011-52b6-4694-b773-db50a1c40c9e.jsonl","cwd":"/Users/arafa/Developer/ccx-quota-supervisor","hook_event_name":"SessionStart","source":"startup"}
{"session_id":"7b3e4011-52b6-4694-b773-db50a1c40c9e","transcript_path":"/var/folders/dy/hw7c0dcx41l_c0n8g8c5s84w0000gn/T/tmp.qvDgWSd6qN/projects/-Users-arafa-Developer-ccx-quota-supervisor/7b3e4011-52b6-4694-b773-db50a1c40c9e.jsonl","cwd":"/Users/arafa/Developer/ccx-quota-supervisor","effort":{"level":"xhigh"},"hook_event_name":"StopFailure","error":"authentication_failed","last_assistant_message":"Not logged in · Please run /login"}
```

Header key count:

```text
0
```

I did not force a real upstream 429 because doing so would consume quota on a
real account. The captured `StopFailure` payload shape matches the existing
ccx hook parser assumptions and contained no `anthropic-ratelimit-*` keys or
other response-header fields.
