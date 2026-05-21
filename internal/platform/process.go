package platform

import (
	"context"
	"os"
)

// DetachedProcessSpec describes a child process that should outlive ccx.
type DetachedProcessSpec struct {
	Executable string
	Args       []string
	Env        []string
	LogPath    string
}

// OSProcessManager provides the process operations needed by daemon lifecycle
// commands.
type OSProcessManager struct{}

// Alive reports whether pid currently belongs to a live process.
func (OSProcessManager) Alive(pid int) bool {
	return ProcessAlive(pid)
}

// Terminate asks pid to exit gracefully where the platform supports it.
func (OSProcessManager) Terminate(pid int) error {
	return TerminateProcess(pid)
}

// StartDetached starts spec as a user process detached from the current ccx
// invocation and returns the child pid.
func (OSProcessManager) StartDetached(ctx context.Context, spec *DetachedProcessSpec) (int, error) {
	return StartDetachedProcess(ctx, spec)
}

// ProcessAlive reports whether pid currently belongs to a live process.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return processAliveOS(pid)
}

// TerminateProcess asks pid to exit gracefully where the platform supports it.
func TerminateProcess(pid int) error {
	if pid <= 0 {
		return os.ErrInvalid
	}
	return terminateProcessOS(pid)
}

// StartDetachedProcess starts spec as a user process detached from the current
// ccx invocation and returns the child pid.
func StartDetachedProcess(ctx context.Context, spec *DetachedProcessSpec) (int, error) {
	return startDetachedProcessOS(ctx, spec)
}
