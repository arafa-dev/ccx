# ccx Phase 2 — Integration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire all Phase 1 packages into a working `ccx` binary. Implement `internal/cli` (cobra commands), `internal/server` (HTTP API), `internal/tui` (bubbletea picker), `internal/doctor`, `internal/dashboard` (go:embed glue), and `cmd/ccx/main.go`. Add end-to-end integration tests.

**Architecture:** Phase 2 is the only phase that may import from multiple sibling `internal/*` packages. The cli layer composes the units. The server layer implements `api/openapi.yaml` against `contracts.Store`. The dashboard package holds the embed directive for `web/out`. End-to-end tests run the actual binary as a subprocess.

**Tech Stack:** Go 1.22+, cobra, chi, bubbletea, lipgloss, fsnotify, all already pulled in by Phase 1.

**Spec reference:** [`../specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — sections 4, 6, 8, 11.3.

**Worktree:** This phase runs on `main` after all of A1–A9 are merged. Strongly prefer a single agent for this phase — parallelism here causes more conflicts than it saves.

**Pre-flight:**

```bash
git checkout main && git pull --ff-only
# verify Phase 1 is fully merged
ls internal/profile/profile.go internal/scanner/scanner.go internal/storage/storage.go \
   internal/pricing/pricing.go internal/shell/shell.go internal/platform/platform.go \
   web/out/index.html 2>&1 | grep -v "No such" | wc -l
# expected: 7 (all present)
git checkout -b feat/integration
```

**Exit criteria:**
- `make build` produces `dist/ccx` that runs and shows `--help`
- Every CLI command from spec §4 works end-to-end
- `ccx dashboard` opens browser, dashboard fetches live data
- Integration tests pass on macOS, Linux, Windows
- CI is green on `feat/integration` PR

---

## Task 1: `cmd/ccx/main.go`

**Files:** Create: `cmd/ccx/main.go`

- [ ] **Step 1: Write `cmd/ccx/main.go`**

```go
// Command ccx is the user-facing CLI binary for the ccx workspace manager.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/arafa-dev/ccx/internal/cli"
)

// Set by the build via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.Execute(ctx, cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}))
}
```

- [ ] **Step 2: Verify build (will fail until Task 2 lands `cli.Execute`)**

```bash
go build ./cmd/ccx
```

Expected at this step: failure with `undefined: cli.Execute`. That's fine; Task 2 fixes it.

- [ ] **Step 3: Commit**

```bash
git add cmd/ccx/main.go
git commit -m "feat(cmd): add ccx binary entrypoint"
```

---

## Task 2: `internal/cli` — root command + `BuildInfo` + `Execute`

**Files:**
- Create: `internal/cli/cli.go`
- Create: `internal/cli/cli_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/cli_test.go`:

```go
package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/cli"
)

func TestExecuteHelpShowsAllCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"--help"},
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code != 0 {
		t.Fatalf("--help exit=%d stderr=%q", code, stderr.String())
	}
	want := []string{"profile", "use", "init", "usage", "dashboard", "doctor", "version"}
	got := stdout.String()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("--help missing command %q", w)
		}
	}
}

func TestExecuteVersion(t *testing.T) {
	var stdout bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"version"},
		Stdout: &stdout,
		Build:  cli.BuildInfo{Version: "0.0.0-test"},
	})
	if code != 0 || !strings.Contains(stdout.String(), "0.0.0-test") {
		t.Errorf("version: code=%d out=%q", code, stdout.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cli/...
```

Expected: FAIL — `cli.Run` and `cli.Options` undefined.

- [ ] **Step 3: Write `internal/cli/cli.go`**

```go
// Package cli wires Phase 1 packages into the cobra command tree exposed by cmd/ccx.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// BuildInfo carries version metadata baked in at build time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Options configures a single Run invocation (used by tests and main).
type Options struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Build  BuildInfo
}

// Execute is the production entry point — uses os.Args, os.Stdin/out/err.
func Execute(ctx context.Context, build BuildInfo) int {
	return Run(ctx, Options{
		Args:   os.Args[1:],
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Build:  build,
	})
}

// Run builds the root cobra command from Options and executes it. Returns the
// process exit code.
func Run(ctx context.Context, opts Options) int {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	root := newRootCommand(opts)
	root.SetArgs(opts.Args)
	root.SetOut(opts.Stdout)
	root.SetErr(opts.Stderr)
	if opts.Stdin != nil {
		root.SetIn(opts.Stdin)
	}
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(opts.Stderr, "Error: %s\n", err)
		return 1
	}
	return 0
}

func newRootCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ccx",
		Short:         "Multi-account workspace manager for Claude Code",
		Long:          "ccx switches between Claude Code accounts and tracks usage across them.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		newVersionCommand(opts),
		newProfileCommand(opts),
		newUseCommand(opts),
		newInitCommand(opts),
		newUsageCommand(opts),
		newDashboardCommand(opts),
		newDoctorCommand(opts),
	)
	return cmd
}

func newVersionCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(c *cobra.Command, _ []string) error {
			fmt.Fprintf(c.OutOrStdout(), "ccx %s (commit %s, built %s)\n",
				opts.Build.Version, opts.Build.Commit, opts.Build.Date)
			return nil
		},
	}
}
```

The constructors `newProfileCommand`, `newUseCommand`, etc. are defined in later tasks but referenced now. To make this task's tests pass standalone, add stub constructors in this file that each return `&cobra.Command{Use: "<name>", Short: "stub"}`. Later tasks replace the stubs with real implementations.

Append to `internal/cli/cli.go`:

```go
// The following are stub commands; later tasks replace these with real implementations.

func newProfileCommand(_ Options) *cobra.Command {
	return &cobra.Command{Use: "profile", Short: "Manage profiles", RunE: notImpl("profile")}
}

func newUseCommand(_ Options) *cobra.Command {
	return &cobra.Command{Use: "use", Short: "Activate a profile", RunE: notImpl("use")}
}

func newInitCommand(_ Options) *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Print shell rc snippet", RunE: notImpl("init")}
}

func newUsageCommand(_ Options) *cobra.Command {
	return &cobra.Command{Use: "usage", Short: "Show token usage", RunE: notImpl("usage")}
}

