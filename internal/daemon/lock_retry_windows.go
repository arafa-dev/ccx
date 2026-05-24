//go:build windows

package daemon

import (
	"errors"
	"syscall"
)

const (
	windowsErrorSharingViolation syscall.Errno = 32
	windowsErrorLockViolation    syscall.Errno = 33
)

func isTransientLockAccessError(err error) bool {
	return errors.Is(err, windowsErrorSharingViolation) || errors.Is(err, windowsErrorLockViolation)
}
