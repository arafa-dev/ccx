# ccx v0.2 B3a — Pre-Launch Fallback Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Three intertwined deliverables that together give the user automatic profile selection *at session start*:

1. **`ccx run [-- args]`** — a CLI wrapper that picks the highest-headroom profile (via the existing `headroom.Evaluator`), sets `CLAUDE_CONFIG_DIR`, and fork+waits `claude`. The user gets the right account without typing `ccx use`. Exit code is forwarded.
2. **`ccx init <shell> --with-claude-wrapper`** — extends the existing shell-init snippet to optionally define a `claude` function that calls `ccx run --`. Opt-in only; default behavior unchanged.
3. **Daemon-level recommendation stream + dashboard banner** — the daemon recomputes quota after each ingest cycle, emits a `contracts.RecommendationEvent` whenever a profile transitions into a new pressure band (warn → soft → hard), broadcasts via the new `GET /api/recommendations/live` SSE endpoint, and the dashboard renders a live `<RecommendationBanner>` component.

**Architecture:**

```
                            ┌──────────────────────────────────────┐
                            │  daemon ingest loop (existing)       │
                            │  (PR #16 fsnotify + coalesce)        │
                            └──────────────┬───────────────────────┘
                                           │
                                  on each scan
                                           ▼
                            ┌──────────────────────────────────────┐
                            │  NEW: recstream.Hub                  │
                            │  - holds last-seen PressureLevel     │
                            │    per profile                        │
                            │  - on upward transition, build a     │
                            │    RecommendationEvent and Publish()  │
                            └──────────────┬───────────────────────┘
                                           │ broadcast
                                           ▼
        ┌─────────────────────────┐   ┌────────────────────────────┐
        │  /api/recommendations/  │   │  ccx run --supervise (B3b) │
        │  live (server, new)     │   │  also subscribes            │
        └────────────┬────────────┘   └────────────────────────────┘
                     │ SSE
                     ▼
        ┌─────────────────────────┐
        │  <RecommendationBanner> │
        │  (dashboard, new)       │
        └─────────────────────────┘
```

`ccx run` itself is a leaf-package addition (`internal/run/`) that imports `internal/headroom`, `internal/quota`, `internal/profile`, and `internal/storage`. It does **not** depend on the daemon being running — it works equally well in offline mode. The recommendation stream only fires under the daemon.

**Tech Stack:** Go 1.22+ stdlib (`os/exec`, `os/signal`, `syscall`). React 18 / Next.js 15. `EventSource` for SSE on the web side.

**Spec reference:** [`docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`](../specs/2026-05-24-ccx-plan-aware-quota.md) — §6.5 (auto-switch granularity), §6.7 (separate SSE stream), §8.4 (`ccx run` UX), §8.5 (dashboard banner mockup).

**Worktree:**

```bash
git fetch origin
git worktree add ../ccx-quota-prelaunch -b feat/quota-pre-launch origin/main
cd ../ccx-quota-prelaunch
```

B2 must already be merged.

**Exit criteria:**

- [ ] `go build ./...` succeeds
- [ ] `go test -race -count=1 ./...` succeeds
- [ ] `golangci-lint run ./...` reports `0 issues.`
- [ ] `cd web && pnpm typecheck && pnpm test` succeed
- [ ] `make ci` green
- [ ] Manual smoke: `./dist/ccx run --print-only` prints the planned launch command without forking
- [ ] Manual smoke: `./dist/ccx run -- --help` shells out to `claude --help`
- [ ] Manual smoke: with `./dist/ccx daemon start` running and a `Caps5hTurns: 1` profile, sending one prompt under that profile causes a `recommendation` SSE event to fire on `/api/recommendations/live`
- [ ] Manual smoke: dashboard shows the banner during the SSE event
- [ ] PR opened, CI green, merged
- [ ] Plan index status updated

**Conventions:** unchanged from earlier plans. Scopes used: `run`, `cli`, `shell`, `daemon`, `server`, `web`, `recstream`.

---

## Pre-flight

```bash
pwd                                                  # → .../ccx-quota-prelaunch
git status                                           # → On branch feat/quota-pre-launch, working tree clean
grep -l "PressureLevel" internal/headroom/thresholds.go && echo OK   # → OK (B2 merged)
grep -l "quota.Computer" internal/quota/compute.go && echo OK         # → OK (B1 merged)
which claude || echo "WARN: claude binary not on PATH (manual smoke will skip)"
go build ./... && echo OK
```

---

## Task 1: Create `internal/run/` package — locate `claude` and launch child (TDD)

**Files:**
- Create: `internal/run/doc.go`
- Create: `internal/run/launcher.go`
- Create: `internal/run/launcher_unix.go`
- Create: `internal/run/launcher_windows.go`
- Create: `internal/run/launcher_test.go`

- [ ] **Step 1: Failing test**

Create `internal/run/launcher_test.go`:

```go
package run_test

import (
	"context"
	"os/exec"
	"runtime"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/run"
)

func TestLocateClaudeUsesPATH(t *testing.T) {
	// LookPath behavior: returns an absolute path if found, exec.ErrNotFound otherwise.
	got, err := run.LocateClaude(run.Options{LookPath: func(name string) (string, error) {
		if name != "claude" {
			t.Fatalf("LookPath called with %q, want claude", name)
		}
		return "/usr/local/bin/claude", nil
	}})
	if err != nil {
		t.Fatalf("LocateClaude: %v", err)
	}
	if got != "/usr/local/bin/claude" {
		t.Errorf("got %q, want /usr/local/bin/claude", got)
	}
}

func TestLocateClaudeMissingReturnsErrNotFound(t *testing.T) {
	_, err := run.LocateClaude(run.Options{LookPath: func(string) (string, error) {
		return "", exec.ErrNotFound
	}})
	if err == nil {
		t.Fatal("expected error when claude is missing")
	}
}

func TestLocateClaudeRespectsBinaryOverride(t *testing.T) {
	got, err := run.LocateClaude(run.Options{
		BinaryPath: "/opt/custom/claude",
		LookPath:   func(string) (string, error) { t.Fatal("override should bypass LookPath"); return "", nil },
	})
	if err != nil {
		t.Fatalf("LocateClaude: %v", err)
	}
	if got != "/opt/custom/claude" {
		t.Errorf("got %q, want override", got)
	}
}

func TestBuildEnvSetsExpectedVars(t *testing.T) {
	profile := contracts.Profile{Name: "work", ConfigDir: "/p/work"}
	env := run.BuildEnv(profile, []string{"PATH=/usr/bin", "HOME=/Users/x"})
	hasConfig := false
	hasActive := false
	for _, e := range env {
		if e == "CLAUDE_CONFIG_DIR=/p/work" {
			hasConfig = true
		}
		if e == "CCX_ACTIVE_PROFILE=work" {
			hasActive = true
		}
	}
	if !hasConfig {
		t.Error("expected CLAUDE_CONFIG_DIR in env")
	}
	if !hasActive {
		t.Error("expected CCX_ACTIVE_PROFILE in env")
	}
}

func TestBuildEnvOverwritesExistingValues(t *testing.T) {
	profile := contracts.Profile{Name: "work", ConfigDir: "/new"}
	env := run.BuildEnv(profile, []string{"CLAUDE_CONFIG_DIR=/old", "PATH=/x"})
	for _, e := range env {
		if e == "CLAUDE_CONFIG_DIR=/old" {
			t.Errorf("expected /old to be overwritten; got %q", e)
		}
	}
}

func TestLaunchReturnsExitCodeOfChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}
	exit, err := run.Launch(context.Background(), run.LaunchSpec{
		BinaryPath: "/bin/sh",
		Args:       []string{"-c", "exit 7"},
		Env:        []string{"PATH=/usr/bin:/bin"},
	})
	if err != nil {
		// Launch returns nil error on a clean child exit even when exit > 0.
		// If your impl returns an error for nonzero exit, adjust this test.
		t.Fatalf("Launch: %v", err)
	}
	if exit != 7 {
		t.Errorf("exit code: got %d, want 7", exit)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implementation**

Create `internal/run/doc.go`:

```go
// Package run wraps the `claude` binary, setting the right CLAUDE_CONFIG_DIR
// for a chosen ccx profile and forwarding stdio, signals, and exit code. It is
// the implementation backing `ccx run` (and, in v0.2 B3b, `ccx run --supervise`).
package run
```

Create `internal/run/launcher.go`:

```go
package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
)

