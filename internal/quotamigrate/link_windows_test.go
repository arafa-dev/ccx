//go:build windows

package quotamigrate

import (
	"errors"
	"strings"
	"testing"
)

func TestWindowsSymlinkErrorExplainsRequiredPrivilege(t *testing.T) {
	err := windowsSymlinkError(`C:\ccx\shared-projects`, `C:\profile\projects`, errors.New("privilege not held"))
	if err == nil {
		t.Fatal("windowsSymlinkError returned nil")
	}
	got := err.Error()
	for _, want := range []string{"Developer Mode", "elevated shell", "privilege not held"} {
		if !strings.Contains(got, want) {
			t.Fatalf("error %q missing %q", got, want)
		}
	}
}
