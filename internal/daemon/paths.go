package daemon

import "path/filepath"

const (
	pidFileName    = "daemon.pid"
	statusFileName = "daemon.json"
	logFileName    = "daemon.log"
	lockFileName   = "daemon.lock"
	dbFileName     = "state.db"
)

// Paths are the runtime files owned by one daemon root.
type Paths struct {
	Root       string
	PIDPath    string
	StatusPath string
	LogPath    string
	LockPath   string
	DBPath     string
}

// RuntimePaths returns the daemon runtime paths for root.
func RuntimePaths(root string) Paths {
	return Paths{
		Root:       root,
		PIDPath:    filepath.Join(root, pidFileName),
		StatusPath: filepath.Join(root, statusFileName),
		LogPath:    filepath.Join(root, logFileName),
		LockPath:   filepath.Join(root, lockFileName),
		DBPath:     filepath.Join(root, dbFileName),
	}
}