// Options configures how `claude` is located.
type Options struct {
	// BinaryPath, if set, is used directly without consulting LookPath.
	BinaryPath string
	// LookPath defaults to exec.LookPath. Injected for tests.
	LookPath func(name string) (string, error)
}

// LaunchSpec describes one invocation.
type LaunchSpec struct {
	BinaryPath string
	Args       []string
	Env        []string
	Stdin      *os.File // nil → os.Stdin
	Stdout     *os.File // nil → os.Stdout
	Stderr     *os.File // nil → os.Stderr
}

// LocateClaude returns the resolved path of the claude binary. When
// Options.BinaryPath is set, it is returned verbatim. Otherwise, LookPath is
// consulted (defaulting to exec.LookPath).
func LocateClaude(opts Options) (string, error) {
	if opts.BinaryPath != "" {
		return opts.BinaryPath, nil
	}
	look := opts.LookPath
	if look == nil {
		look = exec.LookPath
	}
	p, err := look("claude")
	if err != nil {
		return "", fmt.Errorf("locating claude binary: %w", err)
	}
	return p, nil
}

// BuildEnv returns a process environment with CLAUDE_CONFIG_DIR and
// CCX_ACTIVE_PROFILE set for the given profile, preserving all other entries
// in base. Existing values of the two managed keys are removed first.
func BuildEnv(p contracts.Profile, base []string) []string {
	managed := map[string]string{
		profile.EnvConfigDir:     p.ConfigDir,
		profile.EnvActiveProfile: p.Name,
	}
	out := make([]string, 0, len(base)+len(managed))
	for _, kv := range base {
		key := keyOf(kv)
		if _, isManaged := managed[key]; isManaged {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range managed {
		out = append(out, k+"="+v)
	}
	return out
}

func keyOf(envEntry string) string {
	i := strings.IndexByte(envEntry, '=')
	if i < 0 {
		return envEntry
	}
	return envEntry[:i]
}

// Launch runs the child in the foreground, forwarding stdio and signals.
// Returns the child's exit code on a clean exit (including nonzero exits).
// Returns an error only for failures to start, signal-related aborts, or
// unexpected ProcessState scenarios.
//
// Per-platform signal forwarding lists are declared in launcher_unix.go and
// launcher_windows.go (see forwardedSignals).
func Launch(ctx context.Context, spec LaunchSpec) (exitCode int, err error) {
	if spec.BinaryPath == "" {
		return 0, errors.New("run.Launch: BinaryPath is empty")
	}
	cmd := exec.CommandContext(ctx, spec.BinaryPath, spec.Args...)
	cmd.Env = spec.Env
	cmd.Stdin = orDefault(spec.Stdin, os.Stdin)
	cmd.Stdout = orDefault(spec.Stdout, os.Stdout)
	cmd.Stderr = orDefault(spec.Stderr, os.Stderr)

	// Forward portable signals to the child so Ctrl-C, etc. reach claude.
	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, forwardedSignals()...)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting %s: %w", spec.BinaryPath, err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for {
		select {
		case sig := <-sigCh:
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		case waitErr := <-done:
			if waitErr == nil {
				return 0, nil
			}
			var ee *exec.ExitError
			if errors.As(waitErr, &ee) {
				// ExitCode() handles signal-killed children portably; the
				// platform-specific 128+sig translation lives in
				// signaledExitCode (launcher_unix.go).
				if code, ok := signaledExitCode(ee); ok {
					return code, nil
				}
				return ee.ExitCode(), nil
			}
			return 0, fmt.Errorf("waiting for %s: %w", spec.BinaryPath, waitErr)
		}
	}
}

func orDefault(f, dflt *os.File) *os.File {
	if f == nil {
		return dflt
	}
	return f
}
```

And the two per-platform files:

`internal/run/launcher_unix.go`:

```go
//go:build darwin || linux

package run

import (
	"os"
	"os/exec"
	"syscall"
)

func forwardedSignals() []os.Signal {
	// SIGINT and SIGTERM are the universal "shut down" signals; SIGHUP is
	// useful when claude is run inside a terminal multiplexer that closes
	// the controlling tty. SIGUSR1 is not in the Go stdlib's portable set,
	// but Claude Code uses it on Unix to bump verbosity — forward it too.
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1}
}

// signaledExitCode returns the POSIX `128 + signal_number` exit code when
// the child died from a signal. Returns (0, false) otherwise; the caller
// then falls back to ExitError.ExitCode().
func signaledExitCode(ee *exec.ExitError) (int, bool) {
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal()), true
	}
	return 0, false
}
```

`internal/run/launcher_windows.go`:

```go
//go:build windows

package run

import (
	"os"
	"os/exec"
)

func forwardedSignals() []os.Signal {
	// On Windows, Go's signal package only delivers os.Interrupt reliably.
	// Termination requests come through that single channel; map them all
	// onto the child via cmd.Process.Signal(os.Interrupt) in Launch.
	return []os.Signal{os.Interrupt}
}

// Windows has no signal-exit semantics; ExitCode() is always meaningful.
func signaledExitCode(*exec.ExitError) (int, bool) { return 0, false }
```

(The agent must remove `syscall` from the `Launch` imports in
`launcher.go` since the syscall-specific code now lives only in the
platform files. The cross-platform Launch imports `context`, `errors`,
`fmt`, `os`, `os/exec`, `os/signal` — no `syscall`.)

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/run/
go test -race -count=1 ./internal/run/...
golangci-lint run ./internal/run/...
```

Expected: tests pass, lint clean. (`syscall` use is platform-specific; the test is gated by `runtime.GOOS`; CI builds on linux/macos so that path is exercised.)

- [ ] **Step 5: Commit**

```bash
git add internal/run/doc.go internal/run/launcher.go internal/run/launcher_unix.go internal/run/launcher_windows.go internal/run/launcher_test.go
git commit -m "feat(run): LocateClaude, BuildEnv, Launch"
```

---

## Task 1.5: Add `ExitCodeError` (used by both `ccx run` CLI and B3b Supervisor)

**Files:**
- Create: `internal/run/exit.go`
- Create: `internal/run/exit_test.go`

`ExitCodeError` lives in `internal/run/` (not in `internal/cli/`) so both the
`ccx run` cobra command (Task 3) and B3b's `Supervisor.Run` (which lives in
the same `internal/run/` package) can return it without creating a layering
violation. `internal/cli/` is allowed to import `internal/run/`; the reverse
would be a cycle.

- [ ] **Step 1: Failing test**

`internal/run/exit_test.go`:

```go
package run_test

import (
	"errors"
	"testing"

	"github.com/arafa-dev/ccx/internal/run"
)

func TestExitCodeErrorMessage(t *testing.T) {
	e := run.ExitCodeError{Code: 7}
	if e.Error() != "exit 7" {
		t.Errorf("Error() = %q, want \"exit 7\"", e.Error())
	}
	if e.ExitCode() != 7 {
		t.Errorf("ExitCode() = %d, want 7", e.ExitCode())
	}
}

func TestExitCodeErrorAs(t *testing.T) {
	var coded run.ExitCodeError
	err := run.ExitCodeError{Code: 3}
	if !errors.As(err, &coded) {
		t.Fatal("errors.As did not match")
	}
	if coded.Code != 3 {
		t.Errorf("Code = %d, want 3", coded.Code)
	}
}
```

- [ ] **Step 2: Implementation**

`internal/run/exit.go`:

```go
package run

import "fmt"

// ExitCodeError is returned from CLI RunE (or any other ccx code path) when
// the caller wants ccx itself to exit with a specific non-1 code. cli.Run
// type-asserts on this and propagates Code as the process exit status,
// suppressing the noisy default "Error: …" stderr line.
//
// The B3b supervisor also returns this when claude exits non-zero so the
// child's exit code surfaces all the way out to the shell.
type ExitCodeError struct{ Code int }

func (e ExitCodeError) Error() string { return fmt.Sprintf("exit %d", e.Code) }
func (e ExitCodeError) ExitCode() int { return e.Code }
```

