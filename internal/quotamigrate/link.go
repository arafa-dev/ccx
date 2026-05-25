//go:build !windows

package quotamigrate

import "os"

func createProjectsLink(target, link string) error {
	return os.Symlink(target, link)
}
