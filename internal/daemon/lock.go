package daemon

import (
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	defaultLockStaleAfter = 30 * time.Second
	envLockHeldByParent   = "CCX_DAEMON_LOCK_HELD"
)

// ErrStartInProgress is returned when another process is currently starting or
// running the daemon for the same ccx root.
var ErrStartInProgress = errors.New("daemon start already in progress")

type daemonLock struct {
	path  string
	owned bool
}

func acquireDaemonLock(paths *Paths, staleAfter time.Duration) (*daemonLock, error) {
	if staleAfter <= 0 {
		staleAfter = defaultLockStaleAfter
	}
	for attempts := 0; attempts < 2; attempts++ {
		file, err := os.OpenFile(paths.LockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path is controlled by ccx home.
		if err == nil {
			_, _ = fmt.Fprintf(file, "pid=%d\nstarted_at=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(paths.LockPath)
				return nil, closeErr
			}
			return &daemonLock{path: paths.LockPath, owned: true}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create daemon lock: %w", err)
		}
		stale, err := daemonLockIsStale(paths.LockPath, staleAfter)
		if err != nil {
			return nil, err
		}
		if !stale {
			return nil, ErrStartInProgress
		}
		_ = os.Remove(paths.LockPath)
	}
	return nil, ErrStartInProgress
}

func adoptDaemonLock(paths *Paths) (*daemonLock, error) {
	if _, err := os.Stat(paths.LockPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return acquireDaemonLock(paths, defaultLockStaleAfter)
		}
		return nil, fmt.Errorf("stat daemon lock: %w", err)
	}
	if err := os.WriteFile(
		paths.LockPath,
		[]byte(fmt.Sprintf("pid=%d\nstarted_at=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))),
		0o600,
	); err != nil {
		return nil, fmt.Errorf("adopt daemon lock: %w", err)
	}
	return &daemonLock{path: paths.LockPath, owned: true}, nil
}

func daemonLockIsStale(path string, staleAfter time.Duration) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("stat daemon lock: %w", err)
	}
	return time.Since(info.ModTime()) > staleAfter, nil
}

func (l *daemonLock) release() {
	if l == nil || !l.owned {
		return
	}
	_ = os.Remove(l.path)
	l.owned = false
}

func (l *daemonLock) disown() {
	if l != nil {
		l.owned = false
	}
}