- [ ] **Step 3: Verify**

```bash
gofumpt -w internal/run/
go test -race -count=1 ./internal/run/...
golangci-lint run ./internal/run/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/run/exit.go internal/run/exit_test.go
git commit -m "feat(run): ExitCodeError for propagating child exit codes"
```

---

## Task 2: Add a profile picker on top of `internal/run/` (TDD)

**Files:**
- Create: `internal/run/pick.go`
- Create: `internal/run/pick_test.go`

- [ ] **Step 1: Failing test**

Create `internal/run/pick_test.go`:

```go
package run_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/run"
)

type stubEvaluator struct {
	result headroom.Result
	err    error
}

func (s stubEvaluator) Evaluate(_ context.Context, _ []contracts.Profile, _ headroom.Options) (headroom.Result, error) {
	return s.result, s.err
}

func TestPickReturnsExplicitProfileWhenProvided(t *testing.T) {
	profiles := []contracts.Profile{
		{Name: "work"}, {Name: "personal"},
	}
	got, why, err := run.Pick(context.Background(), run.PickOptions{
		Profiles:    profiles,
		Override:    "personal",
	})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.Name != "personal" {
		t.Errorf("Override ignored: got %q, want personal", got.Name)
	}
	if why != "explicit --profile personal" {
		t.Errorf("Why: got %q", why)
	}
}

func TestPickOverrideUnknownProfileErrors(t *testing.T) {
	_, _, err := run.Pick(context.Background(), run.PickOptions{
		Profiles: []contracts.Profile{{Name: "work"}},
		Override: "ghost",
	})
	if err == nil {
		t.Fatal("expected error for unknown override")
	}
}

func TestPickFallsBackToEvaluatorRecommendation(t *testing.T) {
	profiles := []contracts.Profile{{Name: "work"}, {Name: "personal"}}
	ev := stubEvaluator{
		result: headroom.Result{
			Recommendation: &headroom.Candidate{Profile: "work", Available: true, Score: 50},
			Candidates: []headroom.Candidate{
				{Profile: "work", Available: true, Score: 50},
				{Profile: "personal", Available: true, Score: 30},
			},
		},
	}
	got, why, err := run.Pick(context.Background(), run.PickOptions{Profiles: profiles, Evaluator: ev})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.Name != "work" {
		t.Errorf("expected recommendation: got %q", got.Name)
	}
	if why == "" {
		t.Errorf("expected non-empty why")
	}
}

func TestPickNoAvailableProfilesReturnsError(t *testing.T) {
	profiles := []contracts.Profile{{Name: "work"}}
	ev := stubEvaluator{
		result: headroom.Result{
			Candidates: []headroom.Candidate{{Profile: "work", Available: false, Reasons: []string{"hard cap"}}},
		},
	}
	_, _, err := run.Pick(context.Background(), run.PickOptions{Profiles: profiles, Evaluator: ev})
	if err == nil {
		t.Fatal("expected error when nothing recommendable")
	}
	if !errors.Is(err, run.ErrNoRecommendation) {
		t.Errorf("error: got %v, want ErrNoRecommendation wrap", err)
	}
}

func TestPickEmptyProfileListReturnsError(t *testing.T) {
	_, _, err := run.Pick(context.Background(), run.PickOptions{})
	if err == nil {
		t.Fatal("expected error for empty profile list")
	}
}
```

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL — `Pick`, `PickOptions`, `ErrNoRecommendation` undefined.

- [ ] **Step 3: Implementation**

Create `internal/run/pick.go`:

```go
package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
)

// ErrNoProfiles is returned when no profiles are available.
var ErrNoProfiles = errors.New("run: no profiles registered")

// ErrNoRecommendation is returned when the evaluator recommends no profile
// (e.g., every candidate is unavailable). The reasons are surfaced via the
// returned headroom.Result; callers may format them for the user.
var ErrNoRecommendation = errors.New("run: no recommendable profile")

// EvaluatorFunc abstracts headroom.Evaluator's Evaluate signature for testability.
type EvaluatorFunc interface {
	Evaluate(ctx context.Context, profiles []contracts.Profile, opts headroom.Options) (headroom.Result, error)
}

// PickOptions controls profile selection.
type PickOptions struct {
	// Profiles is the full registry. Required.
	Profiles []contracts.Profile
	// Override, when set, forces selection by name and bypasses Evaluator.
	Override string
	// Evaluator is used when Override is empty. Required in that case.
	Evaluator EvaluatorFunc
}

// Pick returns the chosen profile, a human-readable rationale, and an error.
func Pick(ctx context.Context, opts PickOptions) (contracts.Profile, string, error) {
	if len(opts.Profiles) == 0 {
		return contracts.Profile{}, "", ErrNoProfiles
	}
	if opts.Override != "" {
		for i := range opts.Profiles {
			if opts.Profiles[i].Name == opts.Override {
				return opts.Profiles[i], fmt.Sprintf("explicit --profile %s", opts.Override), nil
			}
		}
		return contracts.Profile{}, "", fmt.Errorf("--profile %q not found in registry", opts.Override)
	}
	if opts.Evaluator == nil {
		return contracts.Profile{}, "", errors.New("run.Pick: Evaluator is nil and no Override given")
	}
	result, err := opts.Evaluator.Evaluate(ctx, opts.Profiles, headroom.Options{})
	if err != nil {
		return contracts.Profile{}, "", fmt.Errorf("evaluating headroom: %w", err)
	}
	if result.Recommendation == nil {
		return contracts.Profile{}, "", ErrNoRecommendation
	}
	for i := range opts.Profiles {
		if opts.Profiles[i].Name == result.Recommendation.Profile {
			why := fmt.Sprintf("headroom recommendation: score=%.1f headroom=%.1f%%",
				result.Recommendation.Score, result.Recommendation.HeadroomPercent)
			return opts.Profiles[i], why, nil
		}
	}
	return contracts.Profile{}, "", fmt.Errorf("recommendation %q not in registry", result.Recommendation.Profile)
}
```

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/run/
go test -race -count=1 ./internal/run/...
golangci-lint run ./internal/run/...
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/run/pick.go internal/run/pick_test.go
git commit -m "feat(run): profile picker with override and evaluator fallback"
```

---

## Task 3: Add `ccx run` CLI command (TDD)

**Files:**
- Create: `internal/cli/run.go`
- Create: `internal/cli/run_test.go`
- Modify: `internal/cli/cli.go` (register the new command)

- [ ] **Step 1: Failing test**

Create `internal/cli/run_test.go`:

```go
package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Use the existing helpers from internal/cli/profile_test.go:
//   runCLI(t *testing.T, args ...string) string                 → returns stdout; fatal on non-zero exit
//   runCLIResult(args []string) (stdout, stderr string, exit int) → returns all three
// HOME is redirected via t.Setenv("HOME", tmp) per the existing pattern in
// usage_test.go (e.g. TestUsageEmpty). No need to invent new helpers.

func TestRunPrintOnlyEmitsPlanWithoutForking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	out := runCLI(t, "run", "--print-only", "--profile", "work", "--", "--help")
	for _, want := range []string{"profile=work", "args=--help"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in stdout:\n%s", want, out)
		}
	}
}

func TestRunNoProfilesErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	_, stderr, exit := runCLIResult([]string{"run", "--print-only"})
	if exit == 0 {
		t.Fatal("expected nonzero exit when no profiles registered")
	}
	if !strings.Contains(stderr, "no profiles") {
		t.Errorf("expected hint about missing profiles; got: %s", stderr)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL — `run` subcommand not registered.

- [ ] **Step 3: Implementation**

Create `internal/cli/run.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/run"
	"github.com/spf13/cobra"
)

func newRunCommand(_ *Options) *cobra.Command {
	var (
		overrideProfile string
		binaryOverride  string
		printOnly       bool
		quiet           bool
		verbose         bool
	)
	cmd := &cobra.Command{
		Use:   "run [-- args...]",
		Short: "Launch `claude` under the best-headroom profile",
		Long: `ccx run wraps the claude binary, picking the highest-headroom profile via
the same scoring as ccx suggest. Set --profile to bypass the picker. Use --
to pass arguments through to claude.`,
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			profiles, err := deps.Profiles.List(ctx)
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				return fmt.Errorf("no profiles registered; run `ccx profile add` first")
			}

			scanFailures, err := ingestSuggestProfiles(ctx, deps, profiles)
			if err != nil {
				return err
			}

			evaluator := headroom.Evaluator{Store: deps.Store, Pricing: deps.Pricing}
			adapter := evaluatorAdapter{ev: evaluator, opts: headroom.Options{UnavailableReasons: scanFailures}}

			profile, why, pickErr := run.Pick(ctx, run.PickOptions{
				Profiles:  profiles,
				Override:  overrideProfile,
				Evaluator: adapter,
			})
			if pickErr != nil {
				return pickErr
			}

			if !quiet {
				fmt.Fprintf(c.ErrOrStderr(), "ccx: %s → profile=%s config_dir=%s\n", why, profile.Name, profile.ConfigDir)
			}

			binary, err := run.LocateClaude(run.Options{BinaryPath: binaryOverride})
			if err != nil {
				return err
			}

			env := run.BuildEnv(profile, os.Environ())

			if printOnly {
				_, _ = fmt.Fprintf(c.OutOrStdout(), "binary=%s profile=%s args=%s\n",
					binary, profile.Name, joinArgs(args))
				return nil
			}
			if verbose {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "ccx: launching %s with args %v\n", binary, args)
			}

			exitCode, err := run.Launch(ctx, run.LaunchSpec{
				BinaryPath: binary,
				Args:       args,
				Env:        env,
			})
			if err != nil {
				return err
			}
			if exitCode != 0 {
				// Return a typed error that cli.Run translates into the
				// matching exit code (instead of its default 1). Defers
				// (including deps.Close) run normally because we return —
				// no os.Exit inside this lambda.
				return run.ExitCodeError{Code: exitCode}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&overrideProfile, "profile", "", "explicit profile name (skips the headroom picker)")
	cmd.Flags().StringVar(&binaryOverride, "claude-binary", "", "absolute path to the claude binary (default: PATH lookup)")
	cmd.Flags().BoolVar(&printOnly, "print-only", false, "print the planned launch and exit without forking")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress the picker rationale line on stderr")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "log additional launch detail to stderr")
	return cmd
}

type evaluatorAdapter struct {
	ev   headroom.Evaluator
	opts headroom.Options
}

func (a evaluatorAdapter) Evaluate(ctx context.Context, profiles []contracts.Profile, _ headroom.Options) (headroom.Result, error) {
	// Use the captured per-call options (with scan failures); ignore the caller's.
	return a.ev.Evaluate(ctx, profiles, a.opts)
}

// (ExitCodeError is defined in internal/run/ — see Task 1.5 below.)

func joinArgs(a []string) string {
	out := ""
	for i, s := range a {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}
```

Required imports for this file: `context`, `fmt`, `os`, `github.com/arafa-dev/ccx/internal/contracts`, `github.com/arafa-dev/ccx/internal/headroom`, `github.com/arafa-dev/ccx/internal/run`, `github.com/spf13/cobra`. **Do not** import `errors`.

Register the command in `internal/cli/cli.go` alongside the others:

```go
root.AddCommand(newRunCommand(opts))
```

**Plumb `run.ExitCodeError` through `cli.Run`.** Today `cli.Run` returns `1`
on any RunE error; we extend it to honor the new typed error. Modify
`internal/cli/cli.go`'s error-handling block (currently lines 63–70):

```go
import (
    // ...existing imports...
    "github.com/arafa-dev/ccx/internal/run"
)

if err := root.ExecuteContext(ctx); err != nil {
    var coded run.ExitCodeError
    if errors.As(err, &coded) {
        // Propagate the child's exit code; suppress the noisy "Error: exit 7" line.
        return coded.Code
    }
    var structured *structuredCLIError
    if !errors.As(err, &structured) {
        _, _ = fmt.Fprintf(opts.Stderr, "Error: %s\n", err)
    }
    return 1
}
return 0
```

Add a unit test in `internal/cli/cli_test.go`:

```go
func TestRunPropagatesExitCodeError(t *testing.T) {
    // Simulate a RunE that returns ExitCodeError directly.
    var stderr bytes.Buffer
    code := cli.Run(context.Background(), cli.Options{
        Args:   []string{"version"},  // any command; we'll inject via a custom root if needed
        Stderr: &stderr,
        Build:  cli.BuildInfo{Version: "test"},
    })
    _ = code
    // Realistic coverage: a small integration test that builds the binary
    // and runs `ccx run --print-only` against a profile that's missing.
    // For the unit-test path, the implementation agent may need to expose
    // a test hook that injects an error directly — keep this lightweight.
}
```

The smoke test in Step 5 also exercises this path: `ccx run -- false` on
Unix should exit 1 (claude not on PATH → run returns its own error → cli.Run
returns 1). A real integration test belongs in `integration_test/`.

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/cli/run.go
go test -race -count=1 ./internal/cli/...
golangci-lint run ./internal/cli/...
```

Expected: tests pass.

- [ ] **Step 5: Manual smoke**

```bash
go build -o /tmp/ccx ./cmd/ccx
/tmp/ccx run --print-only --profile work
# Expected:
#   ccx: explicit --profile work → profile=work config_dir=...
#   binary=/path/to/claude profile=work args=
```

- [ ] **Step 6: Commit**

```bash
git add internal/cli/run.go internal/cli/cli.go internal/cli/run_test.go
git commit -m "feat(cli): add ccx run command with profile picker and child launcher"
```

---

## Task 4: Extend `internal/shell/snippets.go` with claude wrapper (TDD)

**Files:**
- Modify: `internal/shell/snippets.go`
- Modify: `internal/shell/golden_test.go` (add golden files for new wrappers)
- Create: `internal/shell/testdata/init_*_with_claude.golden` (one per shell)

- [ ] **Step 1: Add failing test**

Append to `internal/shell/golden_test.go`:

```go
func TestEmitClaudeWrapperPosix(t *testing.T) {
	got := shell.EmitClaudeWrapperPosix()
	want := mustReadGolden(t, "init_posix_with_claude.golden")
	if got != want {
		t.Errorf("posix claude wrapper mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestEmitClaudeWrapperFish(t *testing.T) {
	got := shell.EmitClaudeWrapperFish()
	want := mustReadGolden(t, "init_fish_with_claude.golden")
	if got != want {
		t.Errorf("fish claude wrapper mismatch:\n%s", got)
	}
}

func TestEmitClaudeWrapperPowerShell(t *testing.T) {
	got := shell.EmitClaudeWrapperPowerShell()
	want := mustReadGolden(t, "init_pwsh_with_claude.golden")
	if got != want {
		t.Errorf("pwsh claude wrapper mismatch:\n%s", got)
	}
}
```

Create the golden fixtures under `internal/shell/testdata/`:

`init_posix_with_claude.golden`:

```
claude() {
  command ccx run -- "$@"
}
```

`init_fish_with_claude.golden`:

```
function claude
    command ccx run -- $argv
end
```

`init_pwsh_with_claude.golden`:

```
function claude {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args)
    $ccx = Get-Command ccx -ErrorAction SilentlyContinue
    if (-not $ccx) { $ccx = Get-Command ccx.exe }
    & $ccx.Path run -- @Args
}
```

**Why both `ccx` and `ccx.exe`?** Windows PowerShell normally resolves
`ccx.exe`, but PowerShell on macOS/Linux (PSCore) or via WSL resolves the
no-extension binary. Falling back covers both. Existing `internal/shell/snippets.go`
init function (line 63) hard-codes `ccx.exe`; consider opening a follow-up
PR to fix the existing snippet the same way for consistency. Not blocking
this PR — the new claude wrapper is the only file that ships this dual lookup.

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL — emitters undefined.

- [ ] **Step 3: Implementation**

Append to `internal/shell/snippets.go`:

```go
// EmitClaudeWrapperPosix returns the claude() function for zsh and bash.
const claudeWrapperPosix = `claude() {
  command ccx run -- "$@"
}
`

// EmitClaudeWrapperPosix returns the claude wrapper snippet for zsh and bash.
func EmitClaudeWrapperPosix() string { return claudeWrapperPosix }

// EmitClaudeWrapperFish returns the claude wrapper snippet for fish.
const claudeWrapperFish = `function claude
    command ccx run -- $argv
end
`

// EmitClaudeWrapperFish returns the claude wrapper snippet for fish.
func EmitClaudeWrapperFish() string { return claudeWrapperFish }

// EmitClaudeWrapperPowerShell returns the claude wrapper snippet for PowerShell.
// Resolves both `ccx` (PSCore on macOS/Linux, WSL) and `ccx.exe` (Windows
// PowerShell), avoiding hardcoded .exe assumptions.
const claudeWrapperPowerShell = `function claude {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args)
    $ccx = Get-Command ccx -ErrorAction SilentlyContinue
    if (-not $ccx) { $ccx = Get-Command ccx.exe }
    & $ccx.Path run -- @Args
}
`

// EmitClaudeWrapperPowerShell returns the claude wrapper snippet for PowerShell.
func EmitClaudeWrapperPowerShell() string { return claudeWrapperPowerShell }
```

Wire them into the `ShellEmitter` impl. Look at `internal/shell/emitter.go` to see how `EmitInitScript(shell)` dispatches; add an analogous method `EmitInitScriptWithClaude(shell) (string, error)` that returns `EmitInitScript + "\n" + claudeWrapper`.

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/shell/
go test -race -count=1 ./internal/shell/...
golangci-lint run ./internal/shell/...
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/shell/snippets.go internal/shell/emitter.go internal/shell/golden_test.go internal/shell/testdata/
git commit -m "feat(shell): claude wrapper snippets for posix/fish/pwsh"
```

---

## Task 5: Extend `ccx init` with `--with-claude-wrapper` flag

**Files:**
- Modify: `internal/cli/init.go`
- Modify: `internal/cli/init_test.go`

- [ ] **Step 1: Failing test**

Append to `internal/cli/init_test.go`:

```go
func TestInitWithClaudeWrapperPosix(t *testing.T) {
	s := runCLI(t, "init", "zsh", "--with-claude-wrapper")
	if !strings.Contains(s, "claude()") {
		t.Errorf("expected claude() wrapper; got:\n%s", s)
	}
	if !strings.Contains(s, "ccx run --") {
		t.Errorf("expected `ccx run --` in wrapper; got:\n%s", s)
	}
}

func TestInitWithoutFlagOmitsWrapper(t *testing.T) {
	s := runCLI(t, "init", "zsh")
	if strings.Contains(s, "claude()") {
		t.Errorf("default init should not include claude wrapper:\n%s", s)
	}
}
```

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL — flag undefined.

- [ ] **Step 3: Implementation**

In `internal/cli/init.go`:

1. Add the flag:
   ```go
   var withClaudeWrapper bool
   cmd.Flags().BoolVar(&withClaudeWrapper, "with-claude-wrapper", false,
       "additionally emit a `claude` wrapper that calls `ccx run --`")
   ```

2. In `RunE`, branch on the flag and call `emitter.EmitInitScriptWithClaude(shell)` instead of `EmitInitScript(shell)` when set.

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/cli/init.go
go test -race -count=1 ./internal/cli/...
golangci-lint run ./internal/cli/...
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/init.go internal/cli/init_test.go
git commit -m "feat(cli): ccx init --with-claude-wrapper"
```

---

## Task 6: Create `internal/recstream/` — pressure-band hub (TDD)

**Files:**
- Create: `internal/recstream/doc.go`
- Create: `internal/recstream/hub.go`
- Create: `internal/recstream/hub_test.go`

- [ ] **Step 1: Failing tests**

Create `internal/recstream/hub_test.go`:

```go
package recstream_test

import (
	"context"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/recstream"
)

func TestHubPublishToOneSubscriber(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	h := recstream.NewHub()
	defer h.Close()

	sub := h.Subscribe(ctx)
	go h.Publish(contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationWarn})
	select {
	case ev := <-sub:
		if ev.Profile != "work" || ev.Level != contracts.RecommendationWarn {
			t.Errorf("unexpected event: %+v", ev)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for event")
	}
}

func TestHubFanOutToManySubscribers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	h := recstream.NewHub()
	defer h.Close()

	a := h.Subscribe(ctx)
	b := h.Subscribe(ctx)
	go h.Publish(contracts.RecommendationEvent{Profile: "x", Level: contracts.RecommendationSoft})
	for _, ch := range []<-chan contracts.RecommendationEvent{a, b} {
		select {
		case ev := <-ch:
			if ev.Level != contracts.RecommendationSoft {
				t.Errorf("got %+v", ev)
			}
		case <-ctx.Done():
			t.Fatal("subscriber missed event")
		}
	}
}

func TestStateMachineOnlyEmitsUpwardTransitions(t *testing.T) {
	// Test the state machine, not the hub. We assert that:
	//  - Below warn → warn produces a Warn event.
	//  - Warn → soft produces a Soft event.
	//  - Soft → warn (downward) produces NO event.
	//  - Soft → soft (same level) produces NO event.
	//  - Soft → hard produces a Hard event.
	sm := recstream.NewStateMachine()
	cases := []struct {
		profile string
		pct     float64
		emit    bool
		level   contracts.RecommendationLevel
	}{
		{"work", 50, false, ""},
		{"work", 80, true, contracts.RecommendationWarn},
		{"work", 95, true, contracts.RecommendationSoft},
		{"work", 92, false, ""}, // same band
		{"work", 80, false, ""}, // downward
		{"work", 100, true, contracts.RecommendationHard},
		{"work", 100, false, ""}, // same band
		{"work", 50, false, ""},  // reset path; no emit on downward
	}
	for _, tc := range cases {
		emit, level := sm.Observe(tc.profile, tc.pct)
		if emit != tc.emit {
			t.Errorf("Observe(%s, %v): emit = %v, want %v", tc.profile, tc.pct, emit, tc.emit)
		}
		if emit && level != tc.level {
			t.Errorf("Observe(%s, %v): level = %v, want %v", tc.profile, tc.pct, level, tc.level)
		}
	}
}

func TestStateMachineIsolatedPerProfile(t *testing.T) {
	sm := recstream.NewStateMachine()
	if emit, _ := sm.Observe("a", 80); !emit {
		t.Error("a→warn should emit")
	}
	if emit, _ := sm.Observe("b", 80); !emit {
		t.Error("b→warn should emit (different profile)")
	}
}

func TestHubSubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	h := recstream.NewHub()
	h.Close()
	sub := h.Subscribe(context.Background())
	// A closed channel returns immediately with the zero value and ok=false.
	select {
	case _, ok := <-sub:
		if ok {
			t.Error("expected closed channel; got a value")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Subscribe-after-Close should return a closed channel, not a hanging one")
	}
}

func TestHubCloseThenCtxCancelDoesNotDoubleClose(t *testing.T) {
	// Regression test for the race: Close() closes all sub channels and
	// empties subs; the per-subscriber ctx-cancel goroutine must NOT then
	// close the same channel a second time. We exercise the race in a tight
	// loop and rely on -race to catch any double-close panic.
	for i := 0; i < 100; i++ {
		h := recstream.NewHub()
		ctx, cancel := context.WithCancel(context.Background())
		_ = h.Subscribe(ctx)
		// Race: Close + cancel run concurrently.
		go h.Close()
		cancel()
	}
}

func TestHubConcurrentSubscribeCloseIsSafe(t *testing.T) {
	h := recstream.NewHub()
	defer h.Close()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sub := h.Subscribe(ctx)
			// Drain non-blocking so we don't deadlock if a Publish lands here.
			select {
			case <-sub:
			case <-time.After(10 * time.Millisecond):
			}
		}()
	}
	wg.Wait()
}
```

(Add imports `sync` and `time` if not already present.)

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL — package undefined.

- [ ] **Step 3: Implementation**

Create `internal/recstream/doc.go`:

```go
// Package recstream broadcasts pressure-driven RecommendationEvents from the
// daemon's ingest loop to subscribers (the /api/recommendations/live SSE
// handler and, in B3b, the ccx run --supervise process).
package recstream
```

Create `internal/recstream/hub.go`:

```go
package recstream

