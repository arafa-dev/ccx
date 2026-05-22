//go:build darwin || linux

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func processAliveOS(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func processMatchesOS(pid int, expectedExecutable string) bool {
	if runtime.GOOS == "linux" {
		exe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
		if err == nil {
			return sameExecutablePath(exe, expectedExecutable)
		}
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output() //nolint:gosec // Command is constant and pid is an int.
	if err != nil {
		return false
	}
	got := strings.TrimSpace(string(out))
	return got != "" && (sameExecutablePath(got, expectedExecutable) || filepath.Base(got) == filepath.Base(expectedExecutable))
}

func sameExecutablePath(a, b string) bool {
	cleanA, errA := filepath.EvalSymlinks(a)
	if errA != nil {
		cleanA = filepath.Clean(a)
	}
	cleanB, errB := filepath.EvalSymlinks(b)
	if errB != nil {
		cleanB = filepath.Clean(b)
	}
	return cleanA == cleanB
}

func terminateProcessOS(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

func startDetachedProcessOS(ctx context.Context, spec *DetachedProcessSpec) (int, error) {
	logFile, err := os.OpenFile(spec.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // daemon log path is controlled by ccx home.
	if err != nil {
		return 0, fmt.Errorf("open daemon log: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.CommandContext(ctx, spec.Executable, spec.Args...) //nolint:gosec // executable is os.Executable or test-injected.
	cmd.Env = spec.Env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return 0, err
	}
	return pid, nil
}
