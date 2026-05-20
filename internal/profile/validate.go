package profile

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/arafa-dev/ccx/internal/contracts"
)

var profileNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidateProfile checks that p is well-formed enough to be stored in the
// registry. It does NOT touch the filesystem; existence checks are done by
// the Manager so that pure validation is cheap and testable.
//
// Rules:
//   - Name is non-empty
//   - Name matches ^[a-z0-9-]+$
//   - ConfigDir is non-empty and absolute (filepath.IsAbs)
func ValidateProfile(p contracts.Profile) error { //nolint:gocritic // Profile is a value-style contract type.
	if err := validateName(p.Name); err != nil {
		return err
	}
	if p.ConfigDir == "" {
		return fmt.Errorf("config_dir is empty: %w", contracts.ErrInvalidConfigDir)
	}
	if !filepath.IsAbs(p.ConfigDir) {
		return fmt.Errorf("config_dir %q is not absolute: %w", p.ConfigDir, contracts.ErrInvalidConfigDir)
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("profile: name is empty")
	}
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("profile: name %q must match ^[a-z0-9-]+$", name)
	}
	return nil
}
