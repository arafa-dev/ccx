package platform_test

import (
	"os"
	"testing"

	"github.com/arafa-dev/ccx/internal/platform"
)

func TestProcessAliveRecognizesCurrentAndInvalidPID(t *testing.T) {
	if !platform.ProcessAlive(os.Getpid()) {
		t.Fatal("current process should be alive")
	}
	if platform.ProcessAlive(0) {
		t.Fatal("pid 0 should not be reported alive")
	}
	if platform.ProcessAlive(-1) {
		t.Fatal("negative pid should not be reported alive")
	}
}
