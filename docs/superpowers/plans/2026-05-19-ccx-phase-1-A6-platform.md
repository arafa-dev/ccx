# ccx Phase 1 — A6 `internal/platform/` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `internal/platform/` — a small set of OS-conditional helpers (default Claude Code config dir, ccx home dir, path expansion, shell detection, credentials path resolution). Every other Phase 1 package depends on stdlib + `contracts/` only, but several downstream consumers (profile, doctor, cli) will reach for these helpers, so this package locks the OS-conditional surface in one place.

**Architecture:** Common code lives in `platform.go` and `platform_unix.go` / `platform_windows.go` (selected via build tags). Per-OS specializations live in `platform_darwin.go`, `platform_linux.go`, and `platform_windows.go`. The public API is identical on every platform; only the implementation switches. Imports are limited to stdlib and `github.com/arafa-dev/ccx/internal/contracts` (for the `Shell` enum).

**Tech Stack:** Go 1.22+, stdlib only.

**Spec reference:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — Section 5.1 (repo layout), Section 6 (profile switching, default config dir), Appendix A (credential locations per OS).

**Worktree:** `feat/platform` (created off `main` after Phase 0 is tagged).

```bash
git worktree add ../ccx-platform -b feat/platform main
cd ../ccx-platform
```

**Exit criteria:**

- `go build ./internal/platform/...` succeeds on darwin, linux, and windows (verified via `GOOS=... go vet`)
- `go test ./internal/platform/...` passes locally with race detector
- `golangci-lint run` reports no issues for this package
- Public API matches the surface declared in this plan (see Task 2)
- Each OS-specific file has the correct `//go:build` constraint
- PR opened against `main` with green CI

---

## Pre-flight

Confirm working directory is the `feat/platform` worktree and that Phase 0 has landed.

```bash
pwd                                 # → /Users/arafa/Developer/ccx-platform
git status                          # → On branch feat/platform, working tree clean
git log --oneline phase-0..HEAD     # → empty (we branched off phase-0 / main)
ls internal/platform/               # → doc.go only (the Phase 0 stub)
go build ./...                      # → success (nothing to build except stubs)
go test ./internal/contracts/...    # → PASS (contracts are already in main)
```

If `internal/platform/doc.go` does not exist, Phase 0 was not fully merged — stop and rebase.

**Conventions for this plan:**

- All Go code uses tabs for indentation (gofumpt enforced)
- Commit messages: `type(platform): subject` per `docs/conventions.md`
- Every task ends with a commit; do not batch
- Run `go test ./internal/platform/...` and `golangci-lint run ./internal/platform/...` before every commit
- Stdlib only. Importing anything outside stdlib + `internal/contracts` is forbidden; if a dep seems necessary, open a contract-amendment issue instead.
- Use `os.UserHomeDir()`, not `os.Getenv("HOME")` — it handles Windows correctly.
- `t.Setenv` / `t.TempDir` only; never mutate process env without restoring it.

---

## Task 1: Add `ErrCredentialsInKeychain` sentinel

**Files:**
- Create: `internal/platform/errors.go`
- Create: `internal/platform/errors_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/platform/errors_test.go`:

```go
package platform_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/arafa-dev/ccx/internal/platform"
)

func TestErrCredentialsInKeychainIsDistinguishable(t *testing.T) {
	wrapped := fmt.Errorf("resolving creds path: %w", platform.ErrCredentialsInKeychain)

	if !errors.Is(wrapped, platform.ErrCredentialsInKeychain) {
		t.Fatalf("errors.Is should match wrapped ErrCredentialsInKeychain")
	}

	if platform.ErrCredentialsInKeychain.Error() == "" {
		t.Errorf("sentinel must have a non-empty message")
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
go test ./internal/platform/...
```

Expected: FAIL — `platform.ErrCredentialsInKeychain` is undefined.

- [ ] **Step 3: Add the sentinel**

Create `internal/platform/errors.go`:

```go
package platform

import "errors"

// ErrCredentialsInKeychain is returned by CredentialsPath on macOS, where
// Claude Code stores credentials in the system Keychain rather than on disk.
// Callers should detect this with errors.Is and skip file-based credential
// checks.
var ErrCredentialsInKeychain = errors.New("credentials stored in macOS Keychain, no file path")
```

