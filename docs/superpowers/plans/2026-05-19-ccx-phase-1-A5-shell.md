# ccx Phase 1 — A5 `internal/shell/` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `contracts.ShellEmitter` for zsh, bash, fish, and PowerShell. For each shell, emit two scripts: a **use script** (consumed by `eval "$(ccx use <name>)"`) that exports `CLAUDE_CONFIG_DIR` and `CCX_ACTIVE_PROFILE`, and an **init script** (one-time rc-file paste) that defines a wrapper function so `ccx use foo` works without `eval`. Properly escape profile names and config-dir paths so quote/space edge cases never produce broken shell scripts.

**Architecture:** This package depends only on `internal/contracts/` (for `Profile`, `Shell`, sentinel errors) and the Go standard library. It has no sibling-package imports. Golden-file tests under `testdata/golden/` lock down the emitted output character-for-character; the `-update` flag regenerates them.

**Tech Stack:** Go 1.22+, stdlib only.

**Spec reference:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — Section 6 (Profile switching mechanism) and Section 11.2 (table row A5).

**Worktree:** `feat/shell`

```bash
git worktree add ../ccx-shell -b feat/shell main
cd ../ccx-shell
```

**Exit criteria:**
- `go build ./internal/shell/...` succeeds
- `go test ./internal/shell/...` succeeds, including all golden-file comparisons
- `go test ./internal/shell/... -update` regenerates goldens without touching any other file
- `golangci-lint run ./internal/shell/...` reports zero issues
- All four shells × two commands produce byte-exact output matching the snippets in this plan
- PR opened against `main` and CI green

---

## Pre-flight

Confirm the worktree is on `feat/shell`, branched from a green `main` that includes the Phase 0 contracts.

```bash
pwd                                    # → /Users/arafa/Developer/ccx-shell (or equivalent)
git status                             # → On branch feat/shell, working tree clean
git log --oneline | head               # → most recent commits include Phase 0
ls internal/contracts/                 # → types.go, errors.go, interfaces.go, ...
ls internal/shell/                     # → doc.go (Phase 0 stub) and nothing else
```

If `internal/shell/doc.go` is missing, abort and re-run Phase 0. If anything else exists under `internal/shell/`, abort and ask before continuing — this worktree owns that directory exclusively.

**Conventions for this plan:**
- All Go code uses tabs for indentation (gofumpt enforced)
- All commit messages follow `type(scope): subject` — e.g., `feat(shell): add posix use script emitter`
- Every task ends with a commit; do not batch
- Run `go test ./internal/shell/...` and `golangci-lint run ./internal/shell/...` before every commit
- No imports outside `internal/contracts/` and stdlib. If you reach for another sibling package, stop — that's a contract amendment, not a feature edit.
- Snippet outputs in this plan are **byte-exact**. Trailing newlines, indentation, and quoting must match. The golden files are the enforcement mechanism.

**YAGNI guard:** This package emits strings. It does not detect the user's shell (that's `internal/platform/`), does not read profiles from disk (that's `internal/profile/`), and does not write to stdout (that's `internal/cli/`). Resist adding any abstraction not required by a failing test.

---

## Task 1: Add the `Emitter` skeleton

**Files:**
- Modify: `internal/shell/doc.go`
- Create: `internal/shell/emitter.go`

- [ ] **Step 1: Replace the package stub with a real package file**

Overwrite `internal/shell/doc.go` so it stays a doc-only file:

```go
// Package shell emits shell-specific snippets for `ccx use` (eval-style) and
// `ccx init` (one-time rc-file paste). It supports zsh, bash, fish, and
// PowerShell, and properly escapes profile names and config directory paths.
//
// This package depends only on internal/contracts and the standard library.
package shell
```

- [ ] **Step 2: Create `internal/shell/emitter.go` with the `Emitter` type**

