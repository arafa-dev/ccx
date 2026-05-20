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