- [ ] **Step 4: Run the test and confirm it passes**

```bash
go test ./internal/platform/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/errors.go internal/platform/errors_test.go
git commit -m "feat(platform): add ErrCredentialsInKeychain sentinel"
```

---

## Task 2: Declare the public API surface

This task pins the exact public API so OS-conditional files in later tasks have the same signatures everywhere.

**Files:**
- Modify: `internal/platform/doc.go`
- Create: `internal/platform/platform.go`

- [ ] **Step 1: Replace the doc.go stub with the full package doc**

Replace `internal/platform/doc.go` contents with:

```go
// Package platform contains small OS-conditional helpers used by the rest of
// ccx: resolving the default Claude Code config directory, locating the ccx
// state directory, expanding user-supplied paths, detecting the user's shell,
// and figuring out where Claude Code stores credentials per OS.
//
// Implementation files are split via build tags:
//
//	platform.go           common, OS-independent helpers
//	platform_darwin.go    macOS-specific (keychain credentials)
//	platform_linux.go     Linux-specific (file credentials, $SHELL parsing)
//	platform_windows.go   Windows-specific (%USERPROFILE%, PowerShell heuristics)
//
// The public API is identical on every platform. Only the implementation
// switches. Callers do not need to do their own GOOS checks.
package platform
```

- [ ] **Step 2: Create `platform.go` with the cross-platform helpers and the declared API**

`platform.go` is compiled on every OS. It hosts `ExpandPath`, `IsCredentialsInKeychain`'s shared callers, and re-exports the small surface in one place so the public API is reviewable from a single file.

Create `internal/platform/platform.go`:

```go
package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// ccxHomeDirName is the directory name (relative to the user's home) where ccx
// stores its registry and SQLite cache. Exported as a constant for tests.
const ccxHomeDirName = ".ccx"

// ExpandPath expands a leading "~" (and only a leading "~") to the current
// user's home directory, then expands environment variables via os.ExpandEnv,
// and finally returns an absolute, clean path.
//
// Examples (with HOME=/Users/arafa):
//
//	"~/foo"          -> "/Users/arafa/foo"
//	"$HOME/foo"      -> "/Users/arafa/foo"
//	"./relative"     -> "<cwd>/relative"
//	"/abs"           -> "/abs"
//
// "~user" syntax is not supported (just like Go's filepath stdlib).
func ExpandPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("expand path: empty input")
	}

	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand path %q: %w", p, err)
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	}

	p = os.ExpandEnv(p)

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("expand path %q: %w", p, err)
	}
	return filepath.Clean(abs), nil
}

// CCXHome returns the ccx state directory (~/.ccx on Unix,
// %USERPROFILE%\.ccx on Windows). The directory is created with 0700
// permissions if it does not already exist.
func CCXHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate user home: %w", err)
	}
	dir := filepath.Join(home, ccxHomeDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create ccx home %q: %w", dir, err)
	}
	return dir, nil
}

// parseUnixShell maps the basename of a $SHELL value to a contracts.Shell.
// Exposed as an unexported helper so platform_unix.go (darwin+linux) and the
// table-driven tests can share the mapping.
func parseUnixShell(shellEnv string) contracts.Shell {
	if shellEnv == "" {
		return contracts.ShellUnknown
	}
	switch strings.ToLower(filepath.Base(shellEnv)) {
	case "zsh", "-zsh":
		return contracts.ShellZsh
	case "bash", "-bash":
		return contracts.ShellBash
	case "fish", "-fish":
		return contracts.ShellFish
	case "pwsh", "powershell", "powershell.exe", "pwsh.exe":
		return contracts.ShellPowerShell
	default:
		return contracts.ShellUnknown
	}
}
```

- [ ] **Step 3: Verify the package still builds**

```bash
go build ./internal/platform/...
go vet ./internal/platform/...
```

Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/platform/doc.go internal/platform/platform.go
git commit -m "feat(platform): add ExpandPath, CCXHome, and shared shell parser"
```

---

## Task 3: Test `ExpandPath`

**Files:**
- Create: `internal/platform/platform_test.go`

- [ ] **Step 1: Write the table-driven test**

Create `internal/platform/platform_test.go`:

```go
package platform_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arafa-dev/ccx/internal/platform"
)

func TestExpandPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Windows respects USERPROFILE; set both so the test is OS-portable.
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("CCX_TEST_VAR", "expanded")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tilde alone", "~", tmp},
		{"tilde with subpath", "~/foo/bar", filepath.Join(tmp, "foo", "bar")},
		{"env var", "$CCX_TEST_VAR/x", filepath.Join(cwd, "expanded", "x")},
		{"already absolute", filepath.Join(tmp, "abs"), filepath.Join(tmp, "abs")},
		{"relative", "rel/path", filepath.Join(cwd, "rel", "path")},
		{"double slash cleaned", "~//foo", filepath.Join(tmp, "foo")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := platform.ExpandPath(tc.in)
			if err != nil {
				t.Fatalf("ExpandPath(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if !filepath.IsAbs(got) {
				t.Errorf("ExpandPath(%q) = %q is not absolute", tc.in, got)
			}
		})
	}
}

func TestExpandPathEmptyInputErrors(t *testing.T) {
	if _, err := platform.ExpandPath(""); err == nil {
		t.Fatal("ExpandPath(\"\") should return an error")
	} else if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error %q should mention empty input", err.Error())
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
go test ./internal/platform/...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/platform_test.go
git commit -m "test(platform): add ExpandPath table tests"
```

---

## Task 4: Test `CCXHome`

**Files:**
- Modify: `internal/platform/platform_test.go`

- [ ] **Step 1: Append the test**

Append to `internal/platform/platform_test.go`:

```go
func TestCCXHomeCreatesDirIfMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	got, err := platform.CCXHome()
	if err != nil {
		t.Fatalf("CCXHome: %v", err)
	}

	want := filepath.Join(tmp, ".ccx")
	if got != want {
		t.Errorf("CCXHome = %q, want %q", got, want)
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat %q: %v", got, err)
	}
	if !info.IsDir() {
		t.Errorf("CCXHome %q is not a directory", got)
	}
	// Permissions check is Unix-only; Windows uses ACLs and reports 0777-ish.
	if runtimeIsUnix() {
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("CCXHome perm = %o, want 0700", perm)
		}
	}
}

func TestCCXHomeIdempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	first, err := platform.CCXHome()
	if err != nil {
		t.Fatalf("first CCXHome: %v", err)
	}
	second, err := platform.CCXHome()
	if err != nil {
		t.Fatalf("second CCXHome: %v", err)
	}
	if first != second {
		t.Errorf("CCXHome not idempotent: %q vs %q", first, second)
	}
}

func runtimeIsUnix() bool {
	return os.PathSeparator == '/'
}
```

- [ ] **Step 2: Run the tests**

```bash
go test ./internal/platform/...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/platform_test.go
git commit -m "test(platform): add CCXHome creation and idempotence tests"
```

---

## Task 5: Implement `DefaultConfigDir` (cross-platform skeleton + env override)

**Files:**
- Modify: `internal/platform/platform.go`

- [ ] **Step 1: Add the common helper that handles the env override**

Append to `internal/platform/platform.go`:

```go
// claudeConfigDirEnv is the env var Claude Code reads to override its config
// directory. Documented at code.claude.com/docs/en/env-vars.
const claudeConfigDirEnv = "CLAUDE_CONFIG_DIR"

// DefaultConfigDir returns the platform-default Claude Code config directory.
//
//	macOS:   $HOME/.claude (or $HOME/.config/claude if that exists)
//	Linux:   $HOME/.claude (or $HOME/.config/claude if that exists)
//	Windows: %USERPROFILE%\.claude
//
// If CLAUDE_CONFIG_DIR is set in the environment, its value (after path
// expansion) is returned instead. The returned path is absolute but may not
// yet exist on disk.
func DefaultConfigDir() (string, error) {
	if override := os.Getenv(claudeConfigDirEnv); override != "" {
		return ExpandPath(override)
	}
	return defaultConfigDirOS()
}
```

`defaultConfigDirOS` is declared in the per-OS files in Task 6.

- [ ] **Step 2: Verify the file still compiles**

`go build` will fail until Task 6 defines `defaultConfigDirOS` on each platform, so do not run `go build` yet. Just confirm the file parses:

```bash
gofumpt -l internal/platform/
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/platform.go
git commit -m "feat(platform): add DefaultConfigDir with CLAUDE_CONFIG_DIR override"
```

---

## Task 6: Implement OS-specific files

**Files:**
- Create: `internal/platform/platform_darwin.go`
- Create: `internal/platform/platform_linux.go`
- Create: `internal/platform/platform_windows.go`

Each file declares `defaultConfigDirOS`, `detectShellOS`, `credentialsPathOS`, and `isCredentialsInKeychainOS`, behind a `//go:build` constraint matching its filename. The public functions in `platform.go` (Task 7) call into these.

