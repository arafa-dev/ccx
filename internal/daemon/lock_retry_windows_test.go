//go:build windows

package daemon

import (
	"fmt"
	"testing"
)

func TestShouldRetryLockAdoptErrorRetriesWindowsSharingViolation(t *testing.T) {
	err := fmt.Errorf("read daemon lock: %w", windowsErrorSharingViolation)
	if !shouldRetryLockAdoptError(err) {
		t.Fatal("windows sharing violation should be retried")
	}
}
