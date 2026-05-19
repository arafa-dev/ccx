package contracts_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func TestSentinelErrorsAreDistinguishable(t *testing.T) {
	wrapped := fmt.Errorf("looking up profile %q: %w", "work", contracts.ErrProfileNotFound)

	if !errors.Is(wrapped, contracts.ErrProfileNotFound) {
		t.Errorf("errors.Is should match wrapped ErrProfileNotFound")
	}
	if errors.Is(wrapped, contracts.ErrInvalidConfigDir) {
		t.Errorf("errors.Is should NOT match a different sentinel")
	}
}

func TestEveryDefinedSentinelHasMessage(t *testing.T) {
	for _, err := range []error{
		contracts.ErrProfileNotFound,
		contracts.ErrInvalidConfigDir,
		contracts.ErrProfileAlreadyExists,
		contracts.ErrConfigDirConflict,
		contracts.ErrUnknownShell,
		contracts.ErrNoActiveProfile,
	} {
		if err.Error() == "" {
			t.Errorf("sentinel %T has empty message", err)
		}
	}
}
