// Package sharedscan classifies profiles by whether their projects directory
// points at the shared-projects history root.
package sharedscan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// PartitionProfiles splits profiles into shared and legacy scan partitions.
func PartitionProfiles(sharedRoot string, profiles []contracts.Profile) (shared, legacy []contracts.Profile) {
	sharedInfo, err := os.Stat(sharedRoot)
	if err != nil || !sharedInfo.IsDir() {
		return nil, append([]contracts.Profile(nil), profiles...)
	}
	realShared, err := filepath.EvalSymlinks(sharedRoot)
	if err != nil {
		return nil, append([]contracts.Profile(nil), profiles...)
	}

	for i := range profiles {
		profile := profiles[i]
		projects := filepath.Join(profile.ConfigDir, "projects")
		info, err := os.Lstat(projects)
		if errors.Is(err, fs.ErrNotExist) || err != nil || info.Mode()&os.ModeSymlink == 0 {
			legacy = append(legacy, profile)
			continue
		}
		realProjects, err := filepath.EvalSymlinks(projects)
		if err != nil || realProjects != realShared {
			legacy = append(legacy, profile)
			continue
		}
		shared = append(shared, profile)
	}

	return shared, legacy
}
