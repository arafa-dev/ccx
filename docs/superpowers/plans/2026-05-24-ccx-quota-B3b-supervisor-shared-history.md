# ccx v0.2 B3b — Supervisor + Shared History Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable *mid-session* profile swap. When the active profile crosses the hard pressure threshold, `ccx run --supervise` waits for the next `Stop` hook event, kills `claude` gracefully, relaunches under the next-best-headroom sibling with `claude --resume <session-id>`. Conversation history survives the swap because every profile's `<CLAUDE_CONFIG_DIR>/projects/` is a symlink to one shared `~/.ccx/shared-projects/` directory.

**This is the most architecturally invasive phase of v0.2.** Three coupled changes:

1. **Shared history layout.** Existing v0.1 profiles each have their own `<config_dir>/projects/`. We symlink them all to `~/.ccx/shared-projects/`. New profiles get the symlink automatically; existing ones opt in via a new `ccx migrate-shared-history` command with `--dry-run`.
2. **Scanner refactor.** Today `internal/scanner` walks `<config_dir>/projects/` per profile and emits events keyed to that profile. With shared history, all profiles' walks see the same files; we instead walk the shared dir once and attribute each event to the profile that owns the session (looked up via `sessions.profile_name`).
3. **Supervisor mode.** `ccx run --supervise` runs `claude` as a child, monitors hook events (via daemon SSE if running, else local DB poll), and on a hard-pressure event from the active profile: waits for the next `Stop`, signals the child to exit cleanly, relaunches `claude --resume <session-id>` under the recommended sibling.

**Architecture:**

```
                   ┌───────────────────────────────────────────────────┐
                   │  ccx run --supervise                              │
                   │  ┌─────────────────────────┐                      │
                   │  │  child: claude          │  ← CLAUDE_CONFIG_DIR │
                   │  │  (one of N profiles)    │     swapped between  │
                   │  └────────────┬────────────┘     relaunches       │
                   │               │ stdio + signals                   │
                   │               ▼                                   │
                   │  ┌─────────────────────────┐                      │
                   │  │  supervisor goroutine   │ ──┐                  │
                   │  │  - tracks session_id    │   │ on hard event:   │
                   │  │  - subscribes to        │   │  1. wait next    │
                   │  │    RecommendationEvent  │   │     Stop event   │
                   │  │  - picks next profile   │   │  2. SIGTERM      │
                   │  └─────────────────────────┘   │  3. Wait         │
                   │                                │  4. swap env     │
                   │                                │  5. exec claude  │
                   │                                │     --resume sid │
                   │                                └──────────────────┘
                   └───────────────────────────────────────────────────┘

                            symlink layout per profile:
   ~/.claude-profiles/<name>/projects/  →  ~/.ccx/shared-projects/

   Scanner walks ~/.ccx/shared-projects/ once. For each event, looks up
   sessions.profile_name (recorded by the SessionStart hook with --profile=<name>)
   to attribute the event to the right ccx profile.
```

**Tech Stack:** Go 1.22+ stdlib only. New filesystem operations (`os.Symlink`, `os.Readlink`). No new external deps.

**Spec reference:** [`docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`](../specs/2026-05-24-ccx-plan-aware-quota.md) — §5.1 (turn counting source), §6.5 (auto-switch granularity), §6.6 (shared history), §11 (verification gates).

**Worktree:**

```bash
git fetch origin
git worktree add ../ccx-quota-supervisor -b feat/quota-supervisor origin/main
cd ../ccx-quota-supervisor
```

B3a must already be merged.

**Exit criteria:**

- [ ] **Task 0 verification gates passed and findings recorded** (Claude Code symlink behavior, `--resume` flag, no header data in hook payloads). If any gate fails, this plan is paused and the spec is amended.
- [ ] `go build ./...` succeeds across darwin/linux/windows
- [ ] `go test -race -count=1 ./...` succeeds (including the new scanner refactor; integration test under `integration_test/` for the shared scan path)
- [ ] `golangci-lint run ./...` reports `0 issues.`
- [ ] `cd web && pnpm typecheck && pnpm test` succeed
- [ ] `make ci` green
- [ ] Manual smoke: `ccx migrate-shared-history --dry-run` lists planned symlinks without modifying disk
- [ ] Manual smoke: `ccx run --supervise --profile demo` with `Caps5hTurns: 1` swaps to a sibling profile after one Stop event
- [ ] Manual smoke: post-swap, `claude --resume <session-id>` loaded by the supervisor shows the same conversation
- [ ] PR opened, CI green, merged
- [ ] Plan index status updated

**Conventions:** unchanged. Scopes: `run`, `cli`, `scanner`, `quotamigrate`.

---

## Pre-flight

```bash
pwd                                                  # → .../ccx-quota-supervisor
git status                                           # → On branch feat/quota-supervisor, working tree clean
grep -l "func Launch" internal/run/launcher.go && echo OK     # B3a merged
grep -l "func.*EmitClaudeWrapperPosix" internal/shell/snippets.go && echo OK
grep -l "NewHub" internal/recstream/hub.go && echo OK
go build ./... && echo OK
which claude  # supervisor smoke depends on this; record the path for later
```

---

## Task 0: Verification gates (gates the rest of the plan)

**Goal:** validate the three assumptions the supervisor architecture depends on. If any fail, **stop**, update the spec accordingly, and re-plan before continuing.

- [ ] **Gate 0.1: Does Claude Code follow symlinks for `<CLAUDE_CONFIG_DIR>/projects/`?**

```bash
# Pick a throwaway test profile directory.
TEST_PROFILE_DIR=$(mktemp -d)
SHARED_DIR=$(mktemp -d)

# Construct: TEST_PROFILE_DIR/projects → SHARED_DIR
ln -s "$SHARED_DIR" "$TEST_PROFILE_DIR/projects"

# Run claude under the test profile dir, send one minimal prompt.
CLAUDE_CONFIG_DIR="$TEST_PROFILE_DIR" claude -p "hi"

# Verify the new session JSONL landed in SHARED_DIR.
find "$SHARED_DIR" -name '*.jsonl' -mmin -1 | head
```

**Expected:** at least one `.jsonl` file in `$SHARED_DIR` updated within the last minute.

If empty / claude refused / claude wrote to `$TEST_PROFILE_DIR/projects` literally (i.e., overwrote the symlink with a real directory), record the failure mode in a `B3b-task0-findings.md` scratch note (commit it to the worktree under `docs/`) and **stop**.

- [ ] **Gate 0.2: Does `claude --resume <session-id>` exist?**

