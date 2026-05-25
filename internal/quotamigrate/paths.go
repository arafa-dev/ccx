// Package quotamigrate owns the symlink layout that B3b introduces:
// every profile's <CLAUDE_CONFIG_DIR>/projects/ is a symlink to one shared
// directory at <CCX_HOME>/shared-projects/. This package provides the path
// resolvers and the migration command logic.
package quotamigrate

import "path/filepath"

// SharedProjectsPath returns the shared-projects directory under ccxHome.
func SharedProjectsPath(ccxHome string) string {
	return filepath.Join(ccxHome, "shared-projects")
}

// ProfileProjectsPath returns the projects/ path inside the given profile
// config dir.
func ProfileProjectsPath(profileConfigDir string) string {
	return filepath.Join(profileConfigDir, "projects")
}
