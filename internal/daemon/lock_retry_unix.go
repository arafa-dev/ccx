//go:build !windows

package daemon

func isTransientLockAccessError(error) bool {
	return false
}
