package contracts

import "errors"

// Sentinel errors. All ccx packages return one of these (wrapped with %w
// for context) for known, expected failure modes. Tests and callers use
// errors.Is to detect them.
var (
	// ErrProfileNotFound is returned when the requested profile name has no
	// entry in the registry.
	ErrProfileNotFound = errors.New("profile not found")

	// ErrProfileAlreadyExists is returned by profile-add when a profile with
	// the requested name is already registered.
	ErrProfileAlreadyExists = errors.New("profile already exists")

	// ErrInvalidConfigDir is returned when a path is not a valid Claude Code
	// config directory (e.g., not a directory, or unreadable).
	ErrInvalidConfigDir = errors.New("invalid config directory")

	// ErrConfigDirConflict is returned when two profiles would point at the
	// same config directory.
	ErrConfigDirConflict = errors.New("config directory already used by another profile")

	// ErrUnknownShell is returned when a shell name is not recognized by the
	// shell package (see ParseShell).
	ErrUnknownShell = errors.New("unknown shell")

	// ErrNoActiveProfile is returned when an operation requires an active
	// profile but neither CCX_ACTIVE_PROFILE nor CLAUDE_CONFIG_DIR is set.
	ErrNoActiveProfile = errors.New("no active profile")
)