```go
package shell

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Emitter implements contracts.ShellEmitter. It is stateless and safe for
// concurrent use.
type Emitter struct{}

// New returns a new Emitter.
func New() *Emitter {
	return &Emitter{}
}

// EmitUseScript returns the script that, when eval'd by the user's shell,
// activates the given profile. The script sets CLAUDE_CONFIG_DIR and
// CCX_ACTIVE_PROFILE. Profile name and config-dir path are escaped so that
// embedded quotes and spaces never produce a broken script.
//
// Returns contracts.ErrUnknownShell (wrapped) if sh is not a recognized shell.
func (e *Emitter) EmitUseScript(p contracts.Profile, sh contracts.Shell) (string, error) {
	switch sh {
	case contracts.ShellZsh, contracts.ShellBash:
		return emitUsePosix(p), nil
	case contracts.ShellFish:
		return emitUseFish(p), nil
	case contracts.ShellPowerShell:
		return emitUsePowerShell(p), nil
	case contracts.ShellUnknown:
		return "", fmt.Errorf("emitting use script: %w", contracts.ErrUnknownShell)
	default:
		return "", fmt.Errorf("emitting use script for %q: %w", sh.String(), contracts.ErrUnknownShell)
	}
}

// EmitInitScript returns the rc-file snippet the user pastes into their shell
// config once. The snippet defines a wrapper function so `ccx use foo` works
// without `eval`.
//
// Returns contracts.ErrUnknownShell (wrapped) if sh is not a recognized shell.
func (e *Emitter) EmitInitScript(sh contracts.Shell) (string, error) {
	switch sh {
	case contracts.ShellZsh, contracts.ShellBash:
		return initPosix, nil
	case contracts.ShellFish:
		return initFish, nil
	case contracts.ShellPowerShell:
		return initPowerShell, nil
	case contracts.ShellUnknown:
		return "", fmt.Errorf("emitting init script: %w", contracts.ErrUnknownShell)
	default:
		return "", fmt.Errorf("emitting init script for %q: %w", sh.String(), contracts.ErrUnknownShell)
	}
}

// Compile-time check that *Emitter satisfies the contract.
var _ contracts.ShellEmitter = (*Emitter)(nil)
```

The `emitUsePosix`, `emitUseFish`, `emitUsePowerShell`, `initPosix`, `initFish`, and `initPowerShell` symbols are intentionally undefined at this point — they are added in subsequent tasks. The compile error is expected.

- [ ] **Step 3: Verify compile error is exactly the expected one**

```bash
go build ./internal/shell/...
```

Expected: failure messages naming `emitUsePosix`, `emitUseFish`, `emitUsePowerShell`, `initPosix`, `initFish`, `initPowerShell`. Any other failure means you typed something different from this plan — fix before continuing.

- [ ] **Step 4: Do NOT commit yet** — the package doesn't build. Continue to Task 2.

---

## Task 2: POSIX (zsh/bash) escaping helper (TDD)

**Files:**
- Create: `internal/shell/escape.go`
- Create: `internal/shell/escape_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/shell/escape_test.go`:

```go
package shell

import "testing"

func TestEscapePosixSingle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "''"},
		{"simple", "work", "'work'"},
		{"with space", "my work", "'my work'"},
		{"with path", "/Users/arafa/.claude-profiles/work", "'/Users/arafa/.claude-profiles/work'"},
		{"with single quote", "it's", `'it'"'"'s'`},
		{"only single quote", "'", `''"'"''`},
		{"path with quote and space", "/tmp/it's a dir", `'/tmp/it'"'"'s a dir'`},
		{"double quote untouched", `say "hi"`, `'say "hi"'`},
		{"backslash untouched", `a\b`, `'a\b'`},
		{"dollar untouched", `$HOME`, `'$HOME'`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapePosixSingle(tc.in); got != tc.want {
				t.Errorf("escapePosixSingle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/shell/...
```

Expected: build/run error — `escapePosixSingle` undefined.

- [ ] **Step 3: Implement the helper**

Create `internal/shell/escape.go`:

```go
package shell

import "strings"

// escapePosixSingle wraps s in single quotes for POSIX shells (sh/bash/zsh/fish).
// Inside single quotes nothing is interpreted, including backslashes, so the
// only character that needs special handling is the single quote itself.
// We close the quote, insert a quoted literal single quote, and reopen:
//
//	it's   →   'it'"'"'s'
//
// This is the canonical POSIX-portable form used by tools like git, bash's
// printf %q (close enough), and Python's shlex.quote.
func escapePosixSingle(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// escapePowerShellSingle wraps s in single quotes for PowerShell. PowerShell
// uses doubled single quotes ('') to represent a literal single quote inside
// a single-quoted string. Inside single quotes nothing else is interpreted.
func escapePowerShellSingle(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/shell/...
```

