package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/arafa-dev/ccx/internal/contracts"
)

func readStatus(paths *Paths) (contracts.DaemonStatus, bool, error) {
	data, err := os.ReadFile(paths.StatusPath) //nolint:gosec // path is controlled by ccx home.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return contracts.DaemonStatus{}, false, nil
		}
		return contracts.DaemonStatus{}, false, fmt.Errorf("read daemon status: %w", err)
	}
	var status contracts.DaemonStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return contracts.DaemonStatus{}, false, fmt.Errorf("parse daemon status: %w", err)
	}
	return status, true, nil
}

func writeStatus(paths *Paths, status *contracts.DaemonStatus) error {
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		return fmt.Errorf("create daemon root: %w", err)
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("encode daemon status: %w", err)
	}
	tmp := paths.StatusPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write daemon status temp: %w", err)
	}
	if err := os.Rename(tmp, paths.StatusPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("publish daemon status: %w", err)
	}
	return nil
}

func readPID(paths *Paths) (pid int, ok bool, err error) {
	data, err := os.ReadFile(paths.PIDPath) //nolint:gosec // path is controlled by ccx home.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read daemon pid: %w", err)
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false, fmt.Errorf("parse daemon pid: %w", err)
	}
	return pid, true, nil
}

func writePID(paths *Paths, pid int) error {
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		return fmt.Errorf("create daemon root: %w", err)
	}
	return os.WriteFile(paths.PIDPath, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

func removePIDIf(paths *Paths, pid int) {
	current, ok, err := readPID(paths)
	if err == nil && ok && current == pid {
		_ = os.Remove(paths.PIDPath)
	}
}

func removeRuntimeState(paths *Paths) {
	_ = os.Remove(paths.PIDPath)
	_ = os.Remove(paths.StatusPath)
	_ = os.Remove(paths.StatusPath + ".tmp")
}