```bash
claude --help | grep -E "resume|continue|session"
```

Look for an option that resumes a specific session by id. Document the exact flag name and syntax. Update Task 9 (relaunch step) accordingly. If no such flag exists in the installed `claude`, **stop** and report.

- [ ] **Gate 0.3: Do hook payloads include `anthropic-ratelimit-*` headers?**

Trigger a 429 (the easiest path: run a long burst with a low-quota profile, or temporarily set `Caps5hTurns: 1` on a test profile after one turn). Inspect the most recent `StopFailure` payload that ccx received:

```bash
sqlite3 ~/.ccx/state.db \
  "SELECT event_name, reason, error, error_details FROM hook_events ORDER BY ts DESC LIMIT 5;"
```

If `error_details` is a JSON blob with `anthropic-ratelimit-*` keys, the spec's §6.3 was wrong — open a contract-amendment to add structured fields to `contracts.HookEvent`. If not (expected), proceed.

- [ ] **Gate 0.4: Record the findings**

Append to the PR description (and to `docs/superpowers/plans/2026-05-24-ccx-quota-B3b-supervisor-shared-history.md` under a new `## Task 0 findings (filled in by implementation agent)` section) the exact results of all three gates:

```
Gate 0.1 (symlink): PASS — JSONL landed at /tmp/.../<file>.jsonl
Gate 0.2 (resume): PASS — flag is `claude --resume <session-id>` (or: --continue, --session, etc.)
Gate 0.3 (headers): FAIL/PASS — see error_details samples below
```

**Do not commit until all three gates pass or have a documented workaround.** If 0.1 fails, this whole plan is invalid as written.

- [ ] **Gate 0.5: Commit (a note-only commit, no code yet)**

```bash
mkdir -p docs/superpowers/notes
$EDITOR docs/superpowers/notes/2026-XX-XX-b3b-task0-findings.md
git add docs/superpowers/notes/
git commit -m "docs(quota): B3b Task 0 verification findings"
```

---

## Task 1: Add shared-projects path resolver (TDD)

**Files:**
- Create: `internal/quotamigrate/paths.go`
- Create: `internal/quotamigrate/paths_test.go`

A small leaf package for the symlink layout. Lives separately from `internal/quota/` because the migration command is human-facing.

- [ ] **Step 1: Failing test**

```go
package quotamigrate_test

import (
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/quotamigrate"
)

func TestSharedProjectsPath(t *testing.T) {
	got := quotamigrate.SharedProjectsPath("/home/x/.ccx")
	want := "/home/x/.ccx/shared-projects"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProfileProjectsPath(t *testing.T) {
	got := quotamigrate.ProfileProjectsPath("/home/x/.claude-profiles/work")
	want := filepath.Join("/home/x/.claude-profiles/work", "projects")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL.

- [ ] **Step 3: Implementation**

`internal/quotamigrate/paths.go`:

```go
// Package quotamigrate owns the symlink layout that B3b introduces:
// every profile's <CLAUDE_CONFIG_DIR>/projects/ is a symlink to one shared
// directory at <CCX_HOME>/shared-projects/. This package provides the path
// resolvers and the migration command logic.
package quotamigrate

import "path/filepath"

// SharedProjectsPath returns the shared-projects directory under ccxHome.
func SharedProjectsPath(ccxHome string) string {
	return filepath.Join(ccxHome, "shared-projects")
}

// ProfileProjectsPath returns the projects/ path inside the given profile
// config dir.
func ProfileProjectsPath(profileConfigDir string) string {
	return filepath.Join(profileConfigDir, "projects")
}
```

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/quotamigrate/
go test -race -count=1 ./internal/quotamigrate/...
golangci-lint run ./internal/quotamigrate/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/quotamigrate/paths.go internal/quotamigrate/paths_test.go
git commit -m "feat(quotamigrate): shared-projects and profile-projects path helpers"
```

---

## Task 2: Shared-history migration plan + apply (TDD)

**Files:**
- Create: `internal/quotamigrate/migrate.go`
- Create: `internal/quotamigrate/migrate_test.go`

- [ ] **Step 1: Failing test**

```go
package quotamigrate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/quotamigrate"
)

func TestPlanLinksMissingProjects(t *testing.T) {
	ccxHome := t.TempDir()
	profileDir := t.TempDir()
	// No projects/ at all yet → plan should symlink it.
	profile := contracts.Profile{Name: "work", ConfigDir: profileDir}
	steps, err := quotamigrate.Plan(ccxHome, []contracts.Profile{profile})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("steps: got %d, want 1", len(steps))
	}
	if steps[0].Action != quotamigrate.ActionCreateSymlink {
		t.Errorf("action: got %v, want CreateSymlink", steps[0].Action)
	}
}

func TestPlanMovesAndLinksExistingDir(t *testing.T) {
	ccxHome := t.TempDir()
	profileDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(profileDir, "projects/foo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "projects/foo/sess.jsonl"), []byte(`{"type":"user"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	profile := contracts.Profile{Name: "work", ConfigDir: profileDir}
	steps, err := quotamigrate.Plan(ccxHome, []contracts.Profile{profile})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("steps: got %d, want 2 (move + symlink)", len(steps))
	}
	if steps[0].Action != quotamigrate.ActionMoveContents {
		t.Errorf("step 0: got %v", steps[0].Action)
	}
	if steps[1].Action != quotamigrate.ActionCreateSymlink {
		t.Errorf("step 1: got %v", steps[1].Action)
	}
}

func TestPlanSkipsAlreadyLinked(t *testing.T) {
	ccxHome := t.TempDir()
	profileDir := t.TempDir()
	shared := quotamigrate.SharedProjectsPath(ccxHome)
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(profileDir, "projects")); err != nil {
		t.Fatal(err)
	}
	profile := contracts.Profile{Name: "work", ConfigDir: profileDir}
	steps, err := quotamigrate.Plan(ccxHome, []contracts.Profile{profile})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("already-linked should plan zero steps; got %+v", steps)
	}
}