func newDashboardCommand(_ Options) *cobra.Command {
	return &cobra.Command{Use: "dashboard", Short: "Open local dashboard", RunE: notImpl("dashboard")}
}

func newDoctorCommand(_ Options) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Diagnose your install", RunE: notImpl("doctor")}
}

func notImpl(name string) func(*cobra.Command, []string) error {
	return func(*cobra.Command, []string) error {
		return fmt.Errorf("%s: not implemented yet", name)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/cli/...
go build ./cmd/ccx
```

Expected: tests PASS, build succeeds.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat(cli): add root command, BuildInfo, Run/Execute"
```

---

## Task 3: `internal/cli/wiring.go` — shared dependency factory

**Files:** Create: `internal/cli/wiring.go`

This factory builds the live `Store`, `ProfileManager`, `Scanner`, `PricingTable`, `ShellEmitter` for use by every subcommand. It centralizes path resolution and error wrapping.

- [ ] **Step 1: Write `internal/cli/wiring.go`**

```go
package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/pricing"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/scanner"
	"github.com/arafa-dev/ccx/internal/shell"
	"github.com/arafa-dev/ccx/internal/storage"
)

// Deps holds the live implementations every subcommand may need.
type Deps struct {
	Store    contracts.Store
	Profiles *profile.Manager
	Scanner  contracts.Scanner
	Pricing  contracts.PricingTable
	Shell    contracts.ShellEmitter
}

// Close releases all resources owned by Deps. Safe to call on a zero value.
func (d *Deps) Close() error {
	if d == nil || d.Store == nil {
		return nil
	}
	return d.Store.Close()
}

// buildDeps initializes every dependency from disk. Each subcommand calls
// this lazily so that pure-help and version invocations don't touch state.
func buildDeps(ctx context.Context) (*Deps, error) {
	home, err := platform.CCXHome()
	if err != nil {
		return nil, fmt.Errorf("ccx home: %w", err)
	}

	store, err := storage.NewStore(ctx, filepath.Join(home, "state.db"))
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	profMgr, err := profile.NewManager(filepath.Join(home, "profiles.toml"))
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("profile manager: %w", err)
	}

	priceTab, err := pricing.NewTable()
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("pricing: %w", err)
	}

	return &Deps{
		Store:    store,
		Profiles: profMgr,
		Scanner:  scanner.New(scanner.StoreCursorAdapter(store)),
		Pricing:  priceTab,
		Shell:    shell.New(),
	}, nil
}
```

Note: This file references `profile.NewManager`, `scanner.New`, `scanner.StoreCursorAdapter`, `shell.New`, `pricing.NewTable`, `platform.CCXHome`, `storage.NewStore`. Verify each exists with the expected signature; if any A1–A6 plan defined a different constructor name, open a contract-amendment issue and pause. Do not edit Phase 1 packages from this worktree.

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: success. If a constructor name differs, fix only this file to match the Phase 1 actuals.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/wiring.go
git commit -m "feat(cli): add Deps factory wiring Phase 1 packages"
```

---

## Task 4: `internal/cli/profile.go` — `ccx profile add | list | rm | current`

**Files:**
- Create: `internal/cli/profile.go`
- Create: `internal/cli/profile_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/cli"
)

func TestProfileAddListRm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// add
	out := runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)
	if !strings.Contains(out, "Profile 'work' added") {
		t.Errorf("add output: %q", out)
	}

	// list
	out = runCLI(t, "profile", "list")
	if !strings.Contains(out, "work") {
		t.Errorf("list missing 'work': %q", out)
	}

	// rm
	out = runCLI(t, "profile", "rm", "work")
	if !strings.Contains(out, "removed") {
		t.Errorf("rm output: %q", out)
	}
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code != 0 {
		t.Fatalf("exit %d: stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	return stdout.String()
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cli/... -run TestProfileAddListRm
```

Expected: FAIL because the profile command is a stub.

- [ ] **Step 3: Write `internal/cli/profile.go`**

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/spf13/cobra"
)

func newProfileCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage profiles",
	}
	cmd.AddCommand(
		newProfileAddCmd(opts),
		newProfileListCmd(opts),
		newProfileRmCmd(opts),
		newProfileCurrentCmd(opts),
	)
	return cmd
}

func newProfileAddCmd(_ Options) *cobra.Command {
	var configDir, label, color string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a new profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer deps.Close()

			name := args[0]
			p := contracts.Profile{
				Name:       name,
				ConfigDir:  configDir,
				Label:      label,
				Color:      color,
				CreatedAt:  time.Now().UTC(),
				LastUsedAt: time.Time{},
			}
			if err := deps.Profiles.Add(ctx, p); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(),
				"Profile %q added.\nNext: eval \"$(ccx use %s)\" && claude /login\n",
				name, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&configDir, "config-dir", "", "absolute path to the Claude Code config directory (required)")
	cmd.Flags().StringVar(&label, "label", "", "human-readable label")
	cmd.Flags().StringVar(&color, "color", "", "hex accent color for the dashboard, e.g. #3B82F6")
	_ = cmd.MarkFlagRequired("config-dir")
	return cmd
}

func newProfileListCmd(_ Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered profiles with today's usage",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer deps.Close()

			profiles, err := deps.Profiles.List(ctx)
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "No profiles registered. Run `ccx profile add <name> --config-dir <path>`.")
				return nil
			}
			active, _, _ := deps.Profiles.Active(ctx)
			w := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tCONFIG_DIR\tLAST USED\tTODAY ($)")
			for _, p := range profiles {
				marker := " "
				if p.Name == active.Name {
					marker = "*"
				}
				today := todayCostFor(ctx, deps, p.Name)
				lastUsed := "—"
				if !p.LastUsedAt.IsZero() {
					lastUsed = p.LastUsedAt.Format(time.RFC3339)
				}
				fmt.Fprintf(w, "%s%s\t%s\t%s\t$%.2f\n", marker, p.Name, p.ConfigDir, lastUsed, today)
			}
			return w.Flush()
		},
	}
}

func newProfileRmCmd(_ Options) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Unregister a profile (does not delete its config dir)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer deps.Close()
			if err := deps.Profiles.Remove(ctx, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Profile %q removed.\n", args[0])
			return nil
		},
	}
}

func newProfileCurrentCmd(_ Options) *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show the active profile",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer deps.Close()
			p, ok, err := deps.Profiles.Active(ctx)
			if err != nil && !errors.Is(err, contracts.ErrNoActiveProfile) {
				return err
			}
			if !ok {
				if cfg := os.Getenv("CLAUDE_CONFIG_DIR"); cfg != "" {
					fmt.Fprintf(c.OutOrStdout(), "unmanaged config: %s\n", cfg)
					return nil
				}
				fmt.Fprintln(c.OutOrStdout(), "default profile (no CCX_ACTIVE_PROFILE set)")
				return nil
			}
			fmt.Fprintf(c.OutOrStdout(), "%s\nconfig: %s\n", p.Name, p.ConfigDir)
			return nil
		},
	}
}

func todayCostFor(ctx context.Context, deps *Deps, name string) float64 {
	start := time.Now().UTC().Truncate(24 * time.Hour)
	rows, err := deps.Store.QueryUsage(ctx, contracts.UsageQuery{
		Profile: name,
		Range:   contracts.TimeRange{Start: start, End: start.Add(24 * time.Hour)},
	})
	if err != nil {
		return 0
	}
	var sum float64
	for _, r := range rows {
		c, _ := deps.Pricing.Cost(r.Model, r.Day, r.Usage)
		sum += c
	}
	return sum
}
```

- [ ] **Step 4: Remove the `newProfileCommand` stub from `cli.go`**

Open `internal/cli/cli.go`, delete the stub `newProfileCommand` function added in Task 2. The real one now lives in `profile.go` (same package, same name — replaces the stub).

- [ ] **Step 5: Run test to verify pass**

```bash
go test ./internal/cli/... -run TestProfileAddListRm
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/profile.go internal/cli/profile_test.go internal/cli/cli.go
git commit -m "feat(cli): implement profile add | list | rm | current"
```

---

## Task 5: `internal/cli/use.go` — `ccx use [<name>]`

**Files:**
- Create: `internal/cli/use.go`
- Create: `internal/cli/use_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/cli"
)

func TestUseEmitsExportForPOSIX(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SHELL", "/bin/zsh")
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"use", "work"},
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "export CLAUDE_CONFIG_DIR=") {
		t.Errorf("missing export CLAUDE_CONFIG_DIR: %q", got)
	}
	if !strings.Contains(got, "export CCX_ACTIVE_PROFILE=") {
		t.Errorf("missing export CCX_ACTIVE_PROFILE: %q", got)
	}
	if !strings.Contains(got, cfgDir) {
		t.Errorf("missing config dir path: %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cli/... -run TestUseEmitsExportForPOSIX
```

Expected: FAIL.

- [ ] **Step 3: Write `internal/cli/use.go`**

```go
package cli

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/tui"
	"github.com/spf13/cobra"
)

func newUseCommand(_ Options) *cobra.Command {
	var shellOverride string
	cmd := &cobra.Command{
		Use:   "use [name]",
		Short: "Activate a profile in the current shell",
		Long: `Prints shell commands that, when eval'd, switch the active profile.

  eval "$(ccx use work)"

If <name> is omitted, opens an interactive picker.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer deps.Close()

			var p contracts.Profile
			if len(args) == 1 {
				p, err = deps.Profiles.Get(ctx, args[0])
				if err != nil {
					return err
				}
			} else {
				profiles, err := deps.Profiles.List(ctx)
				if err != nil {
					return err
				}
				if len(profiles) == 0 {
					return fmt.Errorf("no profiles registered; run `ccx profile add` first")
				}
				picked, err := tui.PickProfile(profiles)
				if err != nil {
					return err
				}
				p = picked
			}

			sh := platform.DetectShell()
			if shellOverride != "" {
				parsed, ok := contracts.ParseShell(shellOverride)
				if !ok {
					return contracts.ErrUnknownShell
				}
				sh = parsed
			}
			script, err := deps.Shell.EmitUseScript(p, sh)
			if err != nil {
				return err
			}
			if err := deps.Profiles.MarkUsed(ctx, p.Name); err != nil {
				return err
			}
			fmt.Fprint(c.OutOrStdout(), script)
			return nil
		},
	}
	cmd.Flags().StringVar(&shellOverride, "shell", "", "force shell flavor (zsh|bash|fish|pwsh); default: auto-detect")
	return cmd
}
```

- [ ] **Step 4: Remove the stub `newUseCommand` from `cli.go`**

- [ ] **Step 5: Run test to verify pass**

```bash
go test ./internal/cli/... -run TestUseEmitsExportForPOSIX
```

Expected: PASS (assumes `tui.PickProfile` is at least a placeholder; if Task 9 hasn't been completed yet, add a non-interactive fallback that returns the first profile when not connected to a TTY).

Add at the top of `tui` package once Task 9 is in progress; for this task, if `tui.PickProfile` doesn't yet exist, temporarily inline a fallback:

```go
// Temporary fallback if internal/tui is not built yet.
// REMOVE once Task 9 is complete.
func pickFirst(profiles []contracts.Profile) contracts.Profile { return profiles[0] }
```

…and replace the `tui.PickProfile(profiles)` call with `pickFirst(profiles), nil`. Restore the proper call after Task 9.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/use.go internal/cli/use_test.go internal/cli/cli.go
git commit -m "feat(cli): implement ccx use with shell emission"
```

