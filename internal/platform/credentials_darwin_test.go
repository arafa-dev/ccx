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