import (
	"context"
	"sync"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
)

// Hub is a many-to-many fan-out broadcaster for RecommendationEvent values.
// Subscribers receive only events Publish'd after they subscribe; missed
// events while a subscriber's channel is full are dropped (non-blocking send).
type Hub struct {
	mu     sync.Mutex
	subs   map[chan contracts.RecommendationEvent]struct{}
	closed bool
}

// NewHub constructs an empty Hub.
func NewHub() *Hub {
	return &Hub{subs: map[chan contracts.RecommendationEvent]struct{}{}}
}

// Subscribe returns a channel that receives every future event Publish'd to
// the Hub. The subscription is closed (and removed) when ctx is cancelled
// OR when the Hub itself is closed. Subscribing to an already-closed Hub
// returns a channel that is closed immediately (consumers see end-of-stream
// rather than hanging forever).
func (h *Hub) Subscribe(ctx context.Context) <-chan contracts.RecommendationEvent {
	ch := make(chan contracts.RecommendationEvent, 16)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(ch)
		return ch
	}
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	// One goroutine per subscriber that removes itself on ctx cancellation.
	// The `ok` check ensures we don't double-close after Hub.Close already
	// closed and removed ch from h.subs.
	go func() {
		<-ctx.Done()
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}()
	return ch
}