---

## Task 6: `internal/cli/init.go` — `ccx init <shell>`

**Files:**
- Create: `internal/cli/init.go`
- Create: `internal/cli/init_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/cli"
)

func TestInitZshContainsWrapperFunction(t *testing.T) {
	var stdout bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"init", "zsh"},
		Stdout: &stdout,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout.String(), "ccx()") && !strings.Contains(stdout.String(), "function ccx") {
		t.Errorf("missing wrapper function: %q", stdout.String())
	}
}

func TestInitUnknownShellErrors(t *testing.T) {
	var stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Options{
		Args:   []string{"init", "tcsh"},
		Stderr: &stderr,
		Build:  cli.BuildInfo{Version: "test"},
	})
	if code == 0 {
		t.Errorf("expected non-zero exit for unknown shell")
	}
	if !strings.Contains(stderr.String(), "unknown shell") {
		t.Errorf("expected 'unknown shell' in stderr: %q", stderr.String())
	}
}
```

- [ ] **Step 2: Run tests, verify fail**

```bash
go test ./internal/cli/... -run TestInit
```

Expected: FAIL.

- [ ] **Step 3: Write `internal/cli/init.go`**

```go
package cli

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/spf13/cobra"
)

func newInitCommand(_ Options) *cobra.Command {
	return &cobra.Command{
		Use:   "init <shell>",
		Short: "Print the rc-file snippet for the given shell",
		Long:  "Supported shells: zsh, bash, fish, pwsh",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer deps.Close()
			sh, ok := contracts.ParseShell(args[0])
			if !ok {
				return fmt.Errorf("%w: %q", contracts.ErrUnknownShell, args[0])
			}
			script, err := deps.Shell.EmitInitScript(sh)
			if err != nil {
				return err
			}
			fmt.Fprint(c.OutOrStdout(), script)
			return nil
		},
	}
}
```

