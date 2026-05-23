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

func TestProcessIdentityRecognizesCurrentAndInvalidPID(t *testing.T) {
	first, ok := platform.ProcessIdentity(os.Getpid())
	if !ok || first == "" {
		t.Fatalf("current process identity = %q/%v, want non-empty", first, ok)
	}
	second, ok := platform.ProcessIdentity(os.Getpid())
	if !ok || second != first {
		t.Fatalf("second process identity = %q/%v, want %q/true", second, ok, first)
	}
	if identity, ok := platform.ProcessIdentity(0); ok || identity != "" {
		t.Fatalf("pid 0 identity = %q/%v, want empty/false", identity, ok)
	}
}
