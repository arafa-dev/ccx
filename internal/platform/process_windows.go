//go:build windows

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	stillActive     = 259
	detachedProcess = 0x00000008
)

func processAliveOS(pid int) bool {
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}

func processMatchesOS(pid int, expectedExecutable string) bool {
	cmd := exec.Command("wmic", "process", "where", "processid="+strconv.Itoa(pid), "get", "ExecutablePath", "/value") //nolint:gosec // Command is constant and pid is an int.
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ExecutablePath=") {
			continue
		}
		got := strings.TrimPrefix(line, "ExecutablePath=")
		return strings.EqualFold(filepath.Clean(got), filepath.Clean(expectedExecutable))
	}
	return false
}

func terminateProcessOS(pid int) error {
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid)) //nolint:gosec // command is constant and pid is an int.
	if err := cmd.Run(); err != nil && processAliveOS(pid) {
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
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return 0, err
	}
	return pid, nil
}