- [ ] **Step 4: Remove the stub from `cli.go`. Run tests.**

```bash
go test ./internal/cli/... -run TestInit
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/init.go internal/cli/init_test.go internal/cli/cli.go
git commit -m "feat(cli): implement ccx init <shell>"
```

---

## Task 7: `internal/cli/usage.go` — `ccx usage`

**Files:**
- Create: `internal/cli/usage.go`
- Create: `internal/cli/usage_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/cli"
)

func TestUsageEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	out := runCLI(t, "usage")
	if !strings.Contains(out, "Total") {
		t.Errorf("expected 'Total' line in usage output: %q", out)
	}
}

func TestUsageJSONShape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfgDir := filepath.Join(home, "claude-work")
	_ = os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o755)
	runCLI(t, "profile", "add", "work", "--config-dir", cfgDir)

	out := runCLI(t, "usage", "--json")
	var parsed struct {
		Rows  []any   `json:"rows"`
		Total float64 `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	_ = context.Background()
}
```

- [ ] **Step 2: Run tests, verify fail**

```bash
go test ./internal/cli/... -run TestUsage
```

- [ ] **Step 3: Write `internal/cli/usage.go`**

```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/spf13/cobra"
)

func newUsageCommand(_ Options) *cobra.Command {
	var (
		profileFlag string
		since       string
		asJSON      bool
	)
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show aggregated token usage and estimated cost",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer deps.Close()

			if err := ingestAllProfiles(ctx, deps); err != nil {
				return fmt.Errorf("scanning: %w", err)
			}

			window, err := parseSince(since)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			q := contracts.UsageQuery{
				Profile: profileFlag,
				Range:   contracts.TimeRange{Start: now.Add(-window), End: now},
			}
			rows, err := deps.Store.QueryUsage(ctx, q)
			if err != nil {
				return err
			}

			var total float64
			for i, r := range rows {
				cost, _ := deps.Pricing.Cost(r.Model, r.Day, r.Usage)
				rows[i].EstimatedUSD = cost
				total += cost
			}

			if asJSON {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{
					"rows":  rows,
					"total": total,
				})
			}
			return renderUsageTable(c.OutOrStdout(), rows, total, window)
		},
	}
	cmd.Flags().StringVar(&profileFlag, "profile", "", "limit to one profile (default: all)")
	cmd.Flags().StringVar(&since, "since", "24h", "lookback window (e.g. 1d, 7d, 30d)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func parseSince(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// Handle "Nd" shorthand
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("unrecognized --since value %q", s)
}

