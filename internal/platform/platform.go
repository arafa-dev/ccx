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
func parseUnixShell(shellEnv string) contracts.Shell { //nolint:unused // used by OS-specific files added later in this plan
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