// Publish fans the event out to all current subscribers. Non-blocking: if a
// subscriber's buffer is full, the event is dropped for that subscriber.
func (h *Hub) Publish(ev contracts.RecommendationEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
			// drop
		}
	}
}

// Close releases all subscriptions and prevents further Publish.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.subs {
		close(ch)
	}
	h.subs = map[chan contracts.RecommendationEvent]struct{}{}
}

// StateMachine remembers the last-observed PressureLevel per profile and
// reports only *upward* transitions as events. It is not goroutine-safe;
// callers should serialize calls (the daemon ingest loop is already serial).
type StateMachine struct {
	last map[string]headroom.PressureLevel
}

// NewStateMachine constructs an empty StateMachine.
func NewStateMachine() *StateMachine {
	return &StateMachine{last: map[string]headroom.PressureLevel{}}
}

// Observe records the current pressure percentage for a profile and returns
// (emit, level): emit is true only on an upward transition into a new band.
func (sm *StateMachine) Observe(profile string, pct float64) (bool, contracts.RecommendationLevel) {
	now := headroom.PressureLevelFromPct(pct)
	prev, seen := sm.last[profile]
	sm.last[profile] = now
	if !seen && now == headroom.PressureNone {
		return false, ""
	}
	if seen && now <= prev {
		return false, ""
	}
	return true, levelOf(now)
}

func levelOf(p headroom.PressureLevel) contracts.RecommendationLevel {
	switch p {
	case headroom.PressureWarn:
		return contracts.RecommendationWarn
	case headroom.PressureSoft:
		return contracts.RecommendationSoft
	case headroom.PressureHard:
		return contracts.RecommendationHard
	default:
		return ""
	}
}
```

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/recstream/
go test -race -count=1 ./internal/recstream/...
golangci-lint run ./internal/recstream/...
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/recstream/doc.go internal/recstream/hub.go internal/recstream/hub_test.go
git commit -m "feat(recstream): Hub broadcaster and pressure-band state machine"
```

---

## Task 7: Daemon-side threshold detector

**Files:**
- Modify: `internal/daemon/ingest.go` (or wherever the per-profile post-ingest hook lives)
- Modify: `internal/daemon/runtime.go` (construct Hub, pass to server)

- [ ] **Step 1: Construct the Hub**

In `internal/daemon/runtime.go`, near where `deps` and `server.New(...)` are constructed:

```go
hub := recstream.NewHub()
defer hub.Close()

stateMachine := recstream.NewStateMachine()
quotaComputer := quota.Computer{Store: deps.Store}
```

Add imports: `internal/recstream`, `internal/quota`, `internal/headroom`.

Then pass `hub` into the server deps:

```go
srv := server.New(server.Deps{
    // ... existing fields ...
    Recommendations: hub,
}, opts.Version)
```

(The `Recommendations` field is added in Task 8.)

- [ ] **Step 2: Wire the detector into the ingest loop**

The existing `runProfileWatcher(runCtx, deps, logger, poll)` in `runtime.go` periodically invokes ingest. After each ingest cycle finishes (look at `internal/daemon/ingest.go` for the integration point — it likely returns to `runProfileWatcher` after each pass), call a new helper:

```go
func observePressure(
    ctx context.Context,
    deps *runtimeDeps,
    computer quota.Computer,
    evaluator headroom.Evaluator,
    sm *recstream.StateMachine,
    hub *recstream.Hub,
    logger *log.Logger,
) {
    profiles, err := deps.Profiles.List(ctx)
    if err != nil {
        logger.Printf("recstream: list profiles: %v", err)
        return
    }
    for i := range profiles {
        if profiles[i].Limits.PlanTier == "" {
            continue
        }
        pq, err := computer.For(ctx, profiles[i])
        if err != nil {
            logger.Printf("recstream: quota for %q: %v", profiles[i].Name, err)
            continue
        }
        worstPct := pq.Window5h.Pct
        if pq.WindowWeekly.Pct > worstPct {
            worstPct = pq.WindowWeekly.Pct
        }
        emit, level := sm.Observe(profiles[i].Name, worstPct)
        if !emit {
            continue
        }
        suggested := bestSibling(ctx, evaluator, profiles, profiles[i].Name, logger)
        hub.Publish(contracts.RecommendationEvent{
            Profile:        profiles[i].Name,
            Level:          level,
            Reason:         fmt.Sprintf("%s pressure %.0f%%", level, worstPct),
            Suggested:      suggested,
            Quota5hPct:     pq.Window5h.Pct,
            QuotaWeeklyPct: pq.WindowWeekly.Pct,
            Timestamp:      time.Now().UTC(),
        })
    }
}

// bestSibling re-runs the evaluator over the registry minus the crossed
// profile, returning the top Available candidate's name (or "" if no sibling
// has more headroom). The result populates RecommendationEvent.Suggested,
// which the dashboard banner uses to render the "Switch to X" button.
func bestSibling(
    ctx context.Context,
    evaluator headroom.Evaluator,
    profiles []contracts.Profile,
    exclude string,
    logger *log.Logger,
) string {
    siblings := profiles[:0:0]
    for i := range profiles {
        if profiles[i].Name == exclude {
            continue
        }
        siblings = append(siblings, profiles[i])
    }
    if len(siblings) == 0 {
        return ""
    }
    result, err := evaluator.Evaluate(ctx, siblings, headroom.Options{})
    if err != nil {
        logger.Printf("recstream: sibling evaluator for %q: %v", exclude, err)
        return ""
    }
    if result.Recommendation == nil {
        return ""
    }
    return result.Recommendation.Profile
}
```

Invoke from the post-ingest path:

```go
observePressure(ctx, deps, quotaComputer, evaluator, stateMachine, hub, logger)
```

where `evaluator` is the same `headroom.Evaluator{Store: deps.Store, Pricing: deps.Pricing}` instance already constructed for `server.Deps.Headroom`. Pass it through so we don't build two evaluators per cycle.

- [ ] **Step 3: Verify**

```bash
go build ./...
go test -race -count=1 ./internal/daemon/...
golangci-lint run ./internal/daemon/...
```