func ingestAllProfiles(ctx context.Context, deps *Deps) error {
	profiles, err := deps.Profiles.List(ctx)
	if err != nil {
		return err
	}
	for _, p := range profiles {
		events, errs := deps.Scanner.Scan(ctx, p)
		batch := make([]contracts.Event, 0, 256)
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			if err := deps.Store.InsertEvents(ctx, p.Name, batch); err != nil {
				return err
			}
			batch = batch[:0]
			return nil
		}
		for events != nil || errs != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ev, ok := <-events:
				if !ok {
					events = nil
					if err := flush(); err != nil {
						return err
					}
					continue
				}
				batch = append(batch, ev)
				if len(batch) >= cap(batch) {
					if err := flush(); err != nil {
						return err
					}
				}
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func renderUsageTable(w interface{ Write([]byte) (int, error) }, rows []contracts.UsageRow, total float64, window time.Duration) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Usage for last %s, all profiles\n\n", window)
	fmt.Fprintln(tw, "PROFILE\tTOKENS (in/out/cache)\tEST. COST\tTOP PROJECT")
	type agg struct {
		usage   contracts.Usage
		cost    float64
		top     string
		topCost float64
	}
	per := map[string]*agg{}
	for _, r := range rows {
		a, ok := per[r.Profile]
		if !ok {
			a = &agg{}
			per[r.Profile] = a
		}
		a.usage = a.usage.Add(r.Usage)
		a.cost += r.EstimatedUSD
		if r.EstimatedUSD > a.topCost {
			a.topCost = r.EstimatedUSD
			a.top = r.Project
		}
	}
	for name, a := range per {
		if a.top == "" {
			a.top = "—"
		}
		fmt.Fprintf(tw, "%s\t%s\t$%.2f\t%s\n",
			name,
			humanTokens(a.usage),
			a.cost,
			a.top,
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w.(interface{ Write([]byte) (int, error) }), "\nTotal: $%.2f\n", total)
	return nil
}

func humanTokens(u contracts.Usage) string {
	return fmt.Sprintf("%s / %s / %s",
		humanCount(u.InputTokens),
		humanCount(u.OutputTokens),
		humanCount(u.CacheReadTokens+u.CacheCreateTokens),
	)
}

func humanCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
```

- [ ] **Step 4: Remove the stub. Run tests.**

```bash
go test ./internal/cli/... -run TestUsage
```

Expected: PASS (empty data → empty rows → "Total: $0.00" → contains "Total").

- [ ] **Step 5: Commit**

```bash
git add internal/cli/usage.go internal/cli/usage_test.go internal/cli/cli.go
git commit -m "feat(cli): implement ccx usage with table and JSON output"
```

---

## Task 8: `internal/server` — HTTP routes per `api/openapi.yaml`

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/handlers.go`
- Create: `internal/server/server_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/server_test.go`:

```go
package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/server"
)

func TestHealthEndpoint(t *testing.T) {
	srv := server.New(server.Deps{Store: &mockStore{}, Pricing: &mockPricing{}}, "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Errorf("status: %d", res.StatusCode)
	}
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	if !body.OK || body.Version != "test" {
		t.Errorf("got %+v", body)
	}
}

type mockStore struct{ contracts.Store }

func (m *mockStore) ListProfiles(_ context.Context) ([]contracts.Profile, error) {
	return []contracts.Profile{{Name: "demo", ConfigDir: "/tmp/demo"}}, nil
}

func (m *mockStore) QueryUsage(_ context.Context, _ contracts.UsageQuery) ([]contracts.UsageRow, error) {
	return nil, nil
}

type mockPricing struct{}

func (m *mockPricing) Cost(_ string, _ time.Time, _ contracts.Usage) (float64, error) {
	return 0, nil
}
func (m *mockPricing) LastUpdated() time.Time { return time.Time{} }
```

Note the missing `time` import — add at top of file.

- [ ] **Step 2: Run test, verify fail**

```bash
go test ./internal/server/...
```

- [ ] **Step 3: Write `internal/server/server.go`**

```go
// Package server implements the local HTTP API consumed by the embedded dashboard.
// It binds to 127.0.0.1 only and serves the contract defined in api/openapi.yaml.
package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Deps is the set of dependencies the server needs.
type Deps struct {
	Store    contracts.Store
	Pricing  contracts.PricingTable
	Profiles ProfileLister
	WebRoot  http.FileSystem // embedded Next.js static export
}

// ProfileLister exposes the subset of the profile manager the server needs.
type ProfileLister interface {
	List(ctx context.Context) ([]contracts.Profile, error)
}

// Server is the HTTP server.
type Server struct {
	deps    Deps
	version string
	mux     *chi.Mux
}

// New constructs a Server. Call Handler() for the http.Handler, or Serve(...)
// to start a listener.
func New(deps Deps, version string) *Server {
	s := &Server{deps: deps, version: version, mux: chi.NewRouter()}
	s.routes()
	return s
}

// Handler returns the underlying http.Handler. Useful in tests.
func (s *Server) Handler() http.Handler { return s.mux }

// Serve listens on 127.0.0.1 within the port range [startPort, endPort] and
// returns the chosen port plus a function that blocks until the server stops.
// Cancel ctx to stop.
func (s *Server) Serve(ctx context.Context, startPort, endPort int) (int, func() error, error) {
	var (
		ln   net.Listener
		err  error
		port int
	)
	for port = startPort; port <= endPort; port++ {
		ln, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", itoa(port)))
		if err == nil {
			break
		}
	}
	if ln == nil {
		return 0, nil, errors.New("no free port in range")
	}

	srv := &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return port, func() error { return srv.Serve(ln) }, nil
}

func (s *Server) routes() {
	s.mux.Use(middleware.RealIP)
	s.mux.Use(middleware.Recoverer)
	s.mux.Use(securityHeaders)
	s.mux.Get("/api/health", s.handleHealth)
	s.mux.Get("/api/profiles", s.handleProfiles)
	s.mux.Get("/api/usage", s.handleUsage)
	s.mux.Get("/api/usage/live", s.handleUsageLive)
	if s.deps.WebRoot != nil {
		s.mux.Handle("/*", http.FileServer(s.deps.WebRoot))
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func itoa(n int) string {
	return string([]rune{}) + // tiny helper to avoid importing strconv just for one call
		intToString(n)
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
```

- [ ] **Step 4: Write `internal/server/handlers.go`**

```go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "version": s.version})
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	profiles, err := s.deps.Profiles.List(ctx)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	start := time.Now().UTC().Truncate(24 * time.Hour)
	out := make([]map[string]any, 0, len(profiles))
	for _, p := range profiles {
		rows, _ := s.deps.Store.QueryUsage(ctx, contracts.UsageQuery{
			Profile: p.Name,
			Range:   contracts.TimeRange{Start: start, End: start.Add(24 * time.Hour)},
		})
		usage, cost := aggregate(s.deps.Pricing, rows)
		out = append(out, map[string]any{
			"name":         p.Name,
			"config_dir":   p.ConfigDir,
			"label":        p.Label,
			"color":        p.Color,
			"created_at":   p.CreatedAt,
			"last_used_at": p.LastUsedAt,
			"today": map[string]any{
				"usage":         usage,
				"estimated_usd": cost,
			},
		})
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	dur, err := parseSinceParam(q.Get("since"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	now := time.Now().UTC()
	rows, err := s.deps.Store.QueryUsage(ctx, contracts.UsageQuery{
		Profile: q.Get("profile"),
		Project: q.Get("project"),
		Range:   contracts.TimeRange{Start: now.Add(-dur), End: now},
	})
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for i := range rows {
		cost, _ := s.deps.Pricing.Cost(rows[i].Model, rows[i].Day, rows[i].Usage)
		rows[i].EstimatedUSD = cost
	}
	total := totalUsage(rows)
	writeJSON(w, 200, map[string]any{"rows": rows, "total": total})
}

func (s *Server) handleUsageLive(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			rows, err := s.deps.Store.QueryUsage(r.Context(), contracts.UsageQuery{
				Range: contracts.TimeRange{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
			})
			if err != nil {
				continue
			}
			b, _ := json.Marshal(rows)
			fmt.Fprintf(w, "event: usage\ndata: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func parseSinceParam(s string) (time.Duration, error) {
	if s == "" {
		return 24 * time.Hour, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("invalid since: %q", s)
}

func aggregate(p contracts.PricingTable, rows []contracts.UsageRow) (contracts.Usage, float64) {
	var (
		u contracts.Usage
		c float64
	)
	for _, r := range rows {
		u = u.Add(r.Usage)
		cost, _ := p.Cost(r.Model, r.Day, r.Usage)
		c += cost
	}
	return u, c
}

func totalUsage(rows []contracts.UsageRow) map[string]any {
	var (
		u contracts.Usage
		c float64
	)
	for _, r := range rows {
		u = u.Add(r.Usage)
		c += r.EstimatedUSD
	}
	return map[string]any{"usage": u, "estimated_usd": c}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

var _ = context.Background // silence unused import in builds where ctx isn't directly referenced
```

- [ ] **Step 5: Run tests, verify pass**

```bash
go test ./internal/server/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/
git commit -m "feat(server): implement HTTP API per openapi.yaml"
```

---

## Task 9: `internal/tui` — bubbletea profile picker

**Files:**
- Create: `internal/tui/picker.go`
- Create: `internal/tui/picker_test.go`

- [ ] **Step 1: Write the failing test**

```go
package tui_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/tui"
)

func TestPickProfileNoTTYReturnsFirst(t *testing.T) {
	// In a test environment there's no TTY; PickProfile should fall back to
	// the first profile rather than blocking.
	got, err := tui.PickProfile([]contracts.Profile{
		{Name: "work", ConfigDir: "/tmp/w"},
		{Name: "personal", ConfigDir: "/tmp/p"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "work" {
		t.Errorf("want first profile, got %q", got.Name)
	}
}

func TestPickProfileEmpty(t *testing.T) {
	_, err := tui.PickProfile(nil)
	if err == nil {
		t.Errorf("want error for empty profile list")
	}
}
```

- [ ] **Step 2: Run, verify fail**

```bash
go test ./internal/tui/...
```

- [ ] **Step 3: Write `internal/tui/picker.go`**

```go
// Package tui provides the bubbletea-based interactive profile picker used by
// `ccx use` when invoked without a profile name.
package tui

import (
	"errors"
	"fmt"
	"os"

	"github.com/arafa-dev/ccx/internal/contracts"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// PickProfile prompts the user to select one profile from the list.
// If there is no TTY (e.g., piped input, test environment), returns the first
// profile and a nil error.
func PickProfile(profiles []contracts.Profile) (contracts.Profile, error) {
	if len(profiles) == 0 {
		return contracts.Profile{}, errors.New("no profiles to pick from")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return profiles[0], nil
	}
	m := initialModel(profiles)
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return contracts.Profile{}, err
	}
	chosen := final.(model).chosen
	if chosen < 0 {
		return contracts.Profile{}, errors.New("cancelled")
	}
	return profiles[chosen], nil
}

type model struct {
	profiles []contracts.Profile
	cursor   int
	chosen   int
}

func initialModel(profiles []contracts.Profile) model {
	return model{profiles: profiles, chosen: -1}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
		case "enter":
			m.chosen = m.cursor
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func (m model) View() string {
	s := titleStyle.Render("Pick a profile:") + "\n\n"
	for i, p := range m.profiles {
		marker := "  "
		line := fmt.Sprintf("%s%-12s %s", marker, p.Name, dimStyle.Render(p.ConfigDir))
		if i == m.cursor {
			line = selectedStyle.Render("▸ " + p.Name + "   " + p.ConfigDir)
		}
		s += line + "\n"
	}
	s += "\n" + dimStyle.Render("↑/↓ select · enter confirm · q cancel")
	return s
}
```

This adds dependencies: `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `golang.org/x/term`. Run:

```bash
go mod tidy
```

- [ ] **Step 4: Run tests, verify pass**

```bash
go test ./internal/tui/...
```

Expected: PASS (test runs without TTY, takes the fallback path).

- [ ] **Step 5: Restore `tui.PickProfile` call in `use.go`**

If you stubbed `pickFirst` in Task 5, remove it and use `tui.PickProfile` again. Re-run:

```bash
go test ./internal/cli/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/tui/ internal/cli/use.go go.mod go.sum
git commit -m "feat(tui): add bubbletea profile picker with no-TTY fallback"
```

---

## Task 10: `internal/doctor` — diagnostic checks

**Files:**
- Create: `internal/doctor/doctor.go`
- Create: `internal/doctor/doctor_test.go`

- [ ] **Step 1: Write the failing test**

```go
package doctor_test

import (
	"context"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/doctor"
)

func TestRunReturnsChecks(t *testing.T) {
	d := doctor.New(doctor.Deps{Profiles: stubProfiles{}})
	checks, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) == 0 {
		t.Error("expected at least one check")
	}
	for _, c := range checks {
		if c.Name == "" {
			t.Errorf("check has empty name: %+v", c)
		}
		if c.Status != "ok" && c.Status != "warn" && c.Status != "fail" {
			t.Errorf("invalid status: %q", c.Status)
		}
	}
}