func TestApplyExecutesPlan(t *testing.T) {
	ccxHome := t.TempDir()
	profileDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(profileDir, "projects/foo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "projects/foo/sess.jsonl"), []byte(`x`), 0o600); err != nil {
		t.Fatal(err)
	}
	steps, err := quotamigrate.Plan(ccxHome, []contracts.Profile{{Name: "work", ConfigDir: profileDir}})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if err := quotamigrate.Apply(steps); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Verify symlink exists and points at shared.
	target, err := os.Readlink(filepath.Join(profileDir, "projects"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != quotamigrate.SharedProjectsPath(ccxHome) {
		t.Errorf("symlink target: got %q", target)
	}
	// Verify content moved.
	if _, err := os.Stat(filepath.Join(quotamigrate.SharedProjectsPath(ccxHome), "foo/sess.jsonl")); err != nil {
		t.Errorf("expected moved file; err: %v", err)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

- [ ] **Step 3: Implementation**

`internal/quotamigrate/migrate.go`:

```go
package quotamigrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Action is what a Step does.
type Action int

const (
	// ActionMoveContents copies a profile's projects subtree into the shared
	// directory (without overwriting siblings) and removes the source dir.
	ActionMoveContents Action = iota + 1
	// ActionCreateSymlink replaces (or creates) the profile's projects entry
	// with a symlink to the shared directory.
	ActionCreateSymlink
)

// Step is one filesystem operation in a migration plan.
type Step struct {
	Profile   string
	Action    Action
	From      string // for MoveContents
	To        string // for both
}

// String renders a step in a human-readable form for `--dry-run` output.
func (s Step) String() string {
	switch s.Action {
	case ActionMoveContents:
		return fmt.Sprintf("[%s] move contents of %s → %s", s.Profile, s.From, s.To)
	case ActionCreateSymlink:
		return fmt.Sprintf("[%s] symlink %s → %s", s.Profile, s.From, s.To)
	default:
		return fmt.Sprintf("[%s] unknown action", s.Profile)
	}
}

// Plan inspects the disk state for each profile and returns the migration
// steps needed to bring it into the shared-projects layout. Returns an empty
// slice when everything is already migrated.
func Plan(ccxHome string, profiles []contracts.Profile) ([]Step, error) {
	shared := SharedProjectsPath(ccxHome)
	var steps []Step
	for i := range profiles {
		p := &profiles[i]
		projects := ProfileProjectsPath(p.ConfigDir)
		info, err := os.Lstat(projects)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("lstat %q: %w", projects, err)
		}
		if info != nil && info.Mode()&os.ModeSymlink != 0 {
			target, lerr := os.Readlink(projects)
			if lerr != nil {
				return nil, fmt.Errorf("readlink %q: %w", projects, lerr)
			}
			if target == shared {
				continue // already migrated
			}
			return nil, fmt.Errorf("[%s] %q is a symlink to unexpected target %q; refusing to overwrite", p.Name, projects, target)
		}
		if info != nil && info.IsDir() {
			steps = append(steps, Step{
				Profile: p.Name, Action: ActionMoveContents,
				From: projects, To: shared,
			})
		}
		steps = append(steps, Step{
			Profile: p.Name, Action: ActionCreateSymlink,
			From: projects, To: shared,
		})
	}
	return steps, nil
}

// Apply executes the plan. It is safe to re-run; idempotent steps are no-ops.
// Stops at the first error and returns it; partial state may be left on disk.
func Apply(steps []Step) error {
	for _, s := range steps {
		switch s.Action {
		case ActionMoveContents:
			if err := mergeDir(s.From, s.To); err != nil {
				return fmt.Errorf("apply move %q→%q: %w", s.From, s.To, err)
			}
			if err := os.Remove(s.From); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("apply remove %q: %w", s.From, err)
			}
		case ActionCreateSymlink:
			// If From exists (e.g., we just removed it but a race created it
			// again, or it never existed), os.Symlink will fail; remove first.
			if _, err := os.Lstat(s.From); err == nil {
				if err := os.Remove(s.From); err != nil {
					return fmt.Errorf("remove pre-symlink %q: %w", s.From, err)
				}
			}
			if err := os.MkdirAll(s.To, 0o700); err != nil {
				return fmt.Errorf("mkdir shared %q: %w", s.To, err)
			}
			if err := os.Symlink(s.To, s.From); err != nil {
				return fmt.Errorf("symlink %q→%q: %w", s.From, s.To, err)
			}
		default:
			return fmt.Errorf("apply: unknown action %v", s.Action)
		}
	}
	return nil
}

// mergeDir copies every entry of src into dst, preserving file mode. Refuses
// to overwrite an existing file at the destination (returns an error). Creates
// dst if missing.
func mergeDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("refusing to overwrite existing file %q", target)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
```

Add `"io"` to the imports at the top of `migrate.go`.

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/quotamigrate/
go test -race -count=1 ./internal/quotamigrate/...
golangci-lint run ./internal/quotamigrate/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/quotamigrate/migrate.go internal/quotamigrate/migrate_test.go
git commit -m "feat(quotamigrate): Plan and Apply for shared-projects migration"
```

---

## Task 3: `ccx migrate-shared-history` CLI command (TDD)

**Files:**
- Create: `internal/cli/migrate.go`
- Create: `internal/cli/migrate_test.go`
- Modify: `internal/cli/cli.go` (register)

- [ ] **Step 1: Failing test**

Use the existing CLI test helpers (`runCLI`, `runCLIResult`) and the
`t.Setenv("HOME", ...)` pattern from `usage_test.go`. No new helpers needed.

```go
// setupQuotaProfile registers one ccx profile under a fresh HOME and returns
// its config directory. Reuses the pattern from internal/cli/usage_test.go.
// Inline-defined (not a shared helper) because it's specific to migration tests.
func setupQuotaProfile(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "demo", "--config-dir", cfgDir)
	return cfgDir
}

func TestMigrateDryRunPrintsPlanWithoutTouchingDisk(t *testing.T) {
	profileDir := setupQuotaProfile(t)
	// Remove the projects symlink that `ccx profile add` creates (Task 4)
	// so this test exercises the missing-projects → plan path.
	_ = os.RemoveAll(filepath.Join(profileDir, "projects"))

	out := runCLI(t, "migrate-shared-history", "--dry-run")
	if !strings.Contains(out, "symlink") {
		t.Errorf("expected 'symlink' in plan output; got:\n%s", out)
	}
	// Disk unchanged.
	if _, err := os.Lstat(filepath.Join(profileDir, "projects")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("dry-run created %s/projects; should not", profileDir)
	}
}

func TestMigrateApplyCreatesSymlink(t *testing.T) {
	profileDir := setupQuotaProfile(t)
	_ = os.RemoveAll(filepath.Join(profileDir, "projects"))

	runCLI(t, "migrate-shared-history")

	info, err := os.Lstat(filepath.Join(profileDir, "projects"))
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected symlink at %s/projects", profileDir)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

- [ ] **Step 3: Implementation**

`internal/cli/migrate.go`:

```go
package cli

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/quotamigrate"
	"github.com/spf13/cobra"
)

func newMigrateSharedHistoryCommand(_ *Options) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "migrate-shared-history",
		Short: "Symlink every profile's projects/ to ~/.ccx/shared-projects/",
		Long: `Walks the profile registry and prints (or executes) the filesystem changes
needed so every profile's <config_dir>/projects/ is a symlink to one shared
directory. Required for ccx run --supervise mid-session swap to preserve
conversation history.`,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			home, err := platform.CCXHome()
			if err != nil {
				return err
			}
			profiles, err := deps.Profiles.List(ctx)
			if err != nil {
				return err
			}
			steps, err := quotamigrate.Plan(home, profiles)
			if err != nil {
				return err
			}
			if len(steps) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "Nothing to do; all profiles already use shared projects.")
				return nil
			}
			for _, s := range steps {
				fmt.Fprintln(c.OutOrStdout(), s.String())
			}
			if dryRun {
				fmt.Fprintln(c.OutOrStdout(), "\n(dry-run; pass without --dry-run to apply)")
				return nil
			}
			if err := quotamigrate.Apply(steps); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "\nMigration complete (%d step(s)).\n", len(steps))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan without modifying anything")
	return cmd
}
```

Register in `cli.go` alongside the other commands.

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/cli/
go test -race -count=1 ./internal/cli/...
golangci-lint run ./internal/cli/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/cli/migrate.go internal/cli/cli.go internal/cli/migrate_test.go
git commit -m "feat(cli): ccx migrate-shared-history"
```

---

## Task 4: Auto-symlink on `ccx profile add`

**Files:**
- Modify: `internal/cli/profile.go` (the `add` subcommand)
- Modify: `internal/cli/profile_test.go`

- [ ] **Step 1: Failing test**

```go
func TestProfileAddCreatesSharedSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	profileDir := filepath.Join(home, "claude-demo")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}

	runCLI(t, "profile", "add", "demo", "--config-dir", profileDir)

	info, err := os.Lstat(filepath.Join(profileDir, "projects"))
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("ccx profile add should create symlink at %s/projects", profileDir)
	}
	target, err := os.Readlink(filepath.Join(profileDir, "projects"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if !strings.HasSuffix(target, "shared-projects") {
		t.Errorf("symlink target = %q, want suffix shared-projects", target)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

- [ ] **Step 3: Implementation**

In `internal/cli/profile.go`, after the existing `profile.Manager.Add(...)` call inside the `add` subcommand's `RunE`, call:

```go
home, err := platform.CCXHome()
if err != nil {
    return err
}
steps, err := quotamigrate.Plan(home, []contracts.Profile{p})
if err != nil {
    return err
}
if err := quotamigrate.Apply(steps); err != nil {
    return err
}
```

Where `p` is the freshly-added profile. Imports: `internal/platform`, `internal/quotamigrate`.

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/cli/profile.go
go test -race -count=1 ./internal/cli/...
golangci-lint run ./internal/cli/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/cli/profile.go internal/cli/profile_test.go
git commit -m "feat(cli): ccx profile add auto-symlinks shared projects/"
```

---

## Task 5: Scanner refactor — walk shared dir once, attribute via sessions table

> ⚠️ **Design-first task.** Unlike every other task in this plan, Task 5 is
> NOT a turnkey TDD recipe with full code. The surrounding `internal/scanner/`
> package is dense (parse.go, file.go, cursor.go, scanner.go, plus inode
> handling, fuzz tests, golden tests, and concurrency tests), and a correct
> shared-scan factoring depends on choices that only become clear once the
> implementation agent has the existing code loaded.
>
> **Required output of Task 5 Step 0 (below):** a short written micro-spec
> (≤200 lines, committed under `docs/superpowers/notes/`) covering:
>
> 1. The exact `ScanShared` signature and the new `SessionLookup` interface.
> 2. The cursor-key change strategy (drop `profile_name`, use sentinel like
>    `"__shared__"`, OR add a new `SharedCursorStore` method).
> 3. The fallback-detection rule: how does the runtime decide between
>    `ScanShared` (symlink present) and `Scan` (per-profile)?
> 4. The attribution miss policy: skip with debug log? error? buffer for
>    retry? The plan defaults to "skip with debug log"; document the choice.
> 5. The migration story for the `events` table — existing rows already have
>    `profile_name`; do we re-attribute on the next scan, or leave them?
>
> Then come back and have a human re-read the micro-spec before writing
> any scanner code. The remaining steps below sketch the surface area; do
> not treat them as a TDD recipe.

**Files (likely):**
- Modify: `internal/scanner/scanner.go`
- Modify: `internal/scanner/scanner_test.go`
- Possibly: `internal/scanner/file.go` (if cursor key changes)
- Possibly: `internal/scanner/cursor.go`

The scanner currently walks `<profile.ConfigDir>/projects/` per profile. After the symlink migration, every profile's walk hits the same files; per-profile cursors duplicate work. New behavior:

- Scanner takes an explicit "projects root" path instead of inferring from a `contracts.Profile`.
- For each event parsed, the scanner looks up the session-id → profile mapping via a new injected `SessionLookup` (backed by `*storage.Store.QuerySessions`).
- If the lookup returns no profile (e.g., the SessionStart hook never fired), the event is skipped with a debug log.
- The cursor key drops `profile_name` and uses just `file_path` (or uses a sentinel — pick in the micro-spec).

This is a significant change but is gated behind the migration — pre-migration profiles still have a non-symlink `projects/` dir, and the scanner can detect that case and fall back to v0.1 behavior.

- [ ] **Step 0: Produce the micro-spec described above**

Commit the file under `docs/superpowers/notes/2026-05-25-b3b-scanner-refactor.md`
with the five decisions written out. Open it as a separate small PR before
starting any scanner code, or merge it inline with the worktree but call
attention to it in the eventual PR description. The implementation agent
should pause here until the micro-spec is reviewed.

- [ ] **Step 1: Failing tests**

Add tests in `internal/scanner/scanner_test.go` that exercise:

- `Scan` with a `SessionLookup` that returns "work" for `s1` and "personal" for `s2`. Inject two JSONL files (one per session) into the shared dir. Assert that the emitted events carry the right profile attribution.
- `Scan` with a `SessionLookup` that returns no result for a session_id. Assert the event is skipped (not emitted) and no error.

(The exact test structure depends on whether the implementation agent chooses to enrich `contracts.Event` with a `Profile` field — currently it lacks one — or to wrap emissions in a new type. Both are valid; the simpler path is a wrapper struct `scanner.AttributedEvent{Event, Profile}` returned on the channel.)

- [ ] **Step 2: Implementation strategy**

Recommend the lighter approach: introduce a new function alongside the existing `Scan`:

```go
// ScanShared walks projectsRoot once and emits events on the channel, attributed
// via the supplied SessionLookup. Use this when the shared-projects symlink
// layout from B3b is active. The traditional per-profile Scan remains for
// backwards compatibility with unmigrated profiles.
func (s *Scanner) ScanShared(ctx context.Context, projectsRoot string, lookup SessionLookup) (<-chan contracts.AttributedEvent, <-chan error)
```

Add to `internal/contracts/types.go`? **NO** — that's frozen. Define `AttributedEvent` in `internal/scanner/` instead:

```go
type AttributedEvent struct {
    contracts.Event
    Profile string
}
```

Wire `cli.QuotaAdapter` / daemon ingest to use `ScanShared` when the shared dir exists and contains the symlink target, falling back to the per-profile `Scan` otherwise.

`SessionLookup` interface:

```go
// SessionLookup resolves session_id → owning ccx profile name. Implemented by
// a thin adapter over *storage.Store.QuerySessions.
type SessionLookup interface {
    ProfileForSession(ctx context.Context, sessionID string) (string, bool, error)
}
```

Implement on `*storage.Store` directly:

```go
// ProfileForSession returns the ccx profile name that owns the given Claude
// Code session, as recorded by the SessionStart hook event.
func (s *Store) ProfileForSession(ctx context.Context, sessionID string) (string, bool, error) {
    const q = `SELECT profile_name FROM sessions WHERE session_id = ? LIMIT 1`
    var name string
    err := s.db.QueryRowContext(ctx, q, sessionID).Scan(&name)
    if errors.Is(err, sql.ErrNoRows) {
        return "", false, nil
    }
    if err != nil {
        return "", false, fmt.Errorf("ProfileForSession: %w", err)
    }
    return name, true, nil
}
```

- [ ] **Step 3: Implementation**

Refactor `scanner.go` to add `ScanShared`. Keep the existing `Scan` API unchanged (callers that still pass per-profile will get the v0.1 behavior). The cursor change for shared scan: key is just `(file_path)` not `(profile, file_path)`. Use a new helper method on `CursorStore` for the shared path, OR adopt a sentinel profile name like `"__shared__"` to reuse the existing `(profile, path)` cursor schema.

The exact code is left to the implementation agent — this plan does not prescribe the full diff because the surrounding scanner package is dense and the right factoring depends on how `cursor.go` is structured. The TDD discipline (failing tests first) is what keeps the refactor honest.

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/scanner/
go test -race -count=1 ./internal/scanner/...
go test -race -count=1 ./internal/storage/...
golangci-lint run ./...
```

- [ ] **Step 5: Wire ingest paths**

Update both `internal/cli/suggest.go::ingestSuggestProfile` and `internal/daemon/ingest.go` to:

1. Check whether `<ccx home>/shared-projects/` exists.
2. If it does and at least one profile's `<config>/projects/` is a symlink to it, run `ScanShared` once (passing all profiles, since shared is one walk) and route emitted `AttributedEvent`s to the right `InsertEvents` call per profile.
3. Otherwise, fall back to the existing per-profile `Scan`.

Add tests in `internal/daemon/ingest_test.go` (or wherever ingest logic is tested) for both branches.

- [ ] **Step 6: Commit (one per logical piece)**

```bash
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "feat(scanner): ScanShared walks one dir, attributes via SessionLookup"

git add internal/storage/sessions.go internal/storage/sessions_test.go
git commit -m "feat(storage): ProfileForSession lookup"

git add internal/cli/suggest.go internal/daemon/ingest.go internal/daemon/ingest_test.go
git commit -m "feat(daemon,cli): use ScanShared when shared-projects symlink active"
```

---

## Task 6: Supervisor — wait-and-swap loop (TDD)

**Files:**
- Create: `internal/run/supervise.go`
- Create: `internal/run/supervise_test.go`

The supervisor is a goroutine running alongside `run.Launch`. It:

1. Subscribes to `contracts.RecommendationEvent` (via `recstream.Hub` if running in-process, or via SSE GET to `/api/recommendations/live` if the daemon is the source).
2. Tracks the active session's session_id by reading from `hook_events` (the `SessionStart` event with the most recent timestamp for the active profile).
3. On a `Hard` event for the active profile, *waits* until the next `Stop` event arrives, then signals the child to exit.
4. After the child exits, calls `run.Pick` to choose the next profile, builds a new env, launches `claude --resume <session-id>` (or the equivalent flag confirmed in Gate 0.2).

- [ ] **Step 1: Failing tests**

Write tests that mock the hook event source and the launcher. The interface surface should look like:

```go
type Supervisor struct {
    Profiles []contracts.Profile
    Picker   func(ctx context.Context, exclude string) (contracts.Profile, string, error)
    Events   <-chan contracts.RecommendationEvent
    Hooks    HookSource          // for finding session_id and waiting on Stop
    Launcher Launcher            // wraps run.Launch
    ResumeFlag string            // e.g., "--resume" or "--continue"; default "--resume"
}

func (s *Supervisor) Run(ctx context.Context, initial contracts.Profile, args []string) error
```

Test scenarios:

- Happy path: launcher returns exit 0 after one swap. Assert the second Launch call's env has `CLAUDE_CONFIG_DIR=<sibling>` and the args include `--resume <session-id>`.
- No Hard event: launcher runs to completion under the initial profile; no swap.
- Hard event but Picker returns ErrNoRecommendation: supervisor logs and exits without swapping.
- Hard event, swap occurs, second profile also hits hard: supervisor swaps again (test with one extra sibling).

- [ ] **Step 2: Implementation**

Create `internal/run/supervise.go`. Sketch:

```go
package run

import (
	"context"
	"fmt"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// HookSource provides what the supervisor needs from hook telemetry.
// All methods MUST respect ctx cancellation — the supervisor's ctrl-c
// path relies on WaitForStop returning ctx.Err() promptly rather than
// blocking until the next poll completes.
type HookSource interface {
    // CurrentSessionID returns the most recent SessionStart's session_id for
    // the given profile, or "" if none observed.
    CurrentSessionID(ctx context.Context, profile string) (string, error)
    // WaitForStop blocks until a Stop or StopFailure event arrives for the
    // given session_id, OR ctx is cancelled. On cancellation it returns
    // ctx.Err() within at most one poll interval (see DBHookSource in Task 7).
    WaitForStop(ctx context.Context, sessionID string) error
}

// StartedProcess is the handle that ChildLauncher.Start returns. It must be
// distinct from os/exec because the Supervisor needs to send a graceful
// SIGTERM before forcing a kill (a plain context cancel maps to SIGKILL on
// Unix via exec.CommandContext, which would skip claude's cleanup path).
type StartedProcess interface {
    // SignalTerminate sends SIGTERM on Unix and the equivalent on Windows.
    SignalTerminate() error
    // Kill is SIGKILL — last resort, after the graceful timeout.
    Kill() error
    // Wait blocks until the child exits and returns its exit code.
    Wait() (int, error)
}

// ChildLauncher is the test-injectable surface for starting `claude` as a
// child process. Implemented by run.Start (a thin wrapper around exec.Command
// that returns a *StartedProcess).
type ChildLauncher interface {
    Start(ctx context.Context, spec LaunchSpec) (StartedProcess, error)
}

// Supervisor orchestrates the mid-session swap.
type Supervisor struct {
    Profiles       []contracts.Profile
    Picker         func(ctx context.Context, exclude string) (contracts.Profile, string, error)
    Events         <-chan contracts.RecommendationEvent
    Hooks          HookSource
    Launcher       ChildLauncher
    BinaryPath     string
    BaseEnv        []string
    ResumeFlag     string
    Logger         func(format string, args ...any)
    // ShutdownGrace is the time we wait for the child to exit after SIGTERM
    // before falling back to SIGKILL. Zero means use defaultShutdownGrace (5s).
    ShutdownGrace  time.Duration
}

const defaultShutdownGrace = 5 * time.Second

func (s *Supervisor) Run(ctx context.Context, initial contracts.Profile, args []string) error {
    current := initial
    for {
        spec := LaunchSpec{
            BinaryPath: s.BinaryPath,
            Args:       args,
            Env:        BuildEnv(current, s.BaseEnv),
        }

        child, err := s.Launcher.Start(ctx, spec)
        if err != nil {
            return fmt.Errorf("starting claude under %s: %w", current.Name, err)
        }
        launchDone := make(chan int, 1)
        go func() {
            exit, _ := child.Wait()
            launchDone <- exit
        }()

        // Watch for a Hard event targeting `current`.
        swap := false
        for !swap {
            select {
            case <-ctx.Done():
                s.shutdown(child, launchDone)
                return ctx.Err()
            case exit := <-launchDone:
                if exit != 0 {
                    return ExitCodeError{Code: exit}
                }
                return nil
            case ev, ok := <-s.Events:
                if !ok {
                    s.Events = nil // events stream closed; continue without it
                    continue
                }
                if ev.Profile != current.Name || ev.Level != contracts.RecommendationHard {
                    continue
                }
                swap = true
            }
        }

        // Identify the session BEFORE waiting (so we know which session-id to
        // pass to --resume). WaitForStop must honor ctx (DBHookSource polls
        // and returns ctx.Err() promptly on cancel — see Task 7).
        sid, err := s.Hooks.CurrentSessionID(ctx, current.Name)
        if err != nil {
            return err
        }
        if sid != "" {
            if err := s.Hooks.WaitForStop(ctx, sid); err != nil {
                // ctx.Canceled or DeadlineExceeded propagates up cleanly.
                s.shutdown(child, launchDone)
                return err
            }
        }

        // Graceful shutdown of the current child between turns.
        s.shutdown(child, launchDone)

        next, why, err := s.Picker(ctx, current.Name)
        if err != nil {
            s.log("supervisor: cannot pick sibling after hard event on %s: %v", current.Name, err)
            return err
        }
        s.log("supervisor: swapping %s → %s (%s)", current.Name, next.Name, why)

        // Inject --resume into a fresh args slice (don't mutate the caller's).
        if sid != "" {
            resumeFlag := s.ResumeFlag
            if resumeFlag == "" {
                resumeFlag = "--resume"
            }
            args = appendResumeFlag(args, resumeFlag, sid)
        }
        current = next
    }
}

// shutdown drives the SIGTERM → wait → SIGKILL ladder, then drains launchDone.
func (s *Supervisor) shutdown(child StartedProcess, launchDone <-chan int) {
    grace := s.ShutdownGrace
    if grace <= 0 {
        grace = defaultShutdownGrace
    }
    _ = child.SignalTerminate()
    select {
    case <-launchDone:
        return
    case <-time.After(grace):
        s.log("supervisor: SIGTERM grace elapsed; sending SIGKILL")
        _ = child.Kill()
        <-launchDone
    }
}

func (s *Supervisor) log(format string, args ...any) {
    if s.Logger != nil {
        s.Logger(format, args...)
    }
}

func appendResumeFlag(args []string, flag, sid string) []string {
    // Always return a fresh slice so we don't mutate the caller's args.
    // If `flag` already appears, replace its value in the copy; otherwise
    // prepend `flag sid` so it precedes user-supplied args.
    out := make([]string, len(args))
    copy(out, args)
    for i, a := range out {
        if a == flag && i+1 < len(out) {
            out[i+1] = sid
            return out
        }
    }
    // Otherwise prepend so it precedes user args.
    return append([]string{flag, sid}, out...)
}
```

- [ ] **Step 3: Verify**

```bash
gofumpt -w internal/run/
go test -race -count=1 ./internal/run/...
golangci-lint run ./internal/run/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/run/supervise.go internal/run/supervise_test.go
git commit -m "feat(run): Supervisor wait-and-swap loop"
```

---

## Task 7: HookSource implementations (TDD)

**Files:**
- Create: `internal/run/hooksource_db.go` (poll-based; no daemon)
- Create: `internal/run/hooksource_sse.go` (subscribe-based; via daemon SSE)
- Create: `internal/run/hooksource_test.go`

- [ ] **Step 1: Failing tests** for both implementations, asserting that:

- DB poller's `CurrentSessionID` returns the right session id from a seeded store.
- DB poller's `WaitForStop` returns nil when a Stop event for that session_id appears within the poll interval.
- **DB poller's `WaitForStop` returns `ctx.Err()` (i.e. `context.Canceled`) within one poll interval after the caller cancels ctx.** (Regression test for the I-5 fix — without this, Ctrl-C during a supervisor wait would hang.)
- SSE poller (using `httptest`) handles `recommendation` events streamed from a fake server.

- [ ] **Step 2: Implementation**

`hooksource_db.go`:

```go
// DBHookSource implements HookSource by polling state.db. It is used when
// the supervisor runs offline (no daemon) and as the fallback for
// SSEHookSource.WaitForStop (the SSE stream only carries pressure events,
// not raw Stop events).
type DBHookSource struct {
    Store        QueryHookEventsStore // narrow interface — *storage.Store satisfies it
    PollInterval time.Duration        // default 2s
}

type QueryHookEventsStore interface {
    QuerySessions(ctx context.Context, q contracts.SessionQuery) ([]contracts.SessionTelemetry, error)
    QueryHookEventsForSession(ctx context.Context, sessionID string, since time.Time) ([]contracts.HookEvent, error)
}

func (h *DBHookSource) pollInterval() time.Duration {
    if h.PollInterval > 0 {
        return h.PollInterval
    }
    return 2 * time.Second
}

func (h *DBHookSource) CurrentSessionID(ctx context.Context, profile string) (string, error) {
    rows, err := h.Store.QuerySessions(ctx, contracts.SessionQuery{Profile: profile, Limit: 1})
    if err != nil {
        return "", err
    }
    if len(rows) == 0 {
        return "", nil
    }
    return rows[0].Session, nil
}

// WaitForStop polls hook_events for the given session_id until a Stop or
// StopFailure event lands, or ctx is cancelled. Returns ctx.Err() promptly
// on cancellation — never blocks the caller beyond one PollInterval.
func (h *DBHookSource) WaitForStop(ctx context.Context, sessionID string) error {
    since := time.Now().UTC()
    ticker := time.NewTicker(h.pollInterval())
    defer ticker.Stop()
    for {
        // Always check ctx first so a cancellation that lands between ticks
        // still returns promptly.
        if err := ctx.Err(); err != nil {
            return err
        }
        events, err := h.Store.QueryHookEventsForSession(ctx, sessionID, since)
        if err != nil {
            return err
        }
        for _, ev := range events {
            if ev.Event == "Stop" || ev.Event == "StopFailure" {
                return nil
            }
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
        }
    }
}
```

`hooksource_sse.go` wraps a `GET /api/recommendations/live` subscription for the `Events` channel. For `WaitForStop`, it delegates to an embedded `DBHookSource` (the SSE stream only carries pressure events, not raw Stop events).

You'll need a new query on `*storage.Store`:

```go
func (s *Store) QueryHookEventsForSession(ctx context.Context, sessionID string, since time.Time) ([]contracts.HookEvent, error)
```

This is a small additive method. Add it in this task with its own test.

**Poll-interval note (M-7):** the default 2s means there is a 0–2s delay
between the moment Claude Code fires its `Stop` hook and the moment the
supervisor sees it and proceeds with the swap. This is acceptable for the
supervisor's "between turns" UX — a few seconds of latency is invisible next
to claude's own model-response time. Document this in `docs/troubleshooting.md`
("supervisor delays the swap by up to 2 seconds; lower via `--poll-interval`
flag if you must — but stay above 250ms to avoid hammering SQLite").

- [ ] **Step 3: Verify + commit (one per file)**

```bash
git add internal/storage/hooks.go internal/storage/hooks_test.go
git commit -m "feat(storage): QueryHookEventsForSession"

git add internal/run/hooksource_db.go internal/run/hooksource_sse.go internal/run/hooksource_test.go
git commit -m "feat(run): DB and SSE HookSource implementations for the supervisor"
```

---

## Task 8: `ccx run --supervise` CLI flag

**Files:**
- Modify: `internal/cli/run.go`
- Modify: `internal/cli/run_test.go`

- [ ] **Step 1: Add `--supervise` flag**

```go
cmd.Flags().BoolVar(&supervise, "supervise", false, "stay attached and mid-session swap on hard pressure")
```

In `RunE`, branch:

```go
if supervise {
    // Construct Supervisor with DBHookSource (always) + Hub (if a daemon hub
    // is reachable; else nil events channel from SSE poller).
    hooks := &run.DBHookSource{Store: deps.Store.(*storage.Store)}
    // SSE source (best-effort; ignore errors and use empty channel).
    sseEvents, _ := run.OpenSSE(ctx, "http://127.0.0.1:7777/api/recommendations/live")
    sup := &run.Supervisor{
        Profiles:   profiles,
        Picker:     func(ctx context.Context, exclude string) (contracts.Profile, string, error) {
            filtered := excludeProfile(profiles, exclude)
            return run.Pick(ctx, run.PickOptions{
                Profiles:  filtered,
                Evaluator: adapter,
            })
        },
        Events:     sseEvents,
        Hooks:      hooks,
        Launcher:   run.LauncherFunc(run.Launch),
        BinaryPath: binary,
        BaseEnv:    os.Environ(),
        ResumeFlag: "--resume", // TODO: confirmed in Gate 0.2; update if different
        Logger:     func(f string, a ...any) { fmt.Fprintf(c.ErrOrStderr(), "ccx: "+f+"\n", a...) },
    }
    return sup.Run(ctx, profile, args)
}
```

Add `excludeProfile` helper and `OpenSSE` (returns a `<-chan contracts.RecommendationEvent` from an SSE GET; closes on ctx.Done).

- [ ] **Step 2: Tests, verify, commit**

```bash
gofumpt -w internal/cli/run.go
go test -race -count=1 ./internal/cli/...
golangci-lint run ./internal/cli/...
git add internal/cli/run.go internal/cli/run_test.go internal/run/sse.go internal/run/sse_test.go
git commit -m "feat(cli): ccx run --supervise mid-session swap"
```

---

## Task 9: Manual integration smoke + verification

**Goal:** prove end-to-end that the supervisor swap works on a real `claude` install. This task is **manual** and produces evidence (logs, transcripts) that get pasted into the PR description.

- [ ] **Step 1: Set up the test rig**

```bash
# Two profiles, both pointing at real Claude Code config dirs (use throwaway ones).
./dist/ccx profile add demo-a --config-dir ~/.claude-profiles/demo-a --plan-tier max5 --caps-5h-turns 1
./dist/ccx profile add demo-b --config-dir ~/.claude-profiles/demo-b --plan-tier max5
./dist/ccx hooks install --profile demo-a
./dist/ccx hooks install --profile demo-b
./dist/ccx migrate-shared-history
```

- [ ] **Step 2: Start the daemon and the supervisor**

```bash
./dist/ccx daemon start
./dist/ccx run --supervise --profile demo-a -- -p "tell me a joke"
```

- [ ] **Step 3: Observe the swap**

Expected ccx stderr:

```
ccx: explicit --profile demo-a → profile=demo-a config_dir=...
... (claude runs, produces a turn, Stop fires)
ccx: supervisor: swapping demo-a → demo-b (headroom recommendation: score=...)
ccx: launching /path/to/claude with args ["--resume" "<sid>" "-p" "tell me a joke"]
... (claude resumes with the same session, under demo-b)
```

- [ ] **Step 4: Confirm conversation continuity**

In the new claude session, ask a follow-up that depends on the previous turn's context (`"now tell it again but funnier"`). If claude responds coherently, history transfer worked.

If history did **not** transfer (claude starts a fresh session), investigate:

- Is the `--resume` flag right? (Re-check Gate 0.2.)
- Did the symlinks actually take effect? (`ls -la ~/.claude-profiles/demo-{a,b}/projects`)
- Is the session JSONL actually at `~/.ccx/shared-projects/<encoded-cwd>/<sid>.jsonl`?

Document findings in the PR description.

- [ ] **Step 5: Stop**

```bash
./dist/ccx daemon stop
```

---

## Task 10: Final verification + PR

- [ ] **Step 1: `make ci`** + web + integration test (the new shared-scan path warrants an integration test).

```bash
make ci
make integration-test
cd web && pnpm test && pnpm typecheck && cd ..
```

- [ ] **Step 2: Commit log inspection**

Expected ~10 commits across `quotamigrate`, `cli`, `scanner`, `storage`, `run`.

- [ ] **Step 3: Push + PR**

```bash
git push -u origin feat/quota-supervisor
gh pr create \
  --base main \
  --title "feat(quota): supervisor + shared history (v0.2 B3b)" \
  --body "$(cat <<'EOF'
## Summary

Mid-session profile swap. When the active profile crosses hard pressure, the
supervisor waits for the next Stop hook event, kills `claude`, and relaunches
under the next-best-headroom sibling with `claude --resume <session-id>`.
Conversation history survives because every profile's projects/ is symlinked
to a single shared directory.

- **internal/quotamigrate**: Plan + Apply for the shared-projects symlink layout
- **ccx migrate-shared-history**: opt-in migration for existing v0.1 profiles (with `--dry-run`)
- **ccx profile add**: auto-symlinks new profiles
- **internal/scanner.ScanShared**: walks the shared dir once, attributes events via the sessions table
- **internal/run.Supervisor**: wait-and-swap loop
- **internal/run.{DBHookSource, SSEHookSource}**: session-id lookup and Stop-event blocking
- **ccx run --supervise**: opt-in supervisor mode

Task 0 verification findings (Claude Code symlink behavior, --resume flag, hook payload coverage) recorded in this PR's description.

Spec: docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md
Plan: docs/superpowers/plans/2026-05-24-ccx-quota-B3b-supervisor-shared-history.md

## Task 0 verification findings

- Gate 0.1 (symlink): <PASS|FAIL — paste evidence>
- Gate 0.2 (resume): <flag name observed>
- Gate 0.3 (headers): <PASS|FAIL — paste sample>

## Test plan

- [x] `make ci` green
- [x] `make integration-test` green
- [x] `pnpm test` and `pnpm typecheck` green
- [x] Manual: migrate-shared-history --dry-run shows the planned symlinks
- [x] Manual: ccx run --supervise swaps demo-a → demo-b after one Stop with Caps5hTurns=1
- [x] Manual: post-swap conversation continues (paste transcript)
EOF
)"
```

- [ ] **Step 4: After merge, update plan index status**

Mark **B3b** row `✅ Merged in #<PR-number>`. Update the v0.2.0 release tag in a follow-up.

---

## Verification criteria (definition of done)

1. **`internal/quotamigrate/`** exports `Plan`, `Apply`, `Step`, `Action`, `SharedProjectsPath`, `ProfileProjectsPath`. Plan returns zero steps when already migrated. Apply is idempotent.

2. **`ccx migrate-shared-history [--dry-run]`** prints the plan and (without `--dry-run`) applies it. Errors out cleanly on symlinks pointing to unexpected targets (refuses to overwrite).

3. **`ccx profile add`** creates the shared symlink for the new profile after registry write.

4. **`scanner.ScanShared(ctx, projectsRoot, lookup)`** walks the shared dir once and emits `AttributedEvent` values. Events for sessions without a `sessions` row are skipped (not errored).

5. **`*storage.Store`** has new methods `ProfileForSession` and `QueryHookEventsForSession`.

6. **Daemon and CLI ingest** detect the shared-symlink layout and use `ScanShared`; fall back to per-profile `Scan` otherwise.

7. **`internal/run.Supervisor`** runs claude under the initial profile, watches an event channel for `Hard` recommendations targeting the active profile, waits for the next Stop, swaps to the next-best-headroom sibling, and relaunches with `--resume <session-id>`.

8. **`ccx run --supervise`** wires everything together and exits with the final child's exit code.

9. **Task 0 verification** has been performed and its findings are recorded in the PR description and (for symlink/resume) the spec is updated if needed.

10. **No frozen files modified.**

11. **PR merged to `main`** with green CI. Plan index updated. v0.2.0 tag pushed in a follow-up.

---

## Rollback

This phase is the most invasive — rollback is correspondingly more careful:

- **Migration applied wrong**: every profile's `<config_dir>/projects/` is now a symlink. To revert, the user can manually:
  ```bash
  rm <config_dir>/projects                              # remove the symlink only (not contents)
  mv ~/.ccx/shared-projects <config_dir>/projects       # if only one profile, simplest path
  # OR copy the relevant subtrees back per profile; the sessions table tells you which session belongs to which profile.
  ```
  We do **not** ship an automated `ccx unmigrate-shared-history` in v0.2; if real users hit this, add it in v0.2.1.

- **Supervisor causes claude to crash on swap**: users can stop using `--supervise` immediately; plain `ccx run` continues to work, as does the v0.1 manual switch path.

- **Scanner ScanShared misattributes events**: the daemon's existing per-profile `Scan` fallback can be force-enabled via an env var (`CCX_SCANNER_FORCE_PER_PROFILE=1`). Add this escape hatch in Task 5 if not already present.

- **Whole phase reverts**: revert the merge commit. The symlinks on disk remain (the rollback above explains how to manually undo). The `internal/quotamigrate` package can be left in the repo — it's harmless without callers. Future agents shouldn't see this as half-done because the `ccx migrate-shared-history` command is gone; rollback should also remove that command's registration.

If revert is needed mid-release, prefer leaving the contract amendments (P0) in place — they're inert without consumers, and v0.3 may want them.
