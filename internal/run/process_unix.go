//go:build darwin || linux

package run

import (
	"os"
	"syscall"
)

func signalTerminateProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