- [ ] **Step 1: Create `platform_darwin.go`**

```go
//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func defaultConfigDirOS() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate user home: %w", err)
	}
	xdg := filepath.Join(home, ".config", "claude")
	if info, err := os.Stat(xdg); err == nil && info.IsDir() {
		return xdg, nil
	}
	return filepath.Join(home, ".claude"), nil
}

func detectShellOS() contracts.Shell {
	return parseUnixShell(os.Getenv("SHELL"))
}

func credentialsPathOS(_ string) (string, error) {
	return "", ErrCredentialsInKeychain
}

func isCredentialsInKeychainOS() bool {
	return true
}
```

- [ ] **Step 2: Create `platform_linux.go`**

```go
//go:build linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func defaultConfigDirOS() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate user home: %w", err)
	}
	xdg := filepath.Join(home, ".config", "claude")
	if info, err := os.Stat(xdg); err == nil && info.IsDir() {
		return xdg, nil
	}
	return filepath.Join(home, ".claude"), nil
}

func detectShellOS() contracts.Shell {
	return parseUnixShell(os.Getenv("SHELL"))
}

func credentialsPathOS(configDir string) (string, error) {
	if configDir == "" {
		return "", fmt.Errorf("credentials path: config dir is empty")
	}
	return filepath.Join(configDir, ".credentials.json"), nil
}

func isCredentialsInKeychainOS() bool {
	return false
}
```

- [ ] **Step 3: Create `platform_windows.go`**

```go
//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func defaultConfigDirOS() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate user home: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

func detectShellOS() contracts.Shell {
	// PowerShell sets PSModulePath in every session it spawns. Bash on Windows
	// (Git Bash, WSL) sets SHELL just like Unix; check that first so a user
	// running ccx from Git Bash isn't misreported as pwsh.
	if s := parseUnixShell(os.Getenv("SHELL")); s != contracts.ShellUnknown {
		return s
	}
	if os.Getenv("PSModulePath") != "" {
		return contracts.ShellPowerShell
	}
	return contracts.ShellUnknown
}

func credentialsPathOS(configDir string) (string, error) {
	if configDir == "" {
		return "", fmt.Errorf("credentials path: config dir is empty")
	}
	return filepath.Join(configDir, ".credentials.json"), nil
}

func isCredentialsInKeychainOS() bool {
	return false
}
```

- [ ] **Step 4: Verify each file's build tag with cross-compilation**

```bash
GOOS=darwin  go vet ./internal/platform/...
GOOS=linux   go vet ./internal/platform/...
GOOS=windows go vet ./internal/platform/...
```

Expected: all three exit 0 with no output. If `vet` complains about unused identifiers, the build tag is wrong on that file.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/platform_darwin.go internal/platform/platform_linux.go internal/platform/platform_windows.go
git commit -m "feat(platform): add OS-specific defaultConfigDir, detectShell, credentialsPath"
```

---

## Task 7: Wire the cross-platform public API to OS helpers

**Files:**
- Modify: `internal/platform/platform.go`

- [ ] **Step 1: Append the public API wrappers**

Append to `internal/platform/platform.go`:

```go
// DetectShell returns the user's current shell, inferring from $SHELL on Unix
// and from $PSModulePath (or parent shell hints) on Windows. Returns
// contracts.ShellUnknown when it cannot decide.
func DetectShell() contracts.Shell {
	return detectShellOS()
}