type stubProfiles struct{}

func (stubProfiles) List(_ context.Context) ([]contracts.Profile, error) {
	return nil, nil
}
```

- [ ] **Step 2: Run, verify fail**

```bash
go test ./internal/doctor/...
```

- [ ] **Step 3: Write `internal/doctor/doctor.go`**

```go
// Package doctor implements `ccx doctor` — structured diagnostic checks.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
)

// Deps holds the doctor's dependencies. ProfileLister is intentionally small
// so the doctor can be tested without spinning up real storage.
type Deps struct {
	Profiles ProfileLister
}

// ProfileLister is the minimal interface doctor needs.
type ProfileLister interface {
	List(ctx context.Context) ([]contracts.Profile, error)
}

// Doctor runs diagnostic checks.
type Doctor struct {
	deps Deps
}

// New constructs a Doctor.
func New(deps Deps) *Doctor { return &Doctor{deps: deps} }

// Run returns the list of checks. Implements contracts.Doctor.
func (d *Doctor) Run(ctx context.Context) ([]contracts.DoctorCheck, error) {
	checks := []contracts.DoctorCheck{
		d.checkClaudeOnPath(),
		d.checkCCXHome(),
		d.checkDefaultConfigDir(),
	}
	checks = append(checks, d.checkRegisteredProfiles(ctx)...)
	checks = append(checks, d.checkShellInit())
	return checks, nil
}

func (d *Doctor) checkClaudeOnPath() contracts.DoctorCheck {
	path, err := exec.LookPath("claude")
	if err != nil {
		return contracts.DoctorCheck{
			Name:        "claude on PATH",
			Status:      "fail",
			Detail:      "claude binary not found in PATH",
			Remediation: "Install Claude Code from https://claude.com/code",
		}
	}
	return contracts.DoctorCheck{Name: "claude on PATH", Status: "ok", Detail: path}
}

