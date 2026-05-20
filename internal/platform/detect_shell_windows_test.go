//go:build windows

package platform_test

import (
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
)

func TestDetectShellWindows(t *testing.T) {
	tests := []struct {
		name     string
		shellEnv string
		psModule string
		want     contracts.Shell
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