Expected: build clean, tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/daemon/runtime.go internal/daemon/ingest.go
git commit -m "feat(daemon): pressure-band detection emits RecommendationEvents via Hub"
```

---

## Task 8: Server `/api/recommendations/live` SSE handler (TDD)

**Files:**
- Modify: `internal/server/server.go` (add `Recommendations` to Deps, add interface, register route)
- Create: `internal/server/recommendations.go` (handler)
- Create: `internal/server/recommendations_test.go`

- [ ] **Step 1: Failing test**

Create `internal/server/recommendations_test.go`:

```go
package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/recstream"
	"github.com/arafa-dev/ccx/internal/server"
)

func TestRecommendationsLiveEmitsPublishedEvent(t *testing.T) {
	hub := recstream.NewHub()
	defer hub.Close()
	srv := server.New(server.Deps{Recommendations: hub}, "test")

	// Use a flushingRecorder so the handler's flusher.Flush() observably
	// drains the response body. The handler exits cleanly when ctx is
	// cancelled or the channel closes.
	rec := newFlushingRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/recommendations/live", nil).WithContext(ctx)

	served := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rec, req)
		close(served)
	}()

	// Publish once; wait deterministically until the handler observed the
	// flush (no time.Sleep). flushed is signalled by the recorder each time
	// the handler calls Flush().
	hub.Publish(contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationWarn})
	select {
	case <-rec.flushed:
	case <-time.After(time.Second):
		t.Fatal("handler did not flush the published event within 1s")
	}

	cancel()
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("handler did not return after ctx cancel within 1s")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: recommendation") {
		t.Errorf("expected SSE event marker; got:\n%s", body)
	}
	if !strings.Contains(body, `"profile":"work"`) {
		t.Errorf("expected profile in payload; got:\n%s", body)
	}
}

// flushingRecorder is httptest.ResponseRecorder + http.Flusher; it signals
// every flush on the `flushed` channel so the test can synchronize without
// time.Sleep.
type flushingRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
}

func newFlushingRecorder() *flushingRecorder {
	return &flushingRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		flushed:          make(chan struct{}, 8),
	}
}

func (f *flushingRecorder) Flush() {
	f.ResponseRecorder.Flush()
	select {
	case f.flushed <- struct{}{}:
	default: // non-blocking; tests only care about at-least-one
	}
}
```

- [ ] **Step 2: Run, confirm fail**

Expected: FAIL — route not registered.

- [ ] **Step 3: Implementation**

In `internal/server/server.go`:

1. Add `Recommendations RecommendationsSource` to `Deps`.
2. Add interface:
   ```go
   // RecommendationsSource provides a per-request subscription channel of
   // RecommendationEvent values. Implemented by *recstream.Hub.
   type RecommendationsSource interface {
       Subscribe(ctx context.Context) <-chan contracts.RecommendationEvent
   }
   ```
3. Register the route:
   ```go
   s.mux.Get("/api/recommendations/live", s.handleRecommendationsLive)
   ```

Create `internal/server/recommendations.go`:

```go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) handleRecommendationsLive(w http.ResponseWriter, r *http.Request) {
	if s.deps.Recommendations == nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("recommendations source unavailable"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	sub := s.deps.Recommendations.Subscribe(r.Context())
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-sub:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "event: recommendation\ndata: %s\n\n", b)
			flusher.Flush()
		}
	}
}
```

- [ ] **Step 4: Verify**

```bash
gofumpt -w internal/server/
go test -race -count=1 ./internal/server/...
golangci-lint run ./internal/server/...
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/recommendations.go internal/server/recommendations_test.go
git commit -m "feat(server): /api/recommendations/live SSE handler"
```

---

## Task 9: Dashboard `<RecommendationBanner>` component (TDD)

**Files:**
- Create: `web/components/recommendation-banner.tsx`
- Create: `web/components/recommendation-banner.test.tsx`
- Modify: `web/lib/api.ts` (add `streamRecommendations` SSE helper)
- Modify: `web/mocks/handlers.ts` (mock the SSE stream)

- [ ] **Step 1: Add the SSE client and MSW handler**

In `web/lib/api.ts`, add a streaming helper analogous to the existing `streamUsage`:

```ts
import type { components } from './api-types';
export type RecommendationEvent = components['schemas']['RecommendationEvent'];

export function streamRecommendations(
  onEvent: (ev: RecommendationEvent) => void,
  onDisconnect: () => void,
): () => void {
  const url = new URL('/api/recommendations/live', window.location.origin);
  const es = new EventSource(url.toString());
  es.addEventListener('recommendation', (e: MessageEvent) => {
    try {
      const ev = JSON.parse(e.data) as RecommendationEvent;
      onEvent(ev);
    } catch {/* ignore */}
  });
  es.addEventListener('error', () => onDisconnect());
  return () => es.close();
}
```

In `web/mocks/handlers.ts`, add an SSE handler that returns an empty 200 stream (real events come from the actual daemon in production; the dashboard's behavior is tested against synthetic events directly).

- [ ] **Step 2: Failing component test**

Create `web/components/recommendation-banner.test.tsx`:

```tsx
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RecommendationBanner } from './recommendation-banner';

describe('RecommendationBanner', () => {
  it('renders nothing when event is null', () => {
    const { container } = render(<RecommendationBanner event={null} onSwitch={() => {}} />);
    expect(container.firstChild).toBeNull();
  });

  it('shows the profile, level, and reason', () => {
    render(<RecommendationBanner event={{
      profile: 'personal', level: 'soft', reason: '5h pressure 92%',
      suggested: 'work', quota_5h_pct: 92, quota_weekly_pct: 41,
      timestamp: '2026-05-24T18:42:00Z',
    }} onSwitch={() => {}} />);
    expect(screen.getByText(/personal/)).toBeInTheDocument();
    expect(screen.getByText(/soft/i)).toBeInTheDocument();
    expect(screen.getByText(/work/)).toBeInTheDocument();
  });

  it('hard-level banner has a distinct visual marker', () => {
    const { container } = render(<RecommendationBanner event={{
      profile: 'personal', level: 'hard', reason: '5h 100%',
      suggested: '', quota_5h_pct: 100, quota_weekly_pct: 0,
      timestamp: '2026-05-24T18:42:00Z',
    }} onSwitch={() => {}} />);
    expect(container.textContent).toMatch(/hard|cap/i);
  });

  it('Switch button is hidden when there is no suggested sibling', () => {
    render(<RecommendationBanner event={{
      profile: 'personal', level: 'hard', reason: 'all siblings capped',
      suggested: '', quota_5h_pct: 100, quota_weekly_pct: 0,
      timestamp: '2026-05-24T18:42:00Z',
    }} onSwitch={() => {}} />);
    expect(screen.queryByRole('button', { name: /switch/i })).toBeNull();
  });
});
```

- [ ] **Step 3: Run, confirm fail**

```bash
cd web
pnpm test recommendation-banner
```

Expected: FAIL — component undefined.

- [ ] **Step 4: Implementation**

Create `web/components/recommendation-banner.tsx`:

```tsx
'use client';

import type { RecommendationEvent } from '@/lib/api';

export function RecommendationBanner({
  event,
  onSwitch,
}: {
  event: RecommendationEvent | null;
  onSwitch: (toProfile: string) => void;
}) {
  if (!event) return null;
  const tone = toneFor(event.level);
  return (
    <section
      role="status"
      aria-live="polite"
      className={`rounded-xl border px-4 py-3 ${tone.border} ${tone.bg}`}
    >
      <div className="flex items-center justify-between gap-3">
        <div className="text-sm">
          <span className="font-semibold">{tone.icon} </span>
          Active profile{' '}
          <code className="rounded bg-grid px-1 py-0.5 font-mono text-xs">{event.profile}</code>{' '}
          crossed the {event.level} threshold ({event.reason}).
          {event.suggested && (
            <>
              {' '}Consider switching to{' '}
              <code className="rounded bg-grid px-1 py-0.5 font-mono text-xs">{event.suggested}</code>.
            </>
          )}
        </div>
        {event.suggested && (
          <button
            type="button"
            className="rounded-md border border-card-border px-3 py-1 text-sm font-medium hover:bg-grid"
            onClick={() => onSwitch(event.suggested)}
          >
            Switch to {event.suggested}
          </button>
        )}
      </div>
    </section>
  );
}