func (d *Doctor) checkCCXHome() contracts.DoctorCheck {
	home, err := platform.CCXHome()
	if err != nil {
		return contracts.DoctorCheck{
			Name:        "ccx home directory",
			Status:      "fail",
			Detail:      err.Error(),
			Remediation: "Verify HOME (Unix) or USERPROFILE (Windows) is set.",
		}
	}
	return contracts.DoctorCheck{Name: "ccx home directory", Status: "ok", Detail: home}
}

func (d *Doctor) checkDefaultConfigDir() contracts.DoctorCheck {
	cfg, err := platform.DefaultConfigDir()
	if err != nil {
		return contracts.DoctorCheck{
			Name:   "default Claude Code config dir",
			Status: "warn",
			Detail: err.Error(),
		}
	}
	if _, err := os.Stat(cfg); err != nil {
		return contracts.DoctorCheck{
			Name:        "default Claude Code config dir",
			Status:      "warn",
			Detail:      fmt.Sprintf("%s does not exist", cfg),
			Remediation: "Run `claude /login` once to initialize it, or register a profile with a different path.",
		}
	}
	return contracts.DoctorCheck{Name: "default Claude Code config dir", Status: "ok", Detail: cfg}
}

func (d *Doctor) checkRegisteredProfiles(ctx context.Context) []contracts.DoctorCheck {
	if d.deps.Profiles == nil {
		return nil
	}
	profiles, err := d.deps.Profiles.List(ctx)
	if err != nil {
		return []contracts.DoctorCheck{{
			Name:   "registered profiles",
			Status: "fail",
			Detail: err.Error(),
		}}
	}
	if len(profiles) == 0 {
		return []contracts.DoctorCheck{{
			Name:        "registered profiles",
			Status:      "warn",
			Detail:      "no profiles registered",
			Remediation: "Run `ccx profile add <name> --config-dir <path>` to register your first profile.",
		}}
	}
	out := make([]contracts.DoctorCheck, 0, len(profiles))
	for _, p := range profiles {
		st := "ok"
		detail := p.ConfigDir
		if _, err := os.Stat(p.ConfigDir); err != nil {
			st = "warn"
			detail = fmt.Sprintf("config dir missing: %s", p.ConfigDir)
		}
		out = append(out, contracts.DoctorCheck{
			Name:   "profile: " + p.Name,
			Status: st,
			Detail: detail,
		})
	}
	return out
}

func (d *Doctor) checkShellInit() contracts.DoctorCheck {
	if os.Getenv("CCX_ACTIVE_PROFILE") != "" {
		return contracts.DoctorCheck{
			Name:   "shell integration",
			Status: "ok",
			Detail: "active profile detected from env",
		}
	}
	return contracts.DoctorCheck{
		Name:        "shell integration",
		Status:      "warn",
		Detail:      "CCX_ACTIVE_PROFILE not set",
		Remediation: "Add `eval \"$(ccx init zsh)\"` to your shell rc file (zsh, bash, fish, or pwsh).",
	}
}
```

- [ ] **Step 4: Run tests, verify pass**

```bash
go test ./internal/doctor/...
```

- [ ] **Step 5: Wire doctor into `internal/cli/doctor.go`**

Create `internal/cli/doctor.go`:

```go
package cli

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCommand(_ Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose your install",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer deps.Close()
			d := doctor.New(doctor.Deps{Profiles: deps.Profiles})
			checks, err := d.Run(ctx)
			if err != nil {
				return err
			}
			for _, ch := range checks {
				icon := "✅"
				switch ch.Status {
				case "warn":
					icon = "⚠️ "
				case "fail":
					icon = "❌"
				}
				fmt.Fprintf(c.OutOrStdout(), "%s %s — %s\n", icon, ch.Name, ch.Detail)
				if ch.Remediation != "" && ch.Status != "ok" {
					fmt.Fprintf(c.OutOrStdout(), "   → %s\n", ch.Remediation)
				}
			}
			return nil
		},
	}
}
```

Remove the stub `newDoctorCommand` from `cli.go`.

- [ ] **Step 6: Commit**

```bash
git add internal/doctor/ internal/cli/doctor.go internal/cli/cli.go
git commit -m "feat(doctor): implement structured diagnostic checks"
```

---

## Task 11: `internal/dashboard` — embed web/out

**Files:** Modify: `internal/dashboard/dashboard.go` (was `doc.go`)

- [ ] **Step 1: Verify `web/out/index.html` exists** (from A7)

```bash
ls web/out/index.html
```

If missing: this task is blocked. Either rebuild via `pnpm --filter web build` or wait for A7 to merge.

- [ ] **Step 2: Replace `internal/dashboard/doc.go` with `dashboard.go`**

```go
// Package dashboard exposes the embedded Next.js static export as an http.FileSystem.
package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:web-out
var embedded embed.FS

// FS returns the embedded dashboard files as an http.FileSystem rooted at the
// dashboard's index. Suitable for passing to http.FileServer.
func FS() (http.FileSystem, error) {
	sub, err := fs.Sub(embedded, "web-out")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}
```

Note the embed directive points at `web-out`, **not** `../web/out`. Go's `//go:embed` only embeds paths under the package directory. So this task adds a Makefile target that symlinks (or copies) `web/out/` into `internal/dashboard/web-out/` before building.

- [ ] **Step 3: Add Makefile target**

Modify `Makefile` to add:

```make
.PHONY: stage-web
stage-web: ## Stage web/out into internal/dashboard for go:embed
	rm -rf internal/dashboard/web-out
	cp -r web/out internal/dashboard/web-out

build: stage-web ## Build the ccx binary
	@mkdir -p dist
	go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY) ./cmd/ccx
```

Add to `.gitignore`:
```
internal/dashboard/web-out/
```

- [ ] **Step 4: Test build**

```bash
make stage-web
go build ./cmd/ccx
ls dist/ccx
./dist/ccx --help
```

Expected: build succeeds, binary runs.

- [ ] **Step 5: Commit**

```bash
git rm internal/dashboard/doc.go
git add internal/dashboard/dashboard.go Makefile .gitignore
git commit -m "feat(dashboard): embed web/out via stage-web Makefile target"
```

---

## Task 12: `internal/cli/dashboard.go` — `ccx dashboard`

**Files:**
- Create: `internal/cli/dashboard.go`

