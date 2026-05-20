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

func detectShellOS() contracts.Shell { //nolint:unused // used after public wrappers are added in the next task
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

func credentialsPathOS(configDir string) (string, error) { //nolint:unused // used after public wrappers are added in the next task
	if configDir == "" {
		return "", fmt.Errorf("credentials path: config dir is empty")
	}
	return filepath.Join(configDir, ".credentials.json"), nil
}

func isCredentialsInKeychainOS() bool { //nolint:unused // used after public wrappers are added in the next task
	return false
}
