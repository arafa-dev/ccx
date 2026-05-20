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

func detectShellOS() contracts.Shell { //nolint:unused // used after public wrappers are added in the next task
	return parseUnixShell(os.Getenv("SHELL"))
}

func credentialsPathOS(_ string) (string, error) { //nolint:unused // used after public wrappers are added in the next task
	return "", ErrCredentialsInKeychain
}

func isCredentialsInKeychainOS() bool { //nolint:unused // used after public wrappers are added in the next task
	return true
}