Expected: PASS for `TestEscapePosixSingle`. The package still won't build because Task 1's emitter file references missing snippet symbols — that's fine, the test compiles the test file in isolation.

If the test does not run because of the Task 1 build error, you may temporarily comment out the body of `internal/shell/emitter.go` to land Task 2's test green, but **uncomment before committing**. A cleaner alternative: skip the test run here and rely on the test run at the end of Task 4 to validate Tasks 2 + 3 together.

- [ ] **Step 5: Do NOT commit yet** — the package doesn't fully build. Continue to Task 3.

---

## Task 3: PowerShell escaping helper (TDD)

**Files:**
- Modify: `internal/shell/escape_test.go`

- [ ] **Step 1: Append failing test**

Append to `internal/shell/escape_test.go`:

```go
func TestEscapePowerShellSingle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "''"},
		{"simple", "work", "'work'"},
		{"with space", "my work", "'my work'"},
		{"with path", `C:\Users\arafa\.claude-profiles\work`, `'C:\Users\arafa\.claude-profiles\work'`},
		{"with single quote", "it's", "'it''s'"},
		{"only single quote", "'", "''''"},
		{"path with quote and space", `C:\tmp\it's a dir`, `'C:\tmp\it''s a dir'`},
		{"double quote untouched", `say "hi"`, `'say "hi"'`},
		{"dollar untouched", `$env:HOME`, `'$env:HOME'`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapePowerShellSingle(tc.in); got != tc.want {
				t.Errorf("escapePowerShellSingle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test ./internal/shell/...
```

Expected: PASS (the implementation already exists from Task 2).

- [ ] **Step 3: Do NOT commit yet** — the package still doesn't fully build because Task 1's emitter references undefined snippet symbols. Continue to Task 4.

---

## Task 4: Use-script snippet generators

**Files:**
- Create: `internal/shell/snippets.go`

- [ ] **Step 1: Create `internal/shell/snippets.go`**

```go
package shell

import "github.com/arafa-dev/ccx/internal/contracts"

// emitUsePosix returns the export script for zsh and bash.
//
// Format (byte-exact, including trailing newline):
//
//	export CLAUDE_CONFIG_DIR='<escaped-path>'
//	export CCX_ACTIVE_PROFILE='<escaped-name>'
func emitUsePosix(p contracts.Profile) string {
	return "export CLAUDE_CONFIG_DIR=" + escapePosixSingle(p.ConfigDir) + "\n" +
		"export CCX_ACTIVE_PROFILE=" + escapePosixSingle(p.Name) + "\n"
}

// emitUseFish returns the export script for fish.
//
// Format (byte-exact, including trailing newline):
//
//	set -gx CLAUDE_CONFIG_DIR '<escaped-path>'
//	set -gx CCX_ACTIVE_PROFILE '<escaped-name>'
func emitUseFish(p contracts.Profile) string {
	return "set -gx CLAUDE_CONFIG_DIR " + escapePosixSingle(p.ConfigDir) + "\n" +
		"set -gx CCX_ACTIVE_PROFILE " + escapePosixSingle(p.Name) + "\n"
}

// emitUsePowerShell returns the export script for PowerShell.
//
// Format (byte-exact, including trailing newline):
//
//	$env:CLAUDE_CONFIG_DIR = '<escaped-path>'
//	$env:CCX_ACTIVE_PROFILE = '<escaped-name>'
func emitUsePowerShell(p contracts.Profile) string {
	return "$env:CLAUDE_CONFIG_DIR = " + escapePowerShellSingle(p.ConfigDir) + "\n" +
		"$env:CCX_ACTIVE_PROFILE = " + escapePowerShellSingle(p.Name) + "\n"
}
```

- [ ] **Step 2: Verify the package builds (snippets compile, but `initPosix` etc. still undefined)**

```bash
go build ./internal/shell/...
```

Expected: still failing on `initPosix`, `initFish`, `initPowerShell`. The three use-script functions now resolve.

- [ ] **Step 3: Do NOT commit yet.** Continue to Task 5.

---

## Task 5: Init-script constants

**Files:**
- Modify: `internal/shell/snippets.go`

- [ ] **Step 1: Append the three init-script constants**

Append to `internal/shell/snippets.go`:

```go
// initPosix is the wrapper function users paste into ~/.zshrc or ~/.bashrc.
// After installation, `ccx use foo` invokes `command ccx use foo` and evals
// the captured stdout. All other ccx subcommands pass through unchanged.
const initPosix = `ccx() {
  if [[ "$1" == "use" ]]; then
    eval "$(command ccx use "${@:2}")"
  else
    command ccx "$@"
  fi
}
`

// initFish is the wrapper function users paste into ~/.config/fish/config.fish.
const initFish = `function ccx
    if test "$argv[1]" = use
        command ccx use $argv[2..] | source
    else
        command ccx $argv
    end
end
`

// initPowerShell is the wrapper function users paste into their PowerShell
// profile (e.g., $PROFILE). It locates the on-disk ccx.exe via Get-Command so
// the function does not recurse into itself when calling out.
const initPowerShell = `function ccx {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args)
    if ($Args.Count -gt 0 -and $Args[0] -eq 'use') {
        $rest = $Args[1..($Args.Count - 1)]
        & (Get-Command ccx.exe).Path use @rest | Out-String | Invoke-Expression
    } else {
        & (Get-Command ccx.exe).Path @Args
    }
}
`
```

Each constant ends with a trailing newline (the closing backtick is on its own line — the newline before it is part of the string). Do not edit or "tidy" the whitespace. The golden files in Task 7 lock this in.

- [ ] **Step 2: Verify the package builds**

```bash
go build ./internal/shell/...
go vet ./internal/shell/...
```

Expected: no output (success).

- [ ] **Step 3: Run all tests so far**

```bash
go test ./internal/shell/...
```

Expected: PASS for `TestEscapePosixSingle` and `TestEscapePowerShellSingle`. No other tests exist yet.

- [ ] **Step 4: Commit**

```bash
git add internal/shell/doc.go internal/shell/emitter.go internal/shell/escape.go internal/shell/escape_test.go internal/shell/snippets.go
git commit -m "feat(shell): add Emitter, escapers, and snippet generators"
```

---

## Task 6: Golden-file harness with `-update` flag (TDD)

**Files:**
- Create: `internal/shell/golden_test.go`

This task wires up the golden-file framework but does not yet add the eight golden files themselves — those land in Task 7. We use Task 6 to make the harness fail loudly when goldens are missing, then Task 7 generates them.

- [ ] **Step 1: Create `internal/shell/golden_test.go`**

```go
package shell_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/shell"
)

// update regenerates the golden files when set to true via `-update`.
//
//	go test ./internal/shell/... -update
var update = flag.Bool("update", false, "regenerate golden files under testdata/golden/")

// fixtureProfile is the profile used for every golden-file test. Its name and
// config dir are deliberately benign — edge-case escaping is exercised by
// dedicated tests in escape_edge_test.go.
var fixtureProfile = contracts.Profile{
	Name:      "work",
	ConfigDir: "/Users/arafa/.claude-profiles/work",
}

func TestEmitUseScriptGolden(t *testing.T) {
	cases := []struct {
		shell  contracts.Shell
		golden string
	}{
		{contracts.ShellZsh, "use_zsh.txt"},
		{contracts.ShellBash, "use_bash.txt"},
		{contracts.ShellFish, "use_fish.txt"},
		{contracts.ShellPowerShell, "use_pwsh.txt"},
	}
	e := shell.New()
	for _, tc := range cases {
		t.Run(tc.shell.String(), func(t *testing.T) {
			got, err := e.EmitUseScript(fixtureProfile, tc.shell)
			if err != nil {
				t.Fatalf("EmitUseScript: %v", err)
			}
			compareGolden(t, tc.golden, got)
		})
	}
}

func TestEmitInitScriptGolden(t *testing.T) {
	cases := []struct {
		shell  contracts.Shell
		golden string
	}{
		{contracts.ShellZsh, "init_zsh.txt"},
		{contracts.ShellBash, "init_bash.txt"},
		{contracts.ShellFish, "init_fish.txt"},
		{contracts.ShellPowerShell, "init_pwsh.txt"},
	}
	e := shell.New()
	for _, tc := range cases {
		t.Run(tc.shell.String(), func(t *testing.T) {
			got, err := e.EmitInitScript(tc.shell)
			if err != nil {
				t.Fatalf("EmitInitScript: %v", err)
			}
			compareGolden(t, tc.golden, got)
		})
	}
}

// compareGolden reads testdata/golden/<name> and compares it to got. When the
// -update flag is set, the golden is rewritten instead. A missing golden is
// only acceptable when -update is set.
func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test ./internal/shell/... -update` to create)", path, err)
	}
	if string(want) != got {
		t.Errorf("golden mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}
```

- [ ] **Step 2: Verify the harness fails because goldens are missing**

```bash
go test ./internal/shell/...
```

Expected: failures on all 8 golden cases with messages like `read golden testdata/golden/use_zsh.txt: ...`. This proves the harness is wired correctly.

- [ ] **Step 3: Do NOT commit yet** — committing a failing test is forbidden. Continue to Task 7 to generate the goldens.

---

## Task 7: Generate the eight golden files

**Files:**
- Create: `internal/shell/testdata/golden/use_zsh.txt`
- Create: `internal/shell/testdata/golden/use_bash.txt`
- Create: `internal/shell/testdata/golden/use_fish.txt`
- Create: `internal/shell/testdata/golden/use_pwsh.txt`
- Create: `internal/shell/testdata/golden/init_zsh.txt`
- Create: `internal/shell/testdata/golden/init_bash.txt`
- Create: `internal/shell/testdata/golden/init_fish.txt`
- Create: `internal/shell/testdata/golden/init_pwsh.txt`

- [ ] **Step 1: Regenerate via the `-update` flag**

```bash
go test ./internal/shell/... -update
```

Expected: all tests PASS and eight new files appear under `internal/shell/testdata/golden/`.

- [ ] **Step 2: Spot-check each golden file matches the spec**

Read each file and confirm it matches these byte-exact contents (trailing newline included). If any does not match, fix the source in `snippets.go` and re-run `-update`.

`testdata/golden/use_zsh.txt` (identical to `use_bash.txt`):

```
export CLAUDE_CONFIG_DIR='/Users/arafa/.claude-profiles/work'
export CCX_ACTIVE_PROFILE='work'
```

`testdata/golden/use_fish.txt`:

```
set -gx CLAUDE_CONFIG_DIR '/Users/arafa/.claude-profiles/work'
set -gx CCX_ACTIVE_PROFILE 'work'
```

`testdata/golden/use_pwsh.txt`:

```
$env:CLAUDE_CONFIG_DIR = '/Users/arafa/.claude-profiles/work'
$env:CCX_ACTIVE_PROFILE = 'work'
```

`testdata/golden/init_zsh.txt` (identical to `init_bash.txt`):

```
ccx() {
  if [[ "$1" == "use" ]]; then
    eval "$(command ccx use "${@:2}")"
  else
    command ccx "$@"
  fi
}
```

`testdata/golden/init_fish.txt`:

```
function ccx
    if test "$argv[1]" = use
        command ccx use $argv[2..] | source
    else
        command ccx $argv
    end
end
```

`testdata/golden/init_pwsh.txt`:

```
function ccx {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Args)
    if ($Args.Count -gt 0 -and $Args[0] -eq 'use') {
        $rest = $Args[1..($Args.Count - 1)]
        & (Get-Command ccx.exe).Path use @rest | Out-String | Invoke-Expression
    } else {
        & (Get-Command ccx.exe).Path @Args
    }
}
```

- [ ] **Step 3: Re-run tests without `-update` to confirm they pass against the on-disk goldens**

```bash
go test ./internal/shell/...
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/shell/golden_test.go internal/shell/testdata/
git commit -m "test(shell): add golden-file harness and 8 baseline goldens"
```

---

## Task 8: Edge-case test — profile name contains a single quote

**Files:**
- Create: `internal/shell/escape_edge_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/shell/escape_edge_test.go`:

```go
package shell_test

import (
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/shell"
)

func TestEmitUseScript_NameWithSingleQuote(t *testing.T) {
	p := contracts.Profile{
		Name:      "it's-work",
		ConfigDir: "/Users/arafa/.claude-profiles/work",
	}
	e := shell.New()

	cases := []struct {
		shell contracts.Shell
		want  string
	}{
		{
			contracts.ShellZsh,
			"export CLAUDE_CONFIG_DIR='/Users/arafa/.claude-profiles/work'\n" +
				`export CCX_ACTIVE_PROFILE='it'"'"'s-work'` + "\n",
		},
		{
			contracts.ShellBash,
			"export CLAUDE_CONFIG_DIR='/Users/arafa/.claude-profiles/work'\n" +
				`export CCX_ACTIVE_PROFILE='it'"'"'s-work'` + "\n",
		},
		{
			contracts.ShellFish,
			"set -gx CLAUDE_CONFIG_DIR '/Users/arafa/.claude-profiles/work'\n" +
				`set -gx CCX_ACTIVE_PROFILE 'it'"'"'s-work'` + "\n",
		},
		{
			contracts.ShellPowerShell,
			"$env:CLAUDE_CONFIG_DIR = '/Users/arafa/.claude-profiles/work'\n" +
				"$env:CCX_ACTIVE_PROFILE = 'it''s-work'\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.shell.String(), func(t *testing.T) {
			got, err := e.EmitUseScript(p, tc.shell)
			if err != nil {
				t.Fatalf("EmitUseScript: %v", err)
			}
			if got != tc.want {
				t.Errorf("escape mismatch:\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}

// Sanity check: the escaped name must never contain an unbalanced quote run.
// If the escaper is broken, the output below would have one terminal that's
// "open" — counting single quotes (POSIX) or odd-doubled quotes (pwsh) would
// catch it.
func TestEmitUseScript_NameWithSingleQuote_QuoteBalance(t *testing.T) {
	p := contracts.Profile{Name: "a'b'c", ConfigDir: "/x"}
	e := shell.New()
	for _, sh := range []contracts.Shell{contracts.ShellZsh, contracts.ShellBash, contracts.ShellFish} {
		got, err := e.EmitUseScript(p, sh)
		if err != nil {
			t.Fatalf("%s: %v", sh, err)
		}
		// POSIX single-quote count must be even.
		if strings.Count(got, "'")%2 != 0 {
			t.Errorf("%s: odd number of single quotes — unbalanced escape: %q", sh, got)
		}
	}
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/shell/...
```

Expected: PASS. The escaping logic from Task 2/3 already handles this correctly; the test exists to lock it in.

- [ ] **Step 3: Commit**

```bash
git add internal/shell/escape_edge_test.go
git commit -m "test(shell): cover profile-name-with-single-quote escaping"
```

---

## Task 9: Edge-case test — config dir contains spaces and a single quote

**Files:**
- Modify: `internal/shell/escape_edge_test.go`

- [ ] **Step 1: Append the test**

Append to `internal/shell/escape_edge_test.go`:

```go
func TestEmitUseScript_ConfigDirWithSpacesAndQuote(t *testing.T) {
	p := contracts.Profile{
		Name:      "work",
		ConfigDir: "/Users/arafa/my profiles/it's work",
	}
	e := shell.New()

	cases := []struct {
		shell contracts.Shell
		want  string
	}{
		{
			contracts.ShellZsh,
			`export CLAUDE_CONFIG_DIR='/Users/arafa/my profiles/it'"'"'s work'` + "\n" +
				"export CCX_ACTIVE_PROFILE='work'\n",
		},
		{
			contracts.ShellBash,
			`export CLAUDE_CONFIG_DIR='/Users/arafa/my profiles/it'"'"'s work'` + "\n" +
				"export CCX_ACTIVE_PROFILE='work'\n",
		},
		{
			contracts.ShellFish,
			`set -gx CLAUDE_CONFIG_DIR '/Users/arafa/my profiles/it'"'"'s work'` + "\n" +
				"set -gx CCX_ACTIVE_PROFILE 'work'\n",
		},
		{
			contracts.ShellPowerShell,
			"$env:CLAUDE_CONFIG_DIR = '/Users/arafa/my profiles/it''s work'\n" +
				"$env:CCX_ACTIVE_PROFILE = 'work'\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.shell.String(), func(t *testing.T) {
			got, err := e.EmitUseScript(p, tc.shell)
			if err != nil {
				t.Fatalf("EmitUseScript: %v", err)
			}
			if got != tc.want {
				t.Errorf("escape mismatch:\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/shell/...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/shell/escape_edge_test.go
git commit -m "test(shell): cover config-dir-with-spaces-and-quote escaping"
```

---

## Task 10: Unknown-shell error path (TDD)

**Files:**
- Create: `internal/shell/unknown_test.go`

- [ ] **Step 1: Write the test**

Create `internal/shell/unknown_test.go`:

```go
package shell_test

import (
	"errors"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/shell"
)

func TestEmitUseScript_UnknownShell(t *testing.T) {
	e := shell.New()

	// ShellUnknown is the zero value; it is an explicit "unknown" sentinel.
	_, err := e.EmitUseScript(contracts.Profile{Name: "work", ConfigDir: "/x"}, contracts.ShellUnknown)
	if err == nil {
		t.Fatal("expected error for ShellUnknown, got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}

	// Out-of-range shell value also routes to the unknown branch.
	_, err = e.EmitUseScript(contracts.Profile{Name: "work", ConfigDir: "/x"}, contracts.Shell(999))
	if err == nil {
		t.Fatal("expected error for Shell(999), got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}
}

func TestEmitInitScript_UnknownShell(t *testing.T) {
	e := shell.New()

	_, err := e.EmitInitScript(contracts.ShellUnknown)
	if err == nil {
		t.Fatal("expected error for ShellUnknown, got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}

	_, err = e.EmitInitScript(contracts.Shell(999))
	if err == nil {
		t.Fatal("expected error for Shell(999), got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}
}

// The wrapped error must include context (the word "use" or "init" plus,
// where available, the shell name). This protects against a future refactor
// that silently drops context.
func TestEmitUseScript_UnknownShell_WrappedMessage(t *testing.T) {
	e := shell.New()
	_, err := e.EmitUseScript(contracts.Profile{Name: "x", ConfigDir: "/x"}, contracts.ShellUnknown)
	if err == nil {
		t.Fatal("expected error")
	}
	if msg := err.Error(); msg == contracts.ErrUnknownShell.Error() {
		t.Errorf("error is not wrapped with context; got bare sentinel message: %q", msg)
	}
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/shell/...
```

Expected: PASS — Task 1's emitter already wraps `contracts.ErrUnknownShell`.

- [ ] **Step 3: Commit**

```bash
git add internal/shell/unknown_test.go
git commit -m "test(shell): cover unknown-shell error wrapping"
```

---

## Task 11: Full local CI gate

- [ ] **Step 1: Format check**

```bash
gofumpt -l ./internal/shell/...
```

Expected: no output.

If files are listed, run `gofumpt -w ./internal/shell/...`, review the diff, and commit if changes are real:

```bash
gofumpt -w ./internal/shell/...
git diff
git add -u && git commit -m "style(shell): gofumpt"
```

- [ ] **Step 2: Vet**

```bash
go vet ./internal/shell/...
```

Expected: no output.

- [ ] **Step 3: Lint**

```bash
golangci-lint run ./internal/shell/...
```

Expected: exit 0, no issues.

- [ ] **Step 4: Test with race detector**

```bash
go test -race -count=1 ./internal/shell/...
```

Expected: PASS for all tests.

- [ ] **Step 5: Confirm `-update` is idempotent**

```bash
go test ./internal/shell/... -update
git status
```

Expected: `git status` reports a clean tree. If anything changed under `testdata/golden/`, the source emitters drifted from what `-update` regenerates — investigate before continuing.

- [ ] **Step 6: Confirm package isolation**

```bash
go list -deps ./internal/shell/... | grep "arafa-dev/ccx" | grep -v "internal/contracts" | grep -v "internal/shell"
```

Expected: no output. The only ccx-internal dependency of `internal/shell` is `internal/contracts`. Any other ccx-internal package in this list is a contract violation and must be removed before continuing.

If any of the above fail, fix the issue and re-run from Step 1 before opening the PR.

---

## Task 12: Open the PR

- [ ] **Step 1: Push the branch**

```bash
git push -u origin feat/shell
```

- [ ] **Step 2: Open the PR via `gh`**

```bash
gh pr create --title "feat(shell): emit use + init scripts for zsh, bash, fish, pwsh" --body "$(cat <<'EOF'
## What

Implements `contracts.ShellEmitter` (`internal/shell/`) for zsh, bash, fish, and PowerShell. Emits two scripts per shell:

- **Use script** (consumed by `eval "$(ccx use <name>)"`) sets `CLAUDE_CONFIG_DIR` and `CCX_ACTIVE_PROFILE`.
- **Init script** (one-time rc-file paste) defines a `ccx` wrapper function so `ccx use foo` works without `eval`.

Escaping handles profile names and config-dir paths containing spaces, single quotes, double quotes, and shell metacharacters.

## Why

Phase 1 plan A5. Section 6 of the design spec defines the switching mechanism; this package owns the script-emission half. Profile + CLI integration land in later plans.

## Contract impact

- [x] This PR does NOT modify `internal/contracts/`, `api/openapi.yaml`, `internal/storage/schema.sql`, or `docs/conventions.md`
- [ ] If it does, this is a contract-amendment PR (other worktrees will rebase)

## Checklist

- [x] Tests added — golden files for all 8 shell×command combos plus three escape-edge tests and unknown-shell wrapping
- [x] `make test` and `make lint` clean locally
- [x] No new dependencies (stdlib only)
- [x] User-visible behavior is internal to the package (no README change yet — that lands with the cli wiring)

## Phase 1 worktree

- Package: `internal/shell`
- Plan: `docs/superpowers/plans/2026-05-19-ccx-phase-1-A5-shell.md`
EOF
)"
```

- [ ] **Step 3: Watch CI**

```bash
gh pr checks --watch
```

Expected: lint, test (×3 OSes), build (×3 OSes) all green. If a job fails on a non-shell file, the worktree may have drifted from `main` — rebase and re-push.

If a job fails on this package, fix on the branch (do **not** force-push over green commits in someone else's review without coordination), push again, and re-watch.

- [ ] **Step 4: Wait for review and merge**

Do not self-merge. Once approved and CI is green, merge with the standard squash-or-rebase strategy chosen for the repo. After merge, the worktree's job is done; other Phase 1 worktrees rebase off the updated `main`.

---

## Phase 1 A5 done definition

All of the following are true:

- [ ] `go build ./internal/shell/...` succeeds
- [ ] `go test -race -count=1 ./internal/shell/...` succeeds
- [ ] `go test ./internal/shell/... -update` produces a clean tree (idempotent)
- [ ] `golangci-lint run ./internal/shell/...` reports zero issues
- [ ] `gofumpt -l ./internal/shell/...` produces no output
- [ ] `go list -deps ./internal/shell/...` shows only `internal/contracts` as a ccx-internal dependency
- [ ] All files from this plan exist and are committed:
  - `internal/shell/doc.go` (updated)
  - `internal/shell/emitter.go`
  - `internal/shell/escape.go`
  - `internal/shell/snippets.go`
  - `internal/shell/escape_test.go`
  - `internal/shell/escape_edge_test.go`
  - `internal/shell/unknown_test.go`
  - `internal/shell/golden_test.go`
  - `internal/shell/testdata/golden/use_zsh.txt`
  - `internal/shell/testdata/golden/use_bash.txt`
  - `internal/shell/testdata/golden/use_fish.txt`
  - `internal/shell/testdata/golden/use_pwsh.txt`
  - `internal/shell/testdata/golden/init_zsh.txt`
  - `internal/shell/testdata/golden/init_bash.txt`
  - `internal/shell/testdata/golden/init_fish.txt`
  - `internal/shell/testdata/golden/init_pwsh.txt`
- [ ] PR opened against `main`, CI green, merged
- [ ] Worktree removed: `git worktree remove ../ccx-shell`