// CredentialsPath returns the credentials file path for the given Claude Code
// config directory. On macOS the credentials live in the Keychain and this
// function returns ("", ErrCredentialsInKeychain); callers should detect that
// with errors.Is.
func CredentialsPath(configDir string) (string, error) {
	return credentialsPathOS(configDir)
}

// IsCredentialsInKeychain reports whether the current OS stores Claude Code
// credentials in the system keychain (true on darwin) rather than on disk
// (false on linux/windows).
func IsCredentialsInKeychain() bool {
	return isCredentialsInKeychainOS()
}
```

- [ ] **Step 2: Verify everything builds and lints**

```bash
go build ./internal/platform/...
go vet ./internal/platform/...
golangci-lint run ./internal/platform/...
```

Expected: all exit 0 with no output.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/platform.go
git commit -m "feat(platform): wire DetectShell, CredentialsPath, IsCredentialsInKeychain"
```

---

## Task 8: Test `DefaultConfigDir`

**Files:**
- Modify: `internal/platform/platform_test.go`

- [ ] **Step 1: Append the tests**

Append to `internal/platform/platform_test.go`:

```go
func TestDefaultConfigDirReturnsAbsolutePath(t *testing.T) {
	// Clear the override so we exercise the OS default branch.
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	got, err := platform.DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir: %v", err)
	}
	if got == "" {
		t.Fatal("DefaultConfigDir returned empty")
	}
	if !filepath.IsAbs(got) {
		t.Errorf("DefaultConfigDir = %q is not absolute", got)
	}
	// It should be located under the (fake) home.
	if !strings.HasPrefix(got, tmp) {
		t.Errorf("DefaultConfigDir = %q should be under HOME %q", got, tmp)
	}
}

func TestDefaultConfigDirRespectsEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	override := filepath.Join(tmp, "custom-claude")
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("CLAUDE_CONFIG_DIR", override)

	got, err := platform.DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir: %v", err)
	}
	if got != override {
		t.Errorf("DefaultConfigDir = %q, want override %q", got, override)
	}
}

func TestDefaultConfigDirExpandsTildeInOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("CLAUDE_CONFIG_DIR", "~/elsewhere")

	got, err := platform.DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir: %v", err)
	}
	want := filepath.Join(tmp, "elsewhere")
	if got != want {
		t.Errorf("DefaultConfigDir = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
go test ./internal/platform/...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/platform_test.go
git commit -m "test(platform): cover DefaultConfigDir default and env override"
```

---

## Task 9: Test `DetectShell`

**Files:**
- Create: `internal/platform/detect_shell_unix_test.go`
- Create: `internal/platform/detect_shell_windows_test.go`

Build-tagged tests so each OS only runs the assertions that match its `detectShellOS` implementation.

- [ ] **Step 1: Create the Unix test file**

```go
//go:build darwin || linux

package platform_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
)

func TestDetectShellUnix(t *testing.T) {
	tests := []struct {
		name     string
		shellEnv string
		want     contracts.Shell
	}{
		{"zsh", "/bin/zsh", contracts.ShellZsh},
		{"bash", "/bin/bash", contracts.ShellBash},
		{"fish", "/usr/local/bin/fish", contracts.ShellFish},
		{"login zsh", "-zsh", contracts.ShellZsh},
		{"unknown shell", "/bin/tcsh", contracts.ShellUnknown},
		{"empty", "", contracts.ShellUnknown},
		{"uppercase basename", "/usr/bin/BASH", contracts.ShellBash},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SHELL", tc.shellEnv)
			if got := platform.DetectShell(); got != tc.want {
				t.Errorf("DetectShell with SHELL=%q = %v, want %v", tc.shellEnv, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Create the Windows test file**

```go
//go:build windows

package platform_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
)

