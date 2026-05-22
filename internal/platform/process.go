package platform

import (
	"context"
	"os"
	"os/exec"
	"time"
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

// Matches reports whether pid appears to be running expectedExecutable.
func (OSProcessManager) Matches(pid int, expectedExecutable string) bool {
	return ProcessMatches(pid, expectedExecutable)
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

// ProcessMatches reports whether pid appears to be running expectedExecutable.
func ProcessMatches(pid int, expectedExecutable string) bool {
	if pid <= 0 || expectedExecutable == "" {
		return false
	}
	return processMatchesOS(pid, expectedExecutable)
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

func cleanupStartedProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}
