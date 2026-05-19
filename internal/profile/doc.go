// Package profile manages the ccx profile registry (~/.ccx/profiles.toml).
// It implements profile CRUD, validation, atomic file writes, and active-
// profile detection. All ProfileManager methods take context.Context. The
// registry file is rewritten atomically on every mutation (write to
// profiles.toml.tmp, then os.Rename).
package profile