func TestDetectShellWindows(t *testing.T) {
	tests := []struct {
		name       string
		shellEnv   string
		psModule   string
		want       contracts.Shell
	}{
		{"SHELL set wins", "C:\\Program Files\\Git\\bin\\bash.exe", "C:\\Modules", contracts.ShellBash},
		{"pwsh via PSModulePath", "", "C:\\Modules", contracts.ShellPowerShell},
		{"nothing set", "", "", contracts.ShellUnknown},
		{"unknown shell falls back to pwsh hint", "C:\\tcsh.exe", "C:\\Modules", contracts.ShellPowerShell},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SHELL", tc.shellEnv)
			t.Setenv("PSModulePath", tc.psModule)
			if got := platform.DetectShell(); got != tc.want {
				t.Errorf("DetectShell(SHELL=%q,PSModulePath=%q) = %v, want %v",
					tc.shellEnv, tc.psModule, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run the tests for the current OS**

```bash
go test ./internal/platform/...
```

Expected: PASS (on macOS/Linux the Unix file runs; the Windows file is skipped by the build tag).

- [ ] **Step 4: Cross-verify the Windows file still type-checks**

```bash
GOOS=windows go vet ./internal/platform/...
```

Expected: exit 0.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/detect_shell_unix_test.go internal/platform/detect_shell_windows_test.go
git commit -m "test(platform): table-driven DetectShell coverage per OS"
```

---

## Task 10: Test `CredentialsPath` and `IsCredentialsInKeychain`

**Files:**
- Create: `internal/platform/credentials_darwin_test.go`
- Create: `internal/platform/credentials_unixlike_test.go`

- [ ] **Step 1: Create the darwin-only test**

```go
//go:build darwin

package platform_test

import (
	"errors"
	"testing"

	"github.com/arafa-dev/ccx/internal/platform"
)

func TestCredentialsPathDarwinReturnsKeychainSentinel(t *testing.T) {
	got, err := platform.CredentialsPath("/some/config")
	if got != "" {
		t.Errorf("CredentialsPath on darwin = %q, want empty", got)
	}
	if !errors.Is(err, platform.ErrCredentialsInKeychain) {
		t.Errorf("CredentialsPath err = %v, want wraps ErrCredentialsInKeychain", err)
	}
}

func TestIsCredentialsInKeychainDarwin(t *testing.T) {
	if !platform.IsCredentialsInKeychain() {
		t.Error("IsCredentialsInKeychain on darwin must be true")
	}
}
```

- [ ] **Step 2: Create the linux+windows test**

```go
//go:build linux || windows

package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/arafa-dev/ccx/internal/platform"
)

func TestCredentialsPathReturnsFileUnderConfigDir(t *testing.T) {
	cfg := filepath.Join("home", "user", ".claude")
	got, err := platform.CredentialsPath(cfg)
	if err != nil {
		t.Fatalf("CredentialsPath: %v", err)
	}
	want := filepath.Join(cfg, ".credentials.json")
	if got != want {
		t.Errorf("CredentialsPath(%q) = %q, want %q", cfg, got, want)
	}
}

func TestCredentialsPathRejectsEmptyConfigDir(t *testing.T) {
	if _, err := platform.CredentialsPath(""); err == nil {
		t.Error("CredentialsPath(\"\") should return an error")
	}
}

func TestIsCredentialsInKeychainNonDarwin(t *testing.T) {
	if platform.IsCredentialsInKeychain() {
		t.Error("IsCredentialsInKeychain on linux/windows must be false")
	}
}
```

- [ ] **Step 3: Run the tests**

```bash
go test ./internal/platform/...
```

Expected: PASS (one of the two files runs depending on host OS).

- [ ] **Step 4: Cross-compile sanity**

```bash
GOOS=darwin  go vet ./internal/platform/...
GOOS=linux   go vet ./internal/platform/...
GOOS=windows go vet ./internal/platform/...
```

Expected: all exit 0.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/credentials_darwin_test.go internal/platform/credentials_unixlike_test.go
git commit -m "test(platform): cover CredentialsPath and IsCredentialsInKeychain per OS"
```

---

## Task 11: Cross-compile sanity sweep

Verify that every build tag is correct and the package compiles cleanly on each target. This catches the common mistake of forgetting `//go:build` on a per-OS file (which would cause duplicate-symbol errors).

- [ ] **Step 1: Cross-compile for each supported target**

```bash
GOOS=darwin  GOARCH=amd64 go build ./internal/platform/...
GOOS=darwin  GOARCH=arm64 go build ./internal/platform/...
GOOS=linux   GOARCH=amd64 go build ./internal/platform/...
GOOS=linux   GOARCH=arm64 go build ./internal/platform/...
GOOS=windows GOARCH=amd64 go build ./internal/platform/...
GOOS=windows GOARCH=arm64 go build ./internal/platform/...
```

Expected: every command exits 0 with no output.

- [ ] **Step 2: Vet each target**

```bash
GOOS=darwin  go vet ./internal/platform/...
GOOS=linux   go vet ./internal/platform/...
GOOS=windows go vet ./internal/platform/...
```

Expected: every command exits 0 with no output.

- [ ] **Step 3: Final lint and test on the host**

```bash
gofumpt -l internal/platform/
golangci-lint run ./internal/platform/...
go test -race -count=1 ./internal/platform/...
```

Expected: `gofumpt` emits no filenames; `golangci-lint` exits 0; tests PASS.

If anything fails, fix it locally and amend the most recent commit only if the fix is purely cosmetic; otherwise add a new commit with a focused message.

- [ ] **Step 4: No commit needed if the sweep is clean**

The sweep is a verification step. If you had to make code fixes, commit them separately:

```bash
git add internal/platform/
git commit -m "fix(platform): address cross-compile or lint findings"
```

---

## Task 12: Open the pull request

- [ ] **Step 1: Push the branch**

```bash
git push -u origin feat/platform
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create \
  --title "feat(platform): OS-conditional helpers (config dir, shell detect, credentials)" \
  --body "Implements plan A6.

## Scope

- Adds \`internal/platform/\` with the six public functions required by Phase 1 plan A6.
- Build-tag split into \`platform.go\`, \`platform_darwin.go\`, \`platform_linux.go\`, \`platform_windows.go\`.
- Stdlib + \`internal/contracts\` only.
- Cross-compile verified for darwin/linux/windows × amd64/arm64.

## Contract impact

- [x] This PR does NOT modify \`internal/contracts/\`, \`api/openapi.yaml\`, \`internal/storage/schema.sql\`, or \`docs/conventions.md\`.

## Phase 1 worktree

- Package: \`internal/platform\`
- Plan: \`docs/superpowers/plans/2026-05-19-ccx-phase-1-A6-platform.md\`
"
```

- [ ] **Step 3: Watch CI**

```bash
gh pr checks --watch
```

Expected: lint, test (×3 OSes), build (×3 OSes) all green.

- [ ] **Step 4: Request review and merge after approval**

Do not self-merge a contract-adjacent package without one human review. After approval and a green run:

```bash
gh pr merge --squash --delete-branch
```

---

## Phase 1 A6 done definition

All of the following are true:

- [ ] `go build ./internal/platform/...` succeeds on the host
- [ ] `GOOS={darwin,linux,windows} go vet ./internal/platform/...` all succeed
- [ ] `go test -race -count=1 ./internal/platform/...` passes on the host
- [ ] `golangci-lint run ./internal/platform/...` reports zero issues
- [ ] `gofumpt -l internal/platform/` produces no output
- [ ] All files from this plan exist and are committed:
  - `internal/platform/doc.go` (replaced)
  - `internal/platform/errors.go`
  - `internal/platform/platform.go`
  - `internal/platform/platform_darwin.go`
  - `internal/platform/platform_linux.go`
  - `internal/platform/platform_windows.go`
  - `internal/platform/errors_test.go`
  - `internal/platform/platform_test.go`
  - `internal/platform/detect_shell_unix_test.go`
  - `internal/platform/detect_shell_windows_test.go`
  - `internal/platform/credentials_darwin_test.go`
  - `internal/platform/credentials_unixlike_test.go`
- [ ] Public API surface matches Task 2 exactly:
  - `DefaultConfigDir() (string, error)`
  - `CCXHome() (string, error)`
  - `ExpandPath(p string) (string, error)`
  - `DetectShell() contracts.Shell`
  - `CredentialsPath(configDir string) (string, error)`
  - `IsCredentialsInKeychain() bool`
  - `ErrCredentialsInKeychain` (sentinel)
- [ ] PR merged to `main` with green CI on all three OSes

After merge, the `feat/platform` worktree can be removed:

```bash
git worktree remove ../ccx-platform
git branch -d feat/platform
```