- [ ] **Step 1: Write `internal/cli/dashboard.go`**

```go
package cli

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/arafa-dev/ccx/internal/dashboard"
	"github.com/arafa-dev/ccx/internal/server"
	"github.com/spf13/cobra"
)

func newDashboardCommand(opts Options) *cobra.Command {
	var (
		port   int
		noOpen bool
	)
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Open the local dashboard",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer deps.Close()

			if err := ingestAllProfiles(ctx, deps); err != nil {
				return fmt.Errorf("initial ingest: %w", err)
			}

			webFS, err := dashboard.FS()
			if err != nil {
				return fmt.Errorf("dashboard assets: %w", err)
			}

			srv := server.New(server.Deps{
				Store:    deps.Store,
				Pricing:  deps.Pricing,
				Profiles: deps.Profiles,
				WebRoot:  webFS,
			}, opts.Build.Version)

			startPort := 7777
			endPort := 7787
			if port != 0 {
				startPort, endPort = port, port
			}

			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			boundPort, runFn, err := srv.Serve(runCtx, startPort, endPort)
			if err != nil {
				return err
			}
			url := fmt.Sprintf("http://127.0.0.1:%d", boundPort)
			fmt.Fprintf(c.OutOrStdout(), "ccx dashboard at %s\n", url)
			if !noOpen {
				go func() {
					time.Sleep(300 * time.Millisecond)
					_ = openBrowser(url)
				}()
			}
			return runFn()
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "port (default: pick next free in 7777-7787)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open a browser")
	return cmd
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
```

Remove the stub `newDashboardCommand` from `cli.go`.

- [ ] **Step 2: Verify build**

```bash
make stage-web
go build ./cmd/ccx
```

- [ ] **Step 3: Smoke test (manual, no test code)**

```bash
./dist/ccx dashboard --no-open --port 7777 &
PID=$!
sleep 1
curl -s http://127.0.0.1:7777/api/health | grep '"ok":true'
kill $PID
```

- [ ] **Step 4: Commit**

```bash
git add internal/cli/dashboard.go internal/cli/cli.go
git commit -m "feat(cli): implement ccx dashboard with embedded server"
```

---

## Task 13: End-to-end integration tests

**Files:** Create: `tests/integration/integration_test.go`

- [ ] **Step 1: Write end-to-end tests**

```go
//go:build integration

package integration_test

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCCXProfileAddListUseDashboardFlow(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	cfgDir := filepath.Join(home, "claude-work")
	if err := os.MkdirAll(filepath.Join(cfgDir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"SHELL=/bin/zsh",
	)

	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		return out.String(), err
	}

	// add
	if out, err := run("profile", "add", "work", "--config-dir", cfgDir); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}

	// list
	out, err := run("profile", "list")
	if err != nil || !strings.Contains(out, "work") {
		t.Fatalf("list: %v\n%s", err, out)
	}

	// use
	out, err = run("use", "work")
	if err != nil || !strings.Contains(out, "CLAUDE_CONFIG_DIR") {
		t.Fatalf("use: %v\n%s", err, out)
	}

	// dashboard (start, hit /api/health, stop)
	dashCmd := exec.Command(bin, "dashboard", "--no-open", "--port", "7787")
	dashCmd.Env = env
	if err := dashCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer dashCmd.Process.Kill()
	time.Sleep(500 * time.Millisecond)

	res, err := http.Get("http://127.0.0.1:7787/api/health")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("health: %v code=%d", err, res.StatusCode)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "ccx")
	if _, err := exec.Command("go", "build", "-o", bin, "./cmd/ccx").CombinedOutput(); err != nil {
		t.Fatalf("build: %v", err)
	}
	return bin
}
```

- [ ] **Step 2: Add Makefile target**

```make
.PHONY: integration-test
integration-test: stage-web ## Run integration tests
	go test -tags integration -count=1 ./tests/integration/...
```

- [ ] **Step 3: Run locally**

```bash
make integration-test
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add tests/integration/ Makefile
git commit -m "test: end-to-end integration tests"
```

---

## Task 14: Expand CI matrix to include integration tests

**Files:** Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add integration-test job after the unit test job**

Append to `.github/workflows/ci.yml`:

```yaml
  integration:
    name: Integration (${{ matrix.os }})
    needs: test
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-22.04, macos-14, windows-2022]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true
      - uses: pnpm/action-setup@v4
        with:
          version: 9
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: pnpm
          cache-dependency-path: web/pnpm-lock.yaml
      - name: Build web
        run: |
          cd web
          pnpm install --frozen-lockfile
          pnpm build
      - name: Stage web for embed
        run: |
          mkdir -p internal/dashboard
          cp -r web/out internal/dashboard/web-out
      - name: Integration test
        run: go test -tags integration -count=1 ./tests/integration/...
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add integration-test job to matrix"
```

---

## Task 15: Open the integration PR

- [ ] **Step 1: Push and open PR**

```bash
git push -u origin feat/integration
gh pr create --fill --title "Phase 2: integration"
```

- [ ] **Step 2: Watch CI**

```bash
gh pr checks --watch
```

All jobs must be green before merge.

- [ ] **Step 3: Merge and tag**

After review and merge:

```bash
git checkout main
git pull --ff-only
git tag -a phase-2 -m "Phase 2 complete: ccx CLI fully integrated"
git push origin phase-2
```

---

## Done definition

- [ ] `dist/ccx --help` lists all commands from spec §4
- [ ] `dist/ccx version` prints version info
- [ ] `dist/ccx profile add/list/rm/current` all work end-to-end
- [ ] `eval "$(dist/ccx use <name>)"` switches `CLAUDE_CONFIG_DIR` and `CCX_ACTIVE_PROFILE`
- [ ] `dist/ccx init zsh|bash|fish|pwsh` emits valid rc snippets
- [ ] `dist/ccx usage` and `dist/ccx usage --json` both work
- [ ] `dist/ccx dashboard` serves the dashboard at `127.0.0.1:7777` with `/api/health` returning `{"ok":true}`
- [ ] `dist/ccx doctor` reports structured checks
- [ ] CI green on macOS, Linux, Windows (unit + integration)
- [ ] Tag `phase-2` pushed

Phase 3 (polish & launch) follows.
