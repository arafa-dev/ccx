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
