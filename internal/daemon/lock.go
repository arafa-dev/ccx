package daemon

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	defaultLockStaleAfter = 30 * time.Second
	envLockHeldByParent   = "CCX_DAEMON_LOCK_HELD"
	envLockToken          = "CCX_DAEMON_LOCK_TOKEN" //nolint:gosec // This is an environment variable name, not a credential value.
)

// ErrStartInProgress is returned when another process is currently starting or
// running the daemon for the same ccx root.
var ErrStartInProgress = errors.New("daemon start already in progress")

type daemonLockRecord struct {
	Token     string    `json:"token"`
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

type daemonLock struct {
	path  string
	token string
	owned bool
}

var beforeObservedLockRemoveHook func()

func setBeforeObservedLockRemoveHookForTest(hook func()) func() {
	old := beforeObservedLockRemoveHook
	beforeObservedLockRemoveHook = hook
	return func() {
		beforeObservedLockRemoveHook = old
	}
}

func acquireDaemonLock(paths *Paths, staleAfter time.Duration, token string) (*daemonLock, error) {
	if staleAfter <= 0 {
		staleAfter = defaultLockStaleAfter
	}
	if token == "" {
		var err error
		token, err = newLockToken()
		if err != nil {
			return nil, err
		}
	}
	for attempts := 0; attempts < 2; attempts++ {
		record := daemonLockRecord{Token: token, PID: os.Getpid(), CreatedAt: time.Now().UTC()}
		file, err := os.OpenFile(paths.LockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path is controlled by ccx home.
		if err == nil {
			if err := encodeLockRecord(file, record); err != nil {
				_ = file.Close()
				_ = os.Remove(paths.LockPath)
				return nil, err
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(paths.LockPath)
				return nil, closeErr
			}
			return &daemonLock{path: paths.LockPath, token: token, owned: true}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create daemon lock: %w", err)
		}
		observed, observedBytes, err := readLockRecord(paths.LockPath)
		if err != nil {
			return nil, err
		}
		if !lockRecordIsStale(observed, staleAfter) {
			return nil, ErrStartInProgress
		}
		removed, err := removeObservedLock(paths.LockPath, observedBytes)
		if err != nil {
			return nil, err
		}
		if !removed {
			return nil, ErrStartInProgress
		}
	}
	return nil, ErrStartInProgress
}

func adoptDaemonLock(paths *Paths, token string) (*daemonLock, error) {
	if token == "" {
		return nil, fmt.Errorf("adopt daemon lock: token is empty")
	}
	record, _, err := readLockRecord(paths.LockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return acquireDaemonLock(paths, defaultLockStaleAfter, token)
		}
		return nil, err
	}
	if record.Token != token {
		return nil, fmt.Errorf("adopt daemon lock: token mismatch")
	}
	record.PID = os.Getpid()
	record.CreatedAt = time.Now().UTC()
	if err := writeLockRecord(paths.LockPath, record); err != nil {
		return nil, fmt.Errorf("adopt daemon lock: %w", err)
	}
	return &daemonLock{path: paths.LockPath, token: token, owned: true}, nil
}

func newLockToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate daemon lock token: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

func readLockRecord(path string) (daemonLockRecord, []byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is controlled by ccx home.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return daemonLockRecord{}, nil, os.ErrNotExist
		}
		return daemonLockRecord{}, nil, fmt.Errorf("read daemon lock: %w", err)
	}
	var record daemonLockRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return daemonLockRecord{}, nil, fmt.Errorf("parse daemon lock: %w", err)
	}
	return record, data, nil
}

func writeLockRecord(path string, record daemonLockRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode daemon lock: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write daemon lock: %w", err)
	}
	return nil
}

func encodeLockRecord(file *os.File, record daemonLockRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode daemon lock: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write daemon lock: %w", err)
	}
	return nil
}

func lockRecordIsStale(record daemonLockRecord, staleAfter time.Duration) bool {
	if record.CreatedAt.IsZero() {
		return true
	}
	return time.Since(record.CreatedAt) > staleAfter
}

func removeObservedLock(path string, observed []byte) (bool, error) {
	if beforeObservedLockRemoveHook != nil {
		beforeObservedLockRemoveHook()
	}
	current, err := os.ReadFile(path) //nolint:gosec // path is controlled by ccx home.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read daemon lock before remove: %w", err)
	}
	if !bytes.Equal(current, observed) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("remove stale daemon lock: %w", err)
	}
	return true, nil
}

func (l *daemonLock) release() {
	if l == nil || !l.owned {
		return
	}
	record, _, err := readLockRecord(l.path)
	if err == nil && record.Token == l.token {
		_ = os.Remove(l.path) //nolint:gosec // daemon lock path is controlled by ccx home.
	}
	l.owned = false
}

func (l *daemonLock) disown() {
	if l != nil {
		l.owned = false
	}
}