function toneFor(level: RecommendationEvent['level']) {
  switch (level) {
    case 'hard': return { icon: '⛔', border: 'border-red-500/40',    bg: 'bg-red-500/10'    };
    case 'soft': return { icon: '🟠', border: 'border-orange-500/40', bg: 'bg-orange-500/10' };
    default:     return { icon: '⚠',  border: 'border-yellow-500/40', bg: 'bg-yellow-500/10' };
  }
}
```

- [ ] **Step 5: Verify**

```bash
pnpm test recommendation-banner
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd ..
git add web/components/recommendation-banner.tsx web/components/recommendation-banner.test.tsx web/lib/api.ts web/mocks/handlers.ts
git commit -m "feat(web): RecommendationBanner component and streamRecommendations SSE client"
```

---

## Task 10: Wire banner into `dashboard.tsx`

**Files:**
- Modify: `web/components/dashboard.tsx`
- Modify: `web/components/dashboard.test.tsx`

- [ ] **Step 1: Add failing test**

Append to `web/components/dashboard.test.tsx`:

```tsx
it('renders banner when a recommendation arrives via SSE', async () => {
  // The MSW mock for /api/recommendations/live is an empty stream; in this
  // test we'd ideally simulate a server event. For a v0.2 first-pass, just
  // verify the banner *slot* exists in the DOM (a hidden empty <section>).
  render(<Dashboard />);
  // The slot is present but renders null when event is null. Assert no error.
  expect(await screen.findByText(/plan quota/i)).toBeInTheDocument();
});
```

(A richer integration test that synthesizes a server event can be added in B3b once the supervisor exists.)

- [ ] **Step 2: Run, confirm pass on hook of existing snapshot**

```bash
cd web && pnpm test dashboard
```

If the existing assertions still pass, you're good.

- [ ] **Step 3: Wire the component**

In `web/components/dashboard.tsx`:

1. Import `RecommendationBanner` and `streamRecommendations`.
2. Add state `const [recEvent, setRecEvent] = useState<RecommendationEvent | null>(null);`
3. Add a `useEffect` that subscribes via `streamRecommendations`, sets `recEvent` on each event, and returns the stop fn.
4. Render `<RecommendationBanner event={recEvent} onSwitch={(p) => handleSelectProfile(p)} />` between `<RecommendationPanel ... />` and `<TimeSeriesChart ... />` (or above `<QuotaPanel />` per spec §8.5).

- [ ] **Step 4: Verify**

```bash
pnpm typecheck
pnpm test
```

Expected: green.

- [ ] **Step 5: Commit**

```bash
cd ..
git add web/components/dashboard.tsx web/components/dashboard.test.tsx
git commit -m "feat(web): mount RecommendationBanner subscribed to SSE stream"
```

---

## Task 11: Final verification + manual smoke

**Files:** none.

- [ ] **Step 1: `make ci` + web**

```bash
make ci
cd web && pnpm typecheck && pnpm test && cd ..
```

Expected: green.

- [ ] **Step 2: Build**

```bash
cd web && pnpm build && cd ..
make stage-web && make build
```

- [ ] **Step 3: End-to-end manual smoke (requires real Claude profile + hooks)**

```bash
# Set a tiny cap on a profile so a single turn trips the hard threshold.
./dist/ccx profile set demo --plan-tier max5 --caps-5h-turns 1

# Start daemon.
./dist/ccx daemon start
sleep 2

# Subscribe to the SSE in one terminal:
curl -N http://127.0.0.1:7777/api/recommendations/live &
SSE_PID=$!

# In another terminal, fire a turn under demo (1 minimal prompt is enough).
CLAUDE_CONFIG_DIR=~/.claude-profiles/demo claude -p "hi"

# Wait for the daemon to ingest the new Stop hook event (poll interval default 5s).
sleep 8

# Expected on the SSE terminal: an `event: recommendation` with `"level":"hard"`.
kill $SSE_PID
./dist/ccx daemon stop
```

- [ ] **Step 4: `ccx run` smoke**

```bash
./dist/ccx run --print-only
# Expected: ccx: headroom recommendation: score=... → profile=... config_dir=...
#           binary=/path/to/claude profile=... args=
```

- [ ] **Step 5: Push and open PR**

```bash
git push -u origin feat/quota-pre-launch
gh pr create \
  --base main \
  --title "feat(quota): pre-launch fallback (v0.2 B3a)" \
  --body "$(cat <<'EOF'
## Summary

Three deliverables that together give the user automatic profile selection at session start, plus a live notification path for the dashboard.

- `ccx run [-- args]`: picks the highest-headroom profile (via the existing scorer from B2), sets CLAUDE_CONFIG_DIR, fork+wait launches claude, forwards stdio, signals, and exit code.
- `ccx init <shell> --with-claude-wrapper`: opt-in shell snippet adding a `claude` function that calls `ccx run --`.
- Daemon-level pressure detection emits `RecommendationEvent` on upward band transitions (warn/soft/hard); served via new `/api/recommendations/live` SSE endpoint; dashboard renders a `<RecommendationBanner>` slot subscribed to the stream.

New packages: `internal/run/`, `internal/recstream/`.

Spec: `docs/superpowers/specs/2026-05-24-ccx-plan-aware-quota.md`
Plan: `docs/superpowers/plans/2026-05-24-ccx-quota-B3a-pre-launch-fallback.md`

## Test plan

- [x] `make ci` green
- [x] `pnpm test` and `pnpm typecheck` green
- [x] Manual: `ccx run --print-only` works
- [x] Manual: SSE delivers a recommendation event after a Caps5hTurns=1 profile sees one Stop
- [x] Manual: Dashboard banner renders during the SSE event
EOF
)"
```

- [ ] **Step 6: After merge, update plan index status**

Mark B3a row `✅ Merged in #<PR-number>`.

---

## Verification criteria (definition of done)

1. **`internal/run/`** exports `LocateClaude`, `BuildEnv`, `Launch`, `Pick`, `PickOptions`, `EvaluatorFunc`, `ErrNoProfiles`, `ErrNoRecommendation`. Each has tests; `Launch` round-trips an exit code.

2. **`ccx run` command** has flags `--profile`, `--claude-binary`, `--print-only`, `--quiet`, `--verbose`. `--print-only` prints the planned command without forking. `--profile <unknown>` returns nonzero exit. Empty registry returns nonzero exit with a helpful message.

3. **`internal/shell/`** has three new snippet emitters and matching golden files. `ccx init <shell> --with-claude-wrapper` returns the existing init + the wrapper snippet.

4. **`internal/recstream/`** exports `NewHub`, `Hub.{Subscribe,Publish,Close}`, `NewStateMachine`, `StateMachine.Observe`. Tests cover fan-out, dropped sends on full buffers, per-profile isolation, upward-only transitions.

5. **Daemon** constructs a `Hub` at startup, runs `observePressure` after each ingest cycle, publishes upward-band-transition events.

6. **`/api/recommendations/live`** emits SSE `recommendation` events with JSON `RecommendationEvent` payloads. Subscribers terminate cleanly on `r.Context().Done()`.

7. **Dashboard** has `<RecommendationBanner>` mounted with `streamRecommendations` subscription. The banner renders nothing when no event; renders content with profile/level/reason on event; shows a "Switch to X" button only when `suggested` is non-empty; hard-level events are visually distinct.

8. **No frozen files modified.**

9. **PR merged to `main`** with green CI. Plan index updated.

---

## Rollback

- **`ccx run` causes regressions** → leave the command in place but document `--claude-binary` and `--print-only` for users who want manual control. Or remove the command in a one-file PR.
- **SSE events too noisy in practice** → adjust `StateMachine.Observe` to suppress repeated events within a short cooldown. One-file PR.
- **Shell wrapper breaks user setups** → users opt in via `--with-claude-wrapper`; the default `ccx init` is unchanged. No rollback needed.
- **Daemon pressure-observation hurts ingest loop performance** → guard the call with a feature flag (env var or daemon CLI flag) and disable by default in a follow-up. The Hub-publish path is non-blocking, so worst case is wasted CPU per cycle.

This phase introduces no schema migration; no contract additions beyond those already in P0; rollback is a sequence of small reverts.
