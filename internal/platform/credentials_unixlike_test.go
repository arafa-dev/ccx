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
