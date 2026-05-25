//go:build windows

package quotamigrate

import (
	"fmt"
	"os"
)

func createProjectsLink(target, link string) error {
	if err := os.Symlink(target, link); err != nil {
		return windowsSymlinkError(target, link, err)
	}
	return nil
}

func windowsSymlinkError(target, link string, err error) error {
	return fmt.Errorf("create directory symlink %q -> %q: %w; enable Windows Developer Mode or run ccx from an elevated shell", link, target, err)
}
